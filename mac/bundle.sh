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

cat > "$app/Contents/Info.plist" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>CFBundleExecutable</key><string>Icelos</string>
	<key>CFBundleIdentifier</key><string>com.dcadolph.icelos</string>
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
