#!/bin/sh
# Build, sign, notarize, and verify the Icelos.app zip and DMG for one version.
#
# Usage: mac/release.sh <version>
#
# Output goes to ICELOS_DIST (default dist/ at the repo root): Icelos_v<version>_macos_universal
# .zip and .dmg, the fixed-name copies Icelos_macos_universal.zip and .dmg,
# Icelos_v<version>_ffmpeg_source.tar (the corresponding source for the bundled ffmpeg), and
# checksums-macos.txt. ICELOS_ARCHS is passed through to bundle.sh. The release refuses to run
# without the bundled ffmpeg.
set -eu
here="$(cd "$(dirname "$0")" && pwd)"
version="${1:?usage: release.sh <version>}"
version="${version#v}"
tag="v$version"
dist="${ICELOS_DIST:-$here/../dist}"
identity="Developer ID Application: Douglas Adolph (U7L4P2YHWJ)"
app="$here/.build/Icelos.app"
zip="$dist/Icelos_${tag}_macos_universal.zip"
dmg="$dist/Icelos_${tag}_macos_universal.dmg"
source_tar="$dist/Icelos_${tag}_ffmpeg_source.tar"

step() {
	echo "==> $*"
}

step "bundle $tag"
ICELOS_VERSION="$version" "$here/bundle.sh"
for tool in icelos ffmpeg ffprobe; do
	if [ ! -x "$app/Contents/Helpers/$tool" ]; then
		echo "release: $tool is missing from the bundle; build it with mac/build-ffmpeg.sh" >&2
		exit 1
	fi
done

step "sign"
"$here/sign.sh" "$app" "$identity"

mkdir -p "$dist"
rm -f "$zip" "$dmg"
step "notarize the app"
ditto -c -k --keepParent "$app" "$zip"
"$here/notarize.sh" "$zip" "$app"
rm -f "$zip"
ditto -c -k --keepParent "$app" "$zip"

step "build the disk image"
venv="$here/.build/dmgbuild"
if [ ! -x "$venv/bin/dmgbuild" ]; then
	python3 -m venv "$venv"
	"$venv/bin/pip" install --quiet dmgbuild==1.6.6
fi
"$venv/bin/dmgbuild" -s "$here/dmg.py" -D app="$app" Icelos "$dmg"
codesign --force --timestamp --sign "$identity" "$dmg"
codesign -dv "$dmg" 2>&1 | grep -q '^TeamIdentifier=U7L4P2YHWJ$'
step "notarize the disk image"
"$here/notarize.sh" "$dmg"

step "verify as a user would"
check="$(mktemp -d)"
mount="$check/mount"
trap 'hdiutil detach "$mount" >/dev/null 2>&1 || true; rm -rf "$check"' EXIT
ditto -x -k "$zip" "$check"
xcrun stapler validate "$check/Icelos.app"
spctl --assess --type execute --verbose=4 "$check/Icelos.app"
codesign --verify --deep --strict --verbose=2 "$check/Icelos.app"
xcrun stapler validate "$dmg"
spctl --assess --type open --context context:primary-signature --verbose=4 "$dmg"
mkdir -p "$mount"
hdiutil attach -nobrowse -readonly -mountpoint "$mount" "$dmg" >/dev/null
spctl --assess --type execute --verbose=4 "$mount/Icelos.app"
test "$(readlink "$mount/Applications")" = /Applications
hdiutil detach "$mount" >/dev/null

step "pack the ffmpeg corresponding source"
"$here/build-ffmpeg.sh" --fetch-only
stage="$check/source"
top="$stage/Icelos_${tag}_ffmpeg_source"
mkdir -p "$top/mac/.build/ffmpeg/src"
cp -- "$here/../LICENSE" "$top/"
cp -- "$here/build-ffmpeg.sh" "$here/ffmpeg-sources.txt" "$top/mac/"
while read -r f; do
	cp -- "$here/.build/ffmpeg/src/$f" "$top/mac/.build/ffmpeg/src/"
done <"$here/.build/ffmpeg/src/MANIFEST"
cat >"$top/README.txt" <<README
Corresponding source for the ffmpeg and ffprobe binaries inside Icelos.app $tag.

mac/.build/ffmpeg/src holds every source tarball listed in mac/ffmpeg-sources.txt. Running
mac/build-ffmpeg.sh arm64 or mac/build-ffmpeg.sh x86_64 from this folder verifies their SHA-256
sums and builds the binaries with no download. Prerequisites are listed at the top of the script.
The Icelos source is at https://github.com/dcadolph/icelos/tree/$tag.
README
rm -f "$source_tar"
COPYFILE_DISABLE=1 tar -cf "$source_tar" -C "$stage" "Icelos_${tag}_ffmpeg_source"

cd "$dist"
cp -- "$(basename "$zip")" Icelos_macos_universal.zip
cp -- "$(basename "$dmg")" Icelos_macos_universal.dmg
shasum -a 256 -- Icelos_*.zip Icelos_*.dmg "$(basename "$source_tar")" >checksums-macos.txt
cat checksums-macos.txt
step "done: $dist"
