// Package update checks GitHub for a newer release of the app (TODO item
// 64), and tells whether Homebrew manages the running copy (items 64, 65).
//
// It only finds out and links to the release: the app never downloads or
// installs anything itself.
package update

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Repo is the project on GitHub, whose releases are checked.
const Repo = "alron/llama-swap-mac-launcher"

// LatestURL is GitHub's API for the newest release that isn't a draft or a
// pre-release.
const LatestURL = "https://api.github.com/repos/" + Repo + "/releases/latest"

// Version is a release's major, minor and patch numbers.
type Version [3]int

// ParseVersion reads "0.3.4", or a tag such as "v0.3.4": three numbers and
// nothing else, so a tag in any other form isn't taken for a release.
func ParseVersion(s string) (Version, bool) {
	parts := strings.Split(strings.TrimPrefix(s, "v"), ".")
	if len(parts) != 3 {
		return Version{}, false
	}
	var v Version
	for i, p := range parts {
		if p == "" || strings.Trim(p, "0123456789") != "" || len(p) > 6 {
			return Version{}, false
		}
		v[i], _ = strconv.Atoi(p)
	}
	return v, true
}

func (v Version) String() string { return fmt.Sprintf("%d.%d.%d", v[0], v[1], v[2]) }

// Newer reports whether v is a later version than w.
func (v Version) Newer(w Version) bool {
	for i := range v {
		if v[i] != w[i] {
			return v[i] > w[i]
		}
	}
	return false
}

// ReleaseURL is v's page on GitHub. It's built from the version, not taken
// from GitHub's answer, so a tampered answer can't send the user elsewhere.
func ReleaseURL(v Version) string {
	return "https://github.com/" + Repo + "/releases/tag/v" + v.String()
}

// Latest asks url (LatestURL, or a stand-in in tests) for the newest
// release. client should name the app in its requests' User-Agent, which
// GitHub's API wants.
func Latest(ctx context.Context, client *http.Client, url string) (Version, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Version{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	resp, err := client.Do(req)
	if err != nil {
		return Version{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Version{}, fmt.Errorf("GitHub answered HTTP %d", resp.StatusCode)
	}
	var release struct {
		Tag        string `json:"tag_name"`
		Draft      bool   `json:"draft"`
		Prerelease bool   `json:"prerelease"`
	}
	// A release's JSON is a few kilobytes; this bounds a misbehaving answer.
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&release); err != nil {
		return Version{}, fmt.Errorf("GitHub's answer didn't read: %w", err)
	}
	if release.Draft || release.Prerelease {
		return Version{}, errors.New("GitHub's latest release is a draft or a pre-release")
	}
	v, ok := ParseVersion(release.Tag)
	if !ok {
		// Quoted, and cut to 40 characters before it's quoted: the error
		// goes into an alert, the log and update.json, and the answer
		// chooses the text (TODO item 86).
		return Version{}, fmt.Errorf("the latest release's tag, %.40q, isn't a version", release.Tag)
	}
	return v, nil
}

// Every is how often the app checks, and Retry how soon it tries again
// after a check that failed (offline, GitHub down, its rate limit).
const (
	Every = 24 * time.Hour
	Retry = time.Hour
)

// State is what the app keeps between checks, beside its preferences.
type State struct {
	// Checked is when the last check was made; Failed, why it failed.
	Checked time.Time `json:"checked"`
	Failed  string    `json:"failed,omitempty"`
	// Latest is the newest release the last good check found.
	Latest string `json:"latest,omitempty"`
	// Notified is the newest release a notification has been posted for,
	// so each gets one.
	Notified string `json:"notified,omitempty"`
}

// Due reports whether a daily check is due at now.
func (s State) Due(now time.Time) bool {
	if s.Checked.IsZero() || now.Before(s.Checked) { // never, or the clock went back
		return true
	}
	wait := Every
	if s.Failed != "" {
		wait = Retry
	}
	return !now.Before(s.Checked.Add(wait))
}

// Available returns the newer release the last good check found, if
// there's one newer than current.
func (s State) Available(current string) (Version, bool) {
	latest, ok := ParseVersion(s.Latest)
	cur, okCur := ParseVersion(current)
	if !ok || !okCur || !latest.Newer(cur) {
		return Version{}, false
	}
	return latest, true
}

// Load reads the state from path; a missing or broken file is no checks.
func Load(path string) State {
	var s State
	b, err := os.ReadFile(path) // #nosec G304 -- the app's own state file
	if err == nil {
		_ = json.Unmarshal(b, &s) // broken: start again, as if never checked
	}
	return s
}

// Save writes the state to path, readable by the user alone.
func (s State) Save(path string) error {
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o600)
}

