#!/usr/bin/env bash
# Build the static FFmpeg that ships inside Icelos.app from the pinned sources in
# mac/ffmpeg-sources.txt.
#
# Usage: mac/build-ffmpeg.sh [--fetch-only] [arm64|x86_64]   (default: the host architecture)
#
# --fetch-only downloads and verifies the tarballs, writes src/MANIFEST naming them, and stops
# before any build. mac/release.sh uses it to pack the corresponding source.
#
# Prerequisites: Xcode command line tools, then
#   brew install cmake nasm meson ninja pkg-config autoconf automake
#
# Output: mac/.build/ffmpeg/<arch>/bin/ffmpeg and ffprobe, linked only against system
# libraries and frameworks. Tarballs are cached in mac/.build/ffmpeg/src and every one is
# checked against its SHA-256 before anything builds. Each library leaves a marker of its tarball
# sum and build recipe, so a rerun skips what is already built. ICELOS_JOBS sets build parallelism.
#
# FFmpeg is configured GPL with version3 (opencore-amr is Apache-2.0), never nonfree. The macOS
# SDK has no lzma.h, so lzma is off.
set -euo pipefail

here="$(cd "$(dirname "$0")" && pwd)"
sources="$here/ffmpeg-sources.txt"
script="$here/$(basename "$0")"
root="$here/.build/ffmpeg"

fetch_only=0
if [ "${1:-}" = --fetch-only ]; then
	fetch_only=1
	shift
fi

host_arch="$(uname -m)"
arch="${1:-$host_arch}"
case "$arch" in
arm64)
	triple=aarch64-apple-darwin
	vpx_target=arm64-darwin23-gcc
	ff_arch=aarch64
	meson_cpu_family=aarch64
	;;
x86_64)
	triple=x86_64-apple-darwin
	vpx_target=x86_64-darwin23-gcc
	ff_arch=x86_64
	meson_cpu_family=x86_64
	;;
*)
	echo "build-ffmpeg: unknown arch $arch (want arm64 or x86_64)" >&2
	exit 2
	;;
esac
cross=0
[ "$arch" != "$host_arch" ] && cross=1

src="$root/src"
prefix="$root/$arch"
work="$root/build-$arch"
logs="$root/logs-$arch"
done_dir="$prefix/.done"
jobs="${ICELOS_JOBS:-$(sysctl -n hw.ncpu)}"
mkdir -p "$src"

# Keep Homebrew and user search paths out of every build.
unset CPATH C_INCLUDE_PATH CPLUS_INCLUDE_PATH LIBRARY_PATH PKG_CONFIG_PATH DYLD_LIBRARY_PATH
export PKG_CONFIG_LIBDIR="$prefix/lib/pkgconfig"
export MACOSX_DEPLOYMENT_TARGET=14.0
export CC=clang CXX=clang++
base_flags="-arch $arch -mmacosx-version-min=14.0"
export CFLAGS="$base_flags -O2 -I$prefix/include"
export CXXFLAGS="$base_flags -O2 -I$prefix/include"
export LDFLAGS="$base_flags -L$prefix/lib"

step() {
	echo "==> [$arch] $*"
}

# fail prints the tail of a library's log and stops the build.
fail() {
	echo "build-ffmpeg: $1 failed, last lines of $logs/$1.log:" >&2
	tail -n 40 "$logs/$1.log" >&2 || true
	exit 1
}

# Each source line is: name, version, license, url, sha256, tab-separated. Bash 3.2 has no
# associative arrays, so the fields sit in parallel arrays indexed like names.
names=() versions=() urls=() sums=() files=()
while IFS=$'\t' read -r n v _ u s; do
	case "$n" in "" | \#*) continue ;; esac
	case "$u" in
	*.tar.xz) f="$n-$v.tar.xz" ;;
	*.tar.bz2) f="$n-$v.tar.bz2" ;;
	*.tar.gz) f="$n-$v.tar.gz" ;;
	*)
		echo "build-ffmpeg: $n: unknown archive type in $u" >&2
		exit 2
		;;
	esac
	names+=("$n") versions+=("$v") urls+=("$u") sums+=("$s") files+=("$f")
