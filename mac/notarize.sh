#!/bin/sh
# Notarize a file with Apple and staple the ticket.
#
# Usage: mac/notarize.sh <file> [staple-target]
#
# With NOTARY_KEY_P8, NOTARY_KEY_ID, and NOTARY_ISSUER_ID set (CI), the App Store Connect key is
# used. Otherwise the sheepdog-notary keychain profile is. The ticket is stapled to
# staple-target, which defaults to the file itself; a zip cannot hold a ticket, so pass the app.
set -eu
file="${1:?usage: notarize.sh <file> [staple-target]}"
target="${2:-$file}"

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

if [ -n "${NOTARY_KEY_P8:-}" ]; then
	printf '%s' "$NOTARY_KEY_P8" >"$tmp/key.p8"
	set -- --key "$tmp/key.p8" --key-id "${NOTARY_KEY_ID:?NOTARY_KEY_ID is not set}" \
		--issuer "${NOTARY_ISSUER_ID:?NOTARY_ISSUER_ID is not set}"
else
	set -- --keychain-profile sheepdog-notary
fi

echo "notarize: submitting $(basename "$file")"
xcrun notarytool submit "$file" --wait --timeout 90m --output-format json "$@" >"$tmp/out.json" ||
	true
cat "$tmp/out.json"
echo
status="$(plutil -extract status raw -o - "$tmp/out.json" 2>/dev/null || true)"
if [ "$status" != Accepted ]; then
	echo "notarize: status is \"${status:-none}\", not Accepted" >&2
	id="$(plutil -extract id raw -o - "$tmp/out.json" 2>/dev/null || true)"
	[ -n "$id" ] && xcrun notarytool log "$id" "$@" >&2
	exit 1
fi

# Apple's ticket service drops connections now and then, so retry before failing.
try=1
until xcrun stapler staple "$target"; do
	if [ "$try" -ge 3 ]; then
		echo "notarize: stapling $target failed three times" >&2
		exit 1
	fi
	try=$((try + 1))
	sleep 20
done