// Cask is the Homebrew cask that installs the app, in our tap.
const Cask = "llama-swap-launcher"

// Caskroom is where Homebrew keeps what it installed, on Apple Silicon,
// the only Macs the app runs on (Intel Homebrew's /usr/local can't install
// the cask, and other prefixes are ones Homebrew doesn't support). A var
// only so a test build can point it elsewhere (-ldflags -X).
var Caskroom = "/opt/homebrew/Caskroom"

// HomebrewManaged reports whether Homebrew manages the app at app, a
// bundle's path: each version's folder in the cask's Caskroom holds a link
// to the app it installed. Homebrew being installed isn't enough, and no
// brew command is run, as it's slow and can update itself.
func HomebrewManaged(caskroom, app string) bool {
	self, err := filepath.EvalSymlinks(app)
	if err != nil {
		return false
	}
	links, _ := filepath.Glob(filepath.Join(caskroom, Cask, "*", filepath.Base(app)))
	for _, link := range links {
		if target, err := filepath.EvalSymlinks(link); err == nil && target == self {
			return true
		}
	}
	return false
}

// InstalledVersion is the version of the app at bundle as it is on disk
// now, from its Info.plist (TODO item 65): after Homebrew, say, has
// installed a newer one over the running copy, it's the new copy's.
func InstalledVersion(bundle string) (Version, error) {
	s, err := PlistString(filepath.Join(bundle, "Contents", "Info.plist"), "CFBundleShortVersionString")
	if err != nil {
		return Version{}, err
	}
	v, ok := ParseVersion(s)
	if !ok {
		return Version{}, fmt.Errorf("the installed version, %q, isn't three numbers", s)
	}
	return v, nil
}

// PlistString reads key's string from an XML property list. It's not a
// full plist reader: enough for an app's Info.plist.
func PlistString(path, key string) (string, error) {
	f, err := os.Open(path) // #nosec G304 -- an app bundle's Info.plist
	if err != nil {
		return "", err
	}
	defer f.Close()
	d := xml.NewDecoder(io.LimitReader(f, 1<<20))
	var lastKey, text string
	for {
		tok, err := d.Token()
		if err == io.EOF {
			return "", fmt.Errorf("%s has no %s", path, key)
		}
		if err != nil {
			return "", fmt.Errorf("%s: %w", path, err)
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
				if lastKey == key {
					return text, nil
				}
				lastKey = ""
			default:
				lastKey = ""
			}
		}
	}
}

// Restarted is what the app writes before it quits to restart into a
// newer version (TODO item 65), and the new version reads at launch: that
// it was an update, from which version, and whether inference was paused,
// which an update restart keeps (the maintainer: the user didn't choose
// to restart).
type Restarted struct {
	From   string `json:"from"`
	To     string `json:"to"`
	Paused bool   `json:"paused,omitempty"`
}

// Save writes r to path, readable by the user alone.
func (r Restarted) Save(path string) error {
	b, err := json.Marshal(r)
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o600)
}

// TakeRestarted reads the marker at path and removes it, so it counts for
// one launch; ok is false when there's none.
func TakeRestarted(path string) (r Restarted, ok bool) {
	b, err := os.ReadFile(path) // #nosec G304 -- the app's own state file
	if err != nil {
		return r, false
	}
	_ = os.Remove(path) // best effort; read once either way
	return r, json.Unmarshal(b, &r) == nil
}