done <"$sources"

# Fetch and verify every tarball before any build starts.
for i in "${!names[@]}"; do
	n="${names[$i]}"
	f="$src/${files[$i]}"
	if [ ! -f "$f" ]; then
		step "download $n ${versions[$i]}"
		curl -fL --retry 3 --silent --show-error -o "$f.part" "${urls[$i]}"
		mv "$f.part" "$f"
	fi
	got="$(shasum -a 256 "$f" | cut -d' ' -f1)"
	if [ "$got" != "${sums[$i]}" ]; then
		echo "build-ffmpeg: $n: SHA-256 mismatch for ${files[$i]}" >&2
		echo "  want ${sums[$i]}" >&2
		echo "  got  $got" >&2
		exit 1
	fi
done
step "all ${#names[@]} tarballs verified"
printf '%s\n' "${files[@]}" >"$src/MANIFEST"
if [ "$fetch_only" = 1 ]; then
	exit 0
fi
mkdir -p "$prefix" "$work" "$logs" "$done_dir"

meson_cross="$work/meson-cross.ini"
cat >"$meson_cross" <<INI
[binaries]
c = 'clang'
cpp = 'clang++'
objc = 'clang'
ar = 'ar'
strip = 'strip'
pkg-config = 'pkg-config'

[built-in options]
c_args = ['-arch', '$arch', '-mmacosx-version-min=14.0', '-I$prefix/include']
cpp_args = ['-arch', '$arch', '-mmacosx-version-min=14.0', '-I$prefix/include']
objc_args = ['-arch', '$arch', '-mmacosx-version-min=14.0']
c_link_args = ['-arch', '$arch', '-mmacosx-version-min=14.0', '-L$prefix/lib']
cpp_link_args = ['-arch', '$arch', '-mmacosx-version-min=14.0', '-L$prefix/lib']

[properties]
pkg_config_libdir = ['$prefix/lib/pkgconfig']

[host_machine]
system = 'darwin'
cpu_family = '$meson_cpu_family'
cpu = '$arch'
endian = 'little'
INI

# unpack extracts a fresh copy of a tarball into $work/<name> and prints that path.
unpack() {
	local d="$work/$1"
	rm -rf "$d"
	mkdir -p "$d"
	tar -xf "$src/$2" -C "$d" --strip-components=1
	echo "$d"
}

# built reports whether a library's marker matches the one this recipe would write.
built() {
	[ -f "$done_dir/$1" ] && [ "$(cat "$done_dir/$1")" = "$2" ]
}

autotools() {
	local host_args=()
	[ "$cross" = 1 ] && host_args=(--host="$triple")
	./configure --prefix="$prefix" --disable-shared --enable-static ${host_args[@]+"${host_args[@]}"} "$@"
	make -j"$jobs"
	make install
}

cmake_build() {
	local srcdir="$1"
	shift
	local cross_args=()
	[ "$cross" = 1 ] && cross_args=(-DCMAKE_SYSTEM_NAME=Darwin -DCMAKE_SYSTEM_PROCESSOR="$arch")
	cmake -S "$srcdir" -B "$srcdir/_build" -G Ninja \
		-DCMAKE_BUILD_TYPE=Release \
		-DCMAKE_INSTALL_PREFIX="$prefix" \
		-DCMAKE_INSTALL_LIBDIR=lib \
		-DCMAKE_PREFIX_PATH="$prefix" \
		-DCMAKE_IGNORE_PREFIX_PATH="/opt/homebrew;/usr/local" \
		-DCMAKE_FIND_FRAMEWORK=LAST \
		-DCMAKE_OSX_ARCHITECTURES="$arch" \
		-DCMAKE_OSX_DEPLOYMENT_TARGET=14.0 \
		-DCMAKE_POLICY_VERSION_MINIMUM=3.5 \
		-DBUILD_SHARED_LIBS=OFF \
		${cross_args[@]+"${cross_args[@]}"} "$@"
	cmake --build "$srcdir/_build" -j "$jobs"
	cmake --install "$srcdir/_build"
}

