# Build settings. BUNDLE_ID is permanent (see CLAUDE.md), so it's defined
# only here.
APP_NAME       := Llama Swap Launcher
BUNDLE_ID      := com.my-wang.llama-swap-launcher
EXE            := llama-swap-launcher
VERSION        ?= 0.2.0
MIN_MACOS      := 15.0
SIGN_IDENTITY  ?= Developer ID Application: Dean Bailey (P56KW9H72P)
NOTARY_PROFILE ?= my-wang
# Shown in the About panel. UTC, so it doesn't depend on where it's built.
BUILD_DATE     := $(shell date -u +%Y-%m-%d)

DEV_APP := build/$(APP_NAME) Dev.app
DEV_ID  := $(BUNDLE_ID).dev
REL_APP := build/release/$(APP_NAME).app
REL_ZIP := build/release/Llama-Swap-Launcher-$(VERSION).zip

export GOOS        := darwin
export GOARCH      := arm64
export CGO_ENABLED := 1
# Without these, cgo targets the build machine's macOS version, and the app
# won't launch on anything older than that.
export CGO_CFLAGS  := -O2 -g -mmacosx-version-min=$(MIN_MACOS)
export CGO_LDFLAGS := -mmacosx-version-min=$(MIN_MACOS)

.PHONY: dev run release icons test vet lint sec clean

# dev builds "Llama Swap Launcher Dev.app": the .dev bundle ID, signed with
# the Developer ID certificate, not notarized. The .app path contains
# spaces, so it can't be a make target; dev always rebuilds (go build's
# cache keeps that quick).
dev: build/AppIcon.icns
	$(call bundle,$(DEV_APP),$(DEV_ID),$(APP_NAME) Dev,--timestamp=none)

# The app icon, in every size macOS asks for: 16 and 32 pt from
# AppIcon-small.png (the centre llama alone), the rest from AppIcon.png.
build/AppIcon.icns: packaging/AppIcon.png packaging/AppIcon-small.png
	rm -rf build/AppIcon.iconset
	mkdir -p build/AppIcon.iconset
	for s in 16 32 128 256 512; do \
		src=packaging/AppIcon.png; [ $$s -le 32 ] && src=packaging/AppIcon-small.png; \
		sips -z $$s $$s $$src --out build/AppIcon.iconset/icon_$${s}x$${s}.png >/dev/null && \
		sips -z $$((s*2)) $$((s*2)) $$src --out build/AppIcon.iconset/icon_$${s}x$${s}@2x.png >/dev/null || exit 1; \
	done
	iconutil -c icns build/AppIcon.iconset -o $@

# icons regenerates both icon masters from the source art; see
# packaging/mkicon. Commit the results.
icons:
	cd packaging/mkicon && CGO_ENABLED=0 GOOS= GOARCH= go run . -src ../art/llama-swap-launcher.png -out ..

# run launches the dev app through LaunchServices, which is how it gets its
# own Local Network permission. Note that `open` passes this shell's
# environment to the app; a Finder launch doesn't.
run: dev
	open "$(DEV_APP)"

# release builds the app under the release bundle ID with a secure
# timestamp, has Apple notarize it, staples the ticket, and zips the result
# for distribution (the zip is zipped again after stapling, so it carries
# the ticket). It ends with Gatekeeper's own assessment and the zip's
# SHA-256, which a Homebrew cask needs.
release: build/AppIcon.icns
	$(call bundle,$(REL_APP),$(BUNDLE_ID),$(APP_NAME),--timestamp)
	rm -f "$(REL_ZIP)"
	ditto -c -k --norsrc --noextattr --keepParent "$(REL_APP)" "$(REL_ZIP)"
	packaging/notarize.sh "$(REL_ZIP)" "$(REL_APP)" "$(NOTARY_PROFILE)"
	rm -f "$(REL_ZIP)"
	# Without --norsrc --noextattr, ditto adds ._ files for extended
	# attributes, which unzip leaves in the bundle, breaking its seal.
	ditto -c -k --norsrc --noextattr --keepParent "$(REL_APP)" "$(REL_ZIP)"
	spctl --assess --type execute --verbose=2 "$(REL_APP)"
	packaging/check-zip.sh "$(REL_ZIP)" "$(APP_NAME)"
	shasum -a 256 "$(REL_ZIP)"

test:
	go test ./...

vet:
	go vet ./...

lint: vet
	staticcheck ./...

# sec runs gosec, which catches things vet and staticcheck don't. Run it at
# stable points; CLAUDE.md records which findings are accepted and why.
sec:
	gosec -quiet ./...

clean:
	rm -rf build

# bundle <app path>,<bundle id>,<display name>,<extra codesign flags>
# It refuses to replace an app that's running, whose bundle it would delete.
define bundle
	@if pgrep -f "$(1)/Contents/MacOS/" >/dev/null; then \
		echo "$(1) is running; quit it before rebuilding" >&2; exit 1; fi
	rm -rf "$(1)"
	mkdir -p "$(1)/Contents/MacOS" "$(1)/Contents/Helpers" "$(1)/Contents/Resources"
	cp build/AppIcon.icns "$(1)/Contents/Resources/AppIcon.icns"
	go build -trimpath -ldflags "-X main.bundleID=$(2) -X main.version=$(VERSION) -X main.buildDate=$(BUILD_DATE)" \
		-o "$(1)/Contents/MacOS/$(EXE)" ./cmd/llama-swap-launcher
	CGO_ENABLED=0 go build -trimpath -ldflags "-X main.bundleID=$(2) -X main.version=$(VERSION)" \
		-o "$(1)/Contents/Helpers/llsl" ./cmd/llsl
	sed -e 's/@BUNDLE_ID@/$(2)/' -e 's/@APP_NAME@/$(3)/' -e 's/@EXE@/$(EXE)/' \
		-e 's/@VERSION@/$(VERSION)/' -e 's/@MIN_MACOS@/$(MIN_MACOS)/' \
		packaging/Info.plist.in > "$(1)/Contents/Info.plist"
	plutil -lint -s "$(1)/Contents/Info.plist"
	codesign --force --options runtime $(4) --identifier $(2).llsl \
		--sign "$(SIGN_IDENTITY)" "$(1)/Contents/Helpers/llsl"
	codesign --force --options runtime $(4) --sign "$(SIGN_IDENTITY)" "$(1)"
	codesign --verify --strict "$(1)"
	@echo "built $(1)"
endef
