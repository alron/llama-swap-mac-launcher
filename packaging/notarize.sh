#!/bin/sh
# notarize.sh <zip> <app> <keychain-profile>
#
# Submits <zip> (a zip of <app>) to Apple for notarization and waits for the
# verdict. If Apple accepts it, staples the ticket to <app>, so Gatekeeper
# can check it offline; otherwise prints Apple's log of what was wrong.
# The keychain profile is the one saved with `xcrun notarytool
# store-credentials`.
set -eu
zip=$1 app=$2 profile=$3

out=$(mktemp)
trap 'rm -f "$out"' EXIT
# notarytool's exit status doesn't tell a rejection from a network failure,
# so read its verdict instead.
xcrun notarytool submit "$zip" --keychain-profile "$profile" --wait --output-format json >"$out" || true
if ! status=$(plutil -extract status raw -o - "$out" 2>/dev/null); then
	echo "notarize.sh: no verdict from notarytool:" >&2
	cat "$out" >&2
	exit 1
fi
id=$(plutil -extract id raw -o - "$out")
echo "notarization $id: $status"
if [ "$status" != "Accepted" ]; then
	xcrun notarytool log "$id" --keychain-profile "$profile" >&2
	exit 1
fi

xcrun stapler staple "$app"
xcrun stapler validate "$app"