meson_build() {
	local cross_args=()
	[ "$cross" = 1 ] && cross_args=(--cross-file "$meson_cross")
	meson setup _build --prefix="$prefix" --libdir=lib --buildtype=release \
		--default-library=static ${cross_args[@]+"${cross_args[@]}"} "$@"
	meson compile -C _build -j "$jobs"
	meson install -C _build
}

build_libogg() { autotools; }

build_libvorbis() {
	sed -i '' 's/-force_cpusubtype_ALL//g' configure
	autotools --with-ogg="$prefix" --disable-oggtest --disable-docs --disable-examples
}

build_libtheora() {
	autotools --with-ogg="$prefix" --disable-oggtest --disable-vorbistest --disable-sdltest \
		--disable-examples --disable-doc
}

build_opus() { autotools --disable-doc --disable-extra-programs; }

build_lame() { autotools --disable-frontend --disable-gtktest --disable-decoder; }

build_opencore_amr() { autotools; }

build_freetype() {
	autotools --with-zlib=no --with-bzip2=no --with-png=no --with-harfbuzz=no --with-brotli=no
}

build_fribidi() { meson_build -Ddocs=false -Dbin=false -Dtests=false; }

build_harfbuzz() {
	meson_build -Dcoretext=enabled -Dfreetype=enabled -Dglib=disabled -Dgobject=disabled \
		-Dcairo=disabled -Dchafa=disabled -Dicu=disabled -Dgraphite2=disabled -Dpng=disabled \
		-Dzlib=disabled -Draster=disabled -Dvector=disabled -Dgpu=disabled -Dgpu_demo=disabled \
		-Dsubset=disabled -Dtests=disabled -Ddocs=disabled -Dutilities=disabled \
		-Dbenchmark=disabled -Dintrospection=disabled
}

build_libass() {
	autotools --disable-fontconfig --disable-libunibreak --disable-require-system-font-provider
}

build_libwebp() {
	autotools --enable-libwebpmux --enable-libwebpdemux --disable-gl --disable-sdl \
		--disable-png --disable-jpeg --disable-tiff --disable-gif --disable-wic
}

build_brotli() { cmake_build "$PWD" -DBROTLI_DISABLE_TESTS=ON; }

build_highway() {
	cmake_build "$PWD" -DHWY_ENABLE_TESTS=OFF -DHWY_ENABLE_EXAMPLES=OFF -DHWY_ENABLE_CONTRIB=OFF \
		-DBUILD_TESTING=OFF
}

build_lcms2() { autotools --without-jpeg --without-tiff; }

build_libjxl() {
	cmake_build "$PWD" -DBUILD_TESTING=OFF \
		-DJPEGXL_FORCE_SYSTEM_BROTLI=ON -DJPEGXL_FORCE_SYSTEM_HWY=ON -DJPEGXL_FORCE_SYSTEM_LCMS2=ON \
		-DJPEGXL_ENABLE_SKCMS=OFF -DJPEGXL_ENABLE_SJPEG=OFF -DJPEGXL_ENABLE_OPENEXR=OFF \
		-DJPEGXL_BUNDLE_LIBPNG=OFF -DJPEGXL_ENABLE_TOOLS=OFF -DJPEGXL_ENABLE_EXAMPLES=OFF \
		-DJPEGXL_ENABLE_BENCHMARK=OFF -DJPEGXL_ENABLE_JNI=OFF -DJPEGXL_ENABLE_PLUGINS=OFF \
		-DJPEGXL_ENABLE_DOXYGEN=OFF -DJPEGXL_ENABLE_MANPAGES=OFF -DJPEGXL_ENABLE_DEVTOOLS=OFF \
		-DJPEGXL_ENABLE_FUZZERS=OFF -DJPEGXL_ENABLE_VIEWERS=OFF
}

