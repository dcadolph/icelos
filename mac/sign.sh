#!/bin/sh
# Sign Icelos.app with the hardened runtime and a secure timestamp, inside out.
#
# Usage: mac/sign.sh <app> [identity]
#
# The identity must be a certificate of team U7L4P2YHWJ. The script checks the certificate's
# OU before signing (exit 2 otherwise) and every signature's TeamIdentifier and runtime flag
# after (exit 3 otherwise). No entitlements are applied.
set -eu
team=U7L4P2YHWJ
app="${1:?usage: sign.sh <app> [identity]}"
identity="${2:-Developer ID Application: Douglas Adolph ($team)}"

subject="$(security find-certificate -c "$identity" -p 2>/dev/null |
	openssl x509 -noout -subject 2>/dev/null || true)"
case "$subject" in
*"OU=$team"* | *"OU = $team"*) ;;
*)
	echo "sign: \"$identity\" is not a team $team certificate; nothing signed" >&2
	echo "sign: subject: ${subject:-certificate not found}" >&2
	exit 2
	;;
esac

sign() {
	codesign --force --options runtime --timestamp --sign "$identity" "$1"
}

for helper in "$app"/Contents/Helpers/*; do
	sign "$helper"
done
sign "$app/Contents/MacOS/Icelos"
sign "$app"

codesign --verify --deep --strict --verbose=2 "$app"
for target in "$app"/Contents/Helpers/* "$app"; do
	info="$(codesign -dv --verbose=2 "$target" 2>&1)"
	if ! echo "$info" | grep -q "^TeamIdentifier=$team$"; then
		echo "sign: $target has the wrong TeamIdentifier" >&2
		echo "$info" >&2
		exit 3
	fi
	if ! echo "$info" | grep -q 'flags=0x10000(runtime)'; then
		echo "sign: $target lacks the hardened runtime" >&2
		echo "$info" >&2
		exit 3
	fi
	echo "$target: TeamIdentifier=$team, runtime"
done
