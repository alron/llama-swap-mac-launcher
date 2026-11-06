package update

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseVersion(t *testing.T) {
	for s, want := range map[string]string{
		"v0.3.4": "0.3.4", "0.3.4": "0.3.4", "v10.20.30": "10.20.30", "v1.02.3": "1.2.3",
	} {
		v, ok := ParseVersion(s)
		if !ok || v.String() != want {
			t.Errorf("%q: %v, %v; want %s", s, v, ok, want)
		}
	}
	// Anything but three numbers isn't a release.
	for _, s := range []string{"", "v", "0.3", "0.3.4.5", "v0.3.4-rc1", "0.3.x", "vv0.3.4", " 0.3.4", "0.3.4 ", "0..4", "-1.0.0",
		"0.3.4\n", "1234567.0.0", "dev", "0.0.0-test"} {
		if v, ok := ParseVersion(s); ok {
			t.Errorf("%q taken as %v", s, v)
		}
	}
}

func TestNewer(t *testing.T) {
	v := func(s string) Version { x, _ := ParseVersion(s); return x }
	for _, tt := range []struct {
		a, b string
		want bool
	}{
		{"0.3.4", "0.3.3", true}, {"0.4.0", "0.3.9", true}, {"1.0.0", "0.99.99", true}, {"0.3.10", "0.3.9", true},
		{"0.3.3", "0.3.3", false}, {"0.3.2", "0.3.3", false}, {"0.2.9", "0.3.0", false},
	} {
		if got := v(tt.a).Newer(v(tt.b)); got != tt.want {
			t.Errorf("%s newer than %s: %v", tt.a, tt.b, got)
		}
	}
}

func TestReleaseURL(t *testing.T) {
	v, _ := ParseVersion("v0.3.4")
	if got := ReleaseURL(v); got != "https://github.com/alron/llama-swap-mac-launcher/releases/tag/v0.3.4" {
		t.Errorf("got %s", got)
	}
}

// The request, against a stand-in for GitHub: it sends the API's headers
// and the client's User-Agent, reads only the tag, and refuses what isn't
// a plain release. The link never comes from the answer.
func TestLatest(t *testing.T) {
	var answer string
	status := http.StatusOK
	var got *http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r
		w.WriteHeader(status)
		w.Write([]byte(answer))
	}))
	defer srv.Close()
	client := &http.Client{Transport: userAgent{"llama-swap-launcher/0.3.3"}}
	ctx := context.Background()

	answer = `{"tag_name":"v0.3.4","draft":false,"prerelease":false,"html_url":"https://evil.example/","name":"v0.3.4"}`
	v, err := Latest(ctx, client, srv.URL)
	if err != nil || v.String() != "0.3.4" {
		t.Fatalf("got %v, %v", v, err)
	}
	if got.Header.Get("User-Agent") != "llama-swap-launcher/0.3.3" || got.Header.Get("Accept") != "application/vnd.github+json" {
		t.Errorf("headers %v", got.Header)
	}
	for name, a := range map[string]string{
		"a pre-release":  `{"tag_name":"v0.4.0","prerelease":true}`,
		"a draft":        `{"tag_name":"v0.4.0","draft":true}`,
		"a tag":          `{"tag_name":"nightly"}`,
		"no tag":         `{}`,
		"not JSON":       `<html>`,
		"a giant answer": `{"tag_name":"v0.4.0","x":"` + strings.Repeat("a", 2<<20) + `"}`,
	} {
		answer = a
		if v, err := Latest(ctx, client, srv.URL); err == nil {
			t.Errorf("%s: taken as %v", name, v)
		}
	}
	answer, status = `{"message":"API rate limit exceeded"}`, http.StatusForbidden
	if _, err := Latest(ctx, client, srv.URL); err == nil || !strings.Contains(err.Error(), "403") {
		t.Errorf("rate limited: %v", err)
	}
}

type userAgent struct{ ua string }

func (u userAgent) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("User-Agent", u.ua)
	return http.DefaultTransport.RoundTrip(r)
}

// Daily after a good check, an hour after a failed one, at once when never.
func TestDue(t *testing.T) {
	now := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)
	for _, tt := range []struct {
		name string
		s    State
		want bool
	}{
		{"never", State{}, true},
		{"23 hours ago", State{Checked: now.Add(-23 * time.Hour)}, false},
		{"24 hours ago", State{Checked: now.Add(-24 * time.Hour)}, true},
		{"failed 59 minutes ago", State{Checked: now.Add(-59 * time.Minute), Failed: "offline"}, false},
		{"failed an hour ago", State{Checked: now.Add(-time.Hour), Failed: "offline"}, true},
		{"in the future (the clock went back)", State{Checked: now.Add(time.Hour)}, true},
	} {
		if got := tt.s.Due(now); got != tt.want {
			t.Errorf("%s: due %v", tt.name, got)
		}
	}
}

