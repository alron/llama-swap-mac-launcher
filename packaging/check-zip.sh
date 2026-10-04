#!/bin/sh
# check-zip.sh ZIP APP_NAME
#
# Unpacks a release zip with /usr/bin/unzip, as people in a terminal do,
# and checks the app inside: no stray ._ files (which ditto writes for
# extended attributes unless told not to, and which break the bundle's
# seal), a valid signature, Gatekeeper's approval, and the stapled ticket.
set -eu
zip=$1
app=$2
dir=$(mktemp -d)
trap 'rm -rf "$dir"' EXIT
/usr/bin/unzip -q "$zip" -d "$dir"
stray=$(find "$dir" -name '._*' | wc -l | tr -d ' ')
if [ "$stray" != 0 ]; then
	echo "check-zip: $stray ._ files in the unzipped app" >&2
	exit 1
fi
codesign --verify --strict --deep "$dir/$app.app"
spctl --assess --type execute "$dir/$app.app"
xcrun stapler validate -q "$dir/$app.app"
echo "check-zip: unpacked with unzip, the app verifies, passes Gatekeeper and has its ticket"
