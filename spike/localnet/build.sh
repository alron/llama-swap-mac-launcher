#!/bin/sh
# Builds the milestone-0 spike as "Llama Swap Launcher Dev.app" in ./build
# and signs it with the Developer ID certificate. Every build embeds a new
# build stamp, so it's a genuinely different binary each time.
set -eu
cd "$(dirname "$0")"

BUNDLE_ID=com.my-wang.llama-swap-launcher.dev
APP_NAME="Llama Swap Launcher Dev"
EXE=llama-swap-launcher
IDENTITY="${SIGN_IDENTITY:-Developer ID Application: Dean Bailey (P56KW9H72P)}"
APP="build/${APP_NAME}.app"
STAMP=$(date +%Y%m%dT%H%M%S)

rm -rf build
mkdir -p "$APP/Contents/MacOS"
CGO_ENABLED=1 GOOS=darwin GOARCH=arm64 \
	go build -ldflags "-X main.buildStamp=$STAMP" -o "$APP/Contents/MacOS/$EXE" .
sed -e "s/@BUNDLE_ID@/$BUNDLE_ID/" -e "s/@APP_NAME@/$APP_NAME/" -e "s/@EXE@/$EXE/" \
	Info.plist.in > "$APP/Contents/Info.plist"
plutil -lint "$APP/Contents/Info.plist" >/dev/null

codesign --force --options runtime --timestamp=none --sign "$IDENTITY" "$APP"
codesign --verify --strict "$APP"

echo "built: $APP (build $STAMP)"
codesign -d -r- "$APP" 2>&1 | sed -n 's/^designated => /designated requirement: /p'
otool -l "$APP/Contents/MacOS/$EXE" | awk '/LC_UUID/ { getline; getline; print "Mach-O UUID: " $2 }'
