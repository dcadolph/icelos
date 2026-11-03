#!/bin/sh
# Assemble an unsigned Icelos.app for local runs in mac/.build/Icelos.app.
# ffmpeg is not bundled yet, so the app uses the Homebrew copy.
set -eu
here="$(cd "$(dirname "$0")" && pwd)"
version="${ICELOS_VERSION:-dev}"
app="$here/.build/Icelos.app"

swift build -c release --package-path "$here"
bin="$(swift build -c release --package-path "$here" --show-bin-path)"

rm -rf "$app"
mkdir -p "$app/Contents/MacOS" "$app/Contents/Helpers" "$app/Contents/Resources"
cp "$bin/Icelos" "$app/Contents/MacOS/Icelos"
(cd "$here/.." && go build -trimpath -ldflags "-s -w -X github.com/dcadolph/icelos/cmd.version=$version" \
	-o "$app/Contents/Helpers/icelos" .)

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
	<key>NSHighResolutionCapable</key><true/>
	<key>NSHumanReadableCopyright</key><string>Free software under the GNU GPL v3 or later.</string>
</dict>
</plist>
PLIST
echo "$app"
