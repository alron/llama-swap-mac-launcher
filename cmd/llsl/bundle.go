package main

import (
	"encoding/xml"
	"io"
	"os"
	"path/filepath"
)

// bundleID is the app's, compiled in by the Makefile. findBundleID prefers
// the app llsl is actually inside.
var bundleID = "com.my-wang.llama-swap-launcher.dev"

// findBundleID works out which app to talk to: $LLSL_BUNDLE_ID if set, else
// the app whose Contents/Helpers holds this binary (so a dev build's llsl
// talks to the dev app), else the compiled-in ID. Homebrew puts a symlink
// to llsl on PATH, so symlinks are followed first.
func findBundleID() string {
	if id := os.Getenv("LLSL_BUNDLE_ID"); id != "" {
		return id
	}
	if exe, err := os.Executable(); err == nil {
		if exe, err = filepath.EvalSymlinks(exe); err == nil {
			contents := filepath.Dir(filepath.Dir(exe)) // .../X.app/Contents/Helpers/llsl
			if filepath.Base(contents) == "Contents" {
				if id := plistBundleID(filepath.Join(contents, "Info.plist")); id != "" {
					return id
				}
			}
		}
	}
	return bundleID
}

// plistBundleID reads CFBundleIdentifier from an XML Info.plist, or
// returns "".
func plistBundleID(path string) string {
	f, err := os.Open(path) // #nosec G304 -- the Info.plist of the app llsl is inside
	if err != nil {
		return ""
	}
	defer f.Close()
	d := xml.NewDecoder(f)
	var lastKey, text string
	for {
		tok, err := d.Token()
		if err == io.EOF || err != nil {
			return ""
		}
		switch t := tok.(type) {
		case xml.StartElement:
			text = ""
		case xml.CharData:
			text += string(t)
		case xml.EndElement:
			switch t.Name.Local {
			case "key":
				lastKey = text
			case "string":
				if lastKey == "CFBundleIdentifier" {
					return text
				}
				lastKey = ""
			default:
				lastKey = ""
			}
		}
	}
}