build_libvpx() {
	./configure --prefix="$prefix" --target="$vpx_target" --enable-static --disable-shared \
		--disable-examples --disable-tools --disable-docs --disable-unit-tests \
		--enable-vp9-highbitdepth --enable-pic
	make -j"$jobs"
	make install
}

build_x264() {
	local host_args=()
	[ "$cross" = 1 ] && host_args=(--host="$triple")
	./configure --prefix="$prefix" --enable-static --enable-pic --disable-cli --bit-depth=all \
		--disable-lavf --disable-swscale --disable-ffms --disable-gpac --disable-lsmash \
		${host_args[@]+"${host_args[@]}"}
	make -j"$jobs"
	make install-lib-static
}

# build_x265 links 8-bit, 10-bit, and 12-bit encoders into one static library.
build_x265() {
	local common=(-DENABLE_SHARED=OFF -DENABLE_CLI=OFF)
	[ "$arch" = arm64 ] && common+=(-DENABLE_SVE=OFF -DENABLE_SVE2=OFF -DENABLE_SVE2_BITPERM=OFF)
	local cross_args=()
	[ "$cross" = 1 ] && cross_args=(-DCMAKE_SYSTEM_NAME=Darwin -DCMAKE_SYSTEM_PROCESSOR="$arch")
	local cm=(-G Ninja -DCMAKE_BUILD_TYPE=Release -DCMAKE_INSTALL_PREFIX="$prefix"
		-DCMAKE_INSTALL_LIBDIR=lib -DCMAKE_IGNORE_PREFIX_PATH="/opt/homebrew;/usr/local"
		-DCMAKE_OSX_ARCHITECTURES="$arch" -DCMAKE_OSX_DEPLOYMENT_TARGET=14.0
		-DCMAKE_POLICY_VERSION_MINIMUM=3.5 ${cross_args[@]+"${cross_args[@]}"} "${common[@]}")
	local depth
	for depth in 12 10; do
		local extra=(-DHIGH_BIT_DEPTH=ON -DEXPORT_C_API=OFF)
		[ "$depth" = 12 ] && extra+=(-DMAIN12=ON)
		if ! { cmake -S source -B "b$depth" "${cm[@]}" "${extra[@]}" &&
			cmake --build "b$depth" -j "$jobs"; }; then
			echo "x265: $depth-bit failed with assembly, retrying with ENABLE_ASSEMBLY=OFF"
			rm -rf "b$depth"
			cmake -S source -B "b$depth" "${cm[@]}" "${extra[@]}" -DENABLE_ASSEMBLY=OFF
			cmake --build "b$depth" -j "$jobs"
		fi
	done
	mkdir -p b8
	cp b10/libx265.a b8/libx265_main10.a
	cp b12/libx265.a b8/libx265_main12.a
	cmake -S source -B b8 "${cm[@]}" -DEXTRA_LIB="x265_main10.a;x265_main12.a" \
		-DEXTRA_LINK_FLAGS=-L. -DLINKED_10BIT=ON -DLINKED_12BIT=ON
	cmake --build b8 -j "$jobs"
	cmake --install b8
	(cd b8 && libtool -static -o libx265_all.a libx265.a libx265_main10.a libx265_main12.a)
	cp b8/libx265_all.a "$prefix/lib/libx265.a"
}

build_svt_av1() { cmake_build "$PWD" -DBUILD_APPS=OFF -DBUILD_TESTING=OFF; }

