#!/bin/bash
# entitlements.sh <profile> <bundle id> <signing identity> <out.plist>
#
# Writes the entitlements to sign the app with, from its Developer ID
# provisioning profile (TODO item 60): its App ID, team, and time-sensitive
# notifications, if the profile grants them. An app claiming entitlements
# its embedded profile doesn't grant won't launch, so the profile is checked
# first: for this bundle ID, naming the signing certificate, not expired.
#
# Only what the app uses is claimed. The profile also allows the team's
# keychain-access-groups, which the app leaves alone: its secrets are in the
# login keychain, tied to its code signature (keychain.m).
set -euo pipefail

profile=$1 bundle_id=$2 identity=$3 out=$4
fail() {
	echo "entitlements: $profile: $*" >&2
	exit 1
}

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
security cms -D -i "$profile" >"$tmp/profile.plist" 2>/dev/null || fail "not a provisioning profile"
get() { plutil -extract "$1" raw -o - "$tmp/profile.plist" 2>/dev/null; }

team=$(get TeamIdentifier.0) || fail "no team"
app_id=$(get Entitlements.com\\.apple\\.application-identifier) || fail "no App ID"
[ "$app_id" = "$team.$bundle_id" ] || fail "it's for $app_id, not $team.$bundle_id"

# ISO 8601 in UTC, so the strings compare as times.
expires=$(get ExpirationDate)
[[ "$expires" > "$(date -u +%Y-%m-%dT%H:%M:%SZ)" ]] || fail "it expired on $expires"

# The certificate signing the app must be one the profile names.
cert=$(security find-identity -v -p codesigning | awk -v id="$identity" 'index($0, "\"" id "\"") { print $2; exit }')
[ -n "$cert" ] || fail "no signing identity \"$identity\" in the keychain"
found=
n=$(get DeveloperCertificates) || fail "no certificates"
for ((i = 0; i < n; i++)); do
	sha=$(get "DeveloperCertificates.$i" | base64 -d | shasum -a 1 | awk '{ print toupper($1) }')
	[ "$sha" = "$cert" ] && found=1
done
[ -n "$found" ] || fail "it doesn't name the signing certificate $cert; regenerate it with that certificate"

rm -f "$out"
args=(-c "Add :com.apple.application-identifier string $app_id"
	-c "Add :com.apple.developer.team-identifier string $team")
if [ "$(get Entitlements.com\\.apple\\.developer\\.usernotifications\\.time-sensitive)" = true ]; then
	args+=(-c "Add :com.apple.developer.usernotifications.time-sensitive bool true")
else
	echo "entitlements: $profile doesn't grant time-sensitive notifications; they'll post at the ordinary level" >&2
fi
/usr/libexec/PlistBuddy "${args[@]}" "$out" >/dev/null
plutil -lint -s "$out"