func TestAvailable(t *testing.T) {
	for _, tt := range []struct {
		latest, current string
		want            string
	}{
		{"0.3.4", "0.3.3", "0.3.4"},
		{"0.3.3", "0.3.3", ""},
		{"0.3.3", "0.3.4", ""}, // a build newer than the latest release
		{"", "0.3.3", ""},
		{"0.3.4", "0.0.0-test", ""}, // a version that isn't three numbers can't be compared
	} {
		v, ok := State{Latest: tt.latest}.Available(tt.current)
		if got := map[bool]string{true: v.String(), false: ""}[ok]; got != tt.want {
			t.Errorf("latest %q, current %q: %q, want %q", tt.latest, tt.current, got, tt.want)
		}
	}
}

func TestStateFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "update.json")
	if s := Load(path); !s.Checked.IsZero() {
		t.Errorf("a missing file: %+v", s)
	}
	want := State{Checked: time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC), Latest: "0.3.4", Notified: "0.3.4"}
	if err := want.Save(path); err != nil {
		t.Fatal(err)
	}
	if got := Load(path); !got.Checked.Equal(want.Checked) || got.Latest != want.Latest || got.Notified != want.Notified {
		t.Errorf("got %+v", got)
	}
	if info, _ := os.Stat(path); info.Mode().Perm() != 0o600 {
		t.Errorf("mode %v", info.Mode())
	}
	os.WriteFile(path, []byte("{broken"), 0o600)
	if s := Load(path); !s.Checked.IsZero() {
		t.Errorf("a broken file: %+v", s)
	}
}

// Homebrew manages the copy its Caskroom links to, not another copy of the
// app, and not when there's no Caskroom.
func TestHomebrewManaged(t *testing.T) {
	dir := t.TempDir()
	app := filepath.Join(dir, "Applications", "Llama Swap Launcher.app")
	other := filepath.Join(dir, "Downloads", "Llama Swap Launcher.app")
	dev := filepath.Join(dir, "build", "Llama Swap Launcher Dev.app")
	for _, a := range []string{app, other, dev} {
		os.MkdirAll(a, 0o700)
	}
	caskroom := filepath.Join(dir, "Caskroom")
	if HomebrewManaged(caskroom, app) {
		t.Error("managed with no Caskroom")
	}
	os.MkdirAll(filepath.Join(caskroom, Cask, "0.3.2"), 0o700)
	os.Symlink(app, filepath.Join(caskroom, Cask, "0.3.2", "Llama Swap Launcher.app"))
	if !HomebrewManaged(caskroom, app) {
		t.Error("the linked copy isn't managed")
	}
	if HomebrewManaged(caskroom, other) || HomebrewManaged(caskroom, dev) {
		t.Error("another copy is managed")
	}
}

// The installed version comes from the bundle's Info.plist as it is now;
// one that isn't three numbers, or a missing bundle, isn't a version.
func TestInstalledVersion(t *testing.T) {
	app := filepath.Join(t.TempDir(), "Llama Swap Launcher.app")
	os.MkdirAll(filepath.Join(app, "Contents"), 0o700)
	write := func(v string) {
		os.WriteFile(filepath.Join(app, "Contents", "Info.plist"), []byte(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
	<key>CFBundleIdentifier</key><string>com.my-wang.llama-swap-launcher</string>
	<key>CFBundleShortVersionString</key><string>`+v+`</string>
	<key>CFBundleVersion</key><string>`+v+`</string>
</dict></plist>`), 0o600)
	}
	write("0.3.4")
	if v, err := InstalledVersion(app); err != nil || v.String() != "0.3.4" {
		t.Errorf("got %v, %v", v, err)
	}
	write("0.0.0-test")
	if _, err := InstalledVersion(app); err == nil {
		t.Error("0.0.0-test taken as a version")
	}
	if _, err := InstalledVersion(filepath.Join(t.TempDir(), "gone.app")); err == nil {
		t.Error("a missing bundle has a version")
	}
}

// The update restart's marker counts for one launch.
func TestRestartedMarker(t *testing.T) {
	path := filepath.Join(t.TempDir(), "restarted.json")
	if _, ok := TakeRestarted(path); ok {
		t.Error("a marker with none written")
	}
	want := Restarted{From: "0.3.3", To: "0.3.4", Paused: true}
	if err := want.Save(path); err != nil {
		t.Fatal(err)
	}
	if got, ok := TakeRestarted(path); !ok || got != want {
		t.Errorf("got %+v, %v", got, ok)
	}
	if _, ok := TakeRestarted(path); ok {
		t.Error("read twice")
	}
}