# build_ffmpeg configures FFmpeg twice out of tree: the full build installs the libraries and
# ffmpeg, and a probe-only build makes an ffprobe without encoders, muxers, or filters, which it
# never uses and which would otherwise double its size.
build_ffmpeg() {
	local cross_args=()
	[ "$cross" = 1 ] && cross_args=(--enable-cross-compile)
	local common=(--prefix="$prefix" --cc=clang --cxx=clang++ --arch="$ff_arch" --target-os=darwin
		${cross_args[@]+"${cross_args[@]}"}
		--pkg-config=pkg-config --pkg-config-flags=--static
		--extra-cflags="$CFLAGS" --extra-cxxflags="$CXXFLAGS" --extra-ldflags="$LDFLAGS"
		--extra-libs="-lc++ -liconv"
		--enable-gpl --enable-version3 --enable-static --disable-shared
		--disable-doc --disable-debug --disable-ffplay --disable-autodetect
		--enable-videotoolbox --enable-audiotoolbox --enable-zlib --enable-bzlib --enable-iconv
		--enable-libvpx --enable-libopus --enable-libvorbis --enable-libopencore-amrnb
		--enable-libopencore-amrwb --enable-libjxl)
	mkdir -p build-full build-probe
	(cd build-full && ../configure "${common[@]}" --disable-ffprobe \
		--enable-libx264 --enable-libx265 --enable-libsvtav1 --enable-libmp3lame \
		--enable-libtheora --enable-libass --enable-libfreetype --enable-libfribidi \
		--enable-libharfbuzz --enable-libwebp &&
		make -j"$jobs" && make install)
	(cd build-probe && ../configure "${common[@]}" --disable-ffmpeg \
		--disable-encoders --disable-muxers --disable-filters --disable-outdevs &&
		make -j"$jobs" ffprobe && install -m 755 ffprobe "$prefix/bin/ffprobe")
}

# A library's marker holds its tarball sum, a hash of its build function, and a hash of the shared
# helpers and flags, so a restored cache rebuilds exactly what the recipe changed. The ffmpeg
# marker also covers the whole script and sources table. Lines are in build order, so once one
# library rebuilds, every later one rebuilds against it.
hash() { shasum -a 256 | cut -d' ' -f1; }
recipe="$(cat "$script" "$sources" | hash)"
helpers="$({
	declare -f unpack autotools cmake_build meson_build
	cat "$meson_cross"
	echo "$CFLAGS $CXXFLAGS $LDFLAGS"
} | hash)"
dirty=0
for i in "${!names[@]}"; do
	n="${names[$i]}"
	mark="${sums[$i]} $(declare -f "build_${n//-/_}" | hash) $helpers"
	[ "$n" = ffmpeg ] && mark="$mark $recipe"
	if [ "$dirty" = 0 ] && built "$n" "$mark"; then
		step "$n ${versions[$i]} already built"
		continue
	fi
	step "build $n ${versions[$i]}"
	start=$SECONDS
	dir="$(unpack "$n" "${files[$i]}")"
	# A subshell on the left of || runs with errexit off, so a failed configure would fall
	# through to make. Capture the status instead and keep errexit on inside.
	set +e
	(set -e; cd "$dir"; "build_${n//-/_}") >"$logs/$n.log" 2>&1
	rc=$?
	set -e
	[ "$rc" = 0 ] || fail "$n"
	echo "$mark" >"$done_dir/$n"
	dirty=1
	step "$n done in $((SECONDS - start))s"
done

# The bundled binaries may link only to system libraries and frameworks.
for bin in ffmpeg ffprobe; do
	path="$prefix/bin/$bin"
	bad="$(otool -L "$path" | tail -n +2 | awk '{print $1}' |
		grep -vE '^(/usr/lib/|/System/Library/)' || true)"
	if [ -n "$bad" ]; then
		echo "build-ffmpeg: $bin links non-system libraries:" >&2
		echo "$bad" >&2
		exit 1
	fi
	if ! lipo -archs "$path" | grep -qx "$arch"; then
		echo "build-ffmpeg: $bin is $(lipo -archs "$path"), want $arch" >&2
		exit 1
	fi
done
step "ffmpeg and ffprobe ready in $prefix/bin"
