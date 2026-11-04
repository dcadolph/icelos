#!/bin/sh
# Assemble an unsigned Icelos.app in mac/.build/Icelos.app.
#
# ICELOS_ARCHS lists the architectures to build (default "arm64 x86_64"). The Swift app and the
# Go helper are built for each and joined into universal binaries. FFmpeg and ffprobe come from
# mac/.build/ffmpeg/<arch>/bin (see build-ffmpeg.sh). When any listed arch lacks them, they are
# left out and the app falls back to a Homebrew ffmpeg.
set -eu
here="$(cd "$(dirname "$0")" && pwd)"
version="${ICELOS_VERSION:-dev}"
archs="${ICELOS_ARCHS:-arm64 x86_64}"
app="$here/.build/Icelos.app"
helpers="$app/Contents/Helpers"

swift_args="-c release --package-path $here"
for a in $archs; do
	swift_args="$swift_args --arch $a"
done
# shellcheck disable=SC2086 # Word splitting builds the argument list.
swift build $swift_args
# shellcheck disable=SC2086
bin="$(swift build $swift_args --show-bin-path)"

rm -rf "$app"
mkdir -p "$app/Contents/MacOS" "$helpers" "$app/Contents/Resources"
cp "$bin/Icelos" "$app/Contents/MacOS/Icelos"

slices=""
for a in $archs; do
	case "$a" in
	arm64) goarch=arm64 ;;
	x86_64) goarch=amd64 ;;
	*)
		echo "bundle: unknown arch $a" >&2
		exit 2
		;;
	esac
	out="$here/.build/icelos-$a"
	(cd "$here/.." && CGO_ENABLED=0 GOOS=darwin GOARCH="$goarch" go build -trimpath \
		-ldflags "-s -w -X github.com/dcadolph/icelos/cmd.version=$version" -o "$out" .)
	slices="$slices $out"
done
# shellcheck disable=SC2086
lipo -create $slices -output "$helpers/icelos"

have_ffmpeg=1
for a in $archs; do
	for tool in ffmpeg ffprobe; do
		[ -x "$here/.build/ffmpeg/$a/bin/$tool" ] || have_ffmpeg=0
	done
done
if [ "$have_ffmpeg" = 1 ]; then
	for tool in ffmpeg ffprobe; do
		tool_slices=""
		for a in $archs; do
			tool_slices="$tool_slices $here/.build/ffmpeg/$a/bin/$tool"
		done
		# shellcheck disable=SC2086
		lipo -create $tool_slices -output "$helpers/$tool"
	done
	"$here/third-party.sh" "$version" >"$app/Contents/Resources/THIRD_PARTY.md"
else
	echo "bundle: no ffmpeg in mac/.build/ffmpeg for every arch in \"$archs\";" \
		"leaving it out, the app will use a Homebrew ffmpeg" >&2
fi

# Build the app icon from assets/icon.png. The source is only read.
iconset="$here/.build/Icelos.iconset"
rm -rf "$iconset"
mkdir -p "$iconset"
for spec in 16:icon_16x16 32:icon_16x16@2x 32:icon_32x32 64:icon_32x32@2x \
	128:icon_128x128 256:icon_128x128@2x 256:icon_256x256 512:icon_256x256@2x \
	512:icon_512x512 1024:icon_512x512@2x; do
	sips -z "${spec%%:*}" "${spec%%:*}" "$here/../assets/icon.png" \
		--out "$iconset/${spec#*:}.png" >/dev/null
done
iconutil -c icns "$iconset" -o "$here/.build/Icelos.icns"
cp "$here/.build/Icelos.icns" "$app/Contents/Resources/Icelos.icns"

# Compile the app accent color into Assets.car. Palette.systemAccent holds the same two colors,
# and PaletteTests keeps them in step. The first color is light, the second is dark.
catalog="$here/.build/Assets.xcassets"
rm -rf "$catalog"
mkdir -p "$catalog/AccentColor.colorset"
cat > "$catalog/Contents.json" <<JSON
{"info":{"author":"xcode","version":1}}
JSON
cat > "$catalog/AccentColor.colorset/Contents.json" <<JSON
{
  "colors" : [
    {
      "color" : {
        "color-space" : "srgb",
        "components" : { "red" : "0x8A", "green" : "0x6A", "blue" : "0x1E", "alpha" : "1.000" }
      },
      "idiom" : "universal"
    },
    {
      "appearances" : [ { "appearance" : "luminosity", "value" : "dark" } ],
      "color" : {
        "color-space" : "srgb",
        "components" : { "red" : "0xB0", "green" : "0x8A", "blue" : "0x32", "alpha" : "1.000" }
      },
      "idiom" : "universal"
    }
  ],
  "info" : { "author" : "xcode", "version" : 1 }
}
JSON
xcrun actool "$catalog" --compile "$app/Contents/Resources" --platform macosx \
	--minimum-deployment-target 14.0 --output-format human-readable-text \
	--output-partial-info-plist "$here/.build/assets.plist"

cat > "$app/Contents/Info.plist" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>CFBundleExecutable</key><string>Icelos</string>
	<key>CFBundleIdentifier</key><string>com.dcadolph.icelos</string>
	<key>CFBundleIconFile</key><string>Icelos</string>
	<key>CFBundleName</key><string>Icelos</string>
	<key>CFBundleDisplayName</key><string>Icelos</string>
	<key>CFBundlePackageType</key><string>APPL</string>
	<key>CFBundleShortVersionString</key><string>${version#v}</string>
	<key>CFBundleVersion</key><string>${version#v}</string>
	<key>LSMinimumSystemVersion</key><string>14.0</string>
	<key>LSApplicationCategoryType</key><string>public.app-category.video</string>
	<key>NSAccentColorName</key><string>AccentColor</string>
	<key>NSHighResolutionCapable</key><true/>
	<key>NSHumanReadableCopyright</key><string>Free software under the GNU GPL v3 or later.</string>
	<key>CFBundleDocumentTypes</key>
	<array>
		<dict>
			<key>CFBundleTypeName</key><string>Media</string>
			<key>CFBundleTypeRole</key><string>Viewer</string>
			<key>LSHandlerRank</key><string>Alternate</string>
			<key>LSItemContentTypes</key>
			<array>
				<string>public.movie</string>
				<string>public.audio</string>
				<string>public.audiovisual-content</string>
			</array>
		</dict>
	</array>
</dict>
</plist>
PLIST
echo "$app"
