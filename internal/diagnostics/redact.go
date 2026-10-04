// Package diagnostics builds the zip that "Collect Diagnostics…" saves for
// bug reports, and redacts secrets from what goes in it.
//
// Redaction works line by line on every file, in layers, since people put
// keys in odd places (macros, cmd lines, comments, logs):
//
//  1. Known secrets: the app's own API keys and secret variables, from the
//     keychain, wherever their exact values appear, in any format.
//  2. llama-swap's own rules (its internal/config/redact.go): keys named
//     like credentials, and the list items under them; credential flags in
//     command lines; Authorization headers; NAME=value variables whose
//     names look secret.
//  3. Values that look like keys wherever they are: Hugging Face, OpenAI
//     style (widened: the app's generated keys are base64, with + and /),
//     GitHub, Slack and AWS patterns, and passwords inside URLs.
//
// "${env.NAME}" references stay: they're names, not secrets, and they help
// whoever reads the report.
package diagnostics

import (
	"archive/zip"
	"io"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Redacted replaces every secret.
const Redacted = "[REDACTED]"

// secretHint matches a key or variable name that suggests a secret. Its
// groups don't capture, so it can sit inside other patterns.
const secretHint = `(?:api_?key|token|secret|passw(?:or)?d|credential|bearer|auth)` // #nosec G101 -- a pattern that finds credentials, not one

var (
	// A YAML or JSON key named like a secret, and its value on the same
	// line: `apiKey: sk-…`, `"token": "…"`, `- password: …`.
	keyed = regexp.MustCompile(`(?i)^(\s*(?:-\s*)?["']?[\w.-]*` + secretHint + `[\w.-]*["']?\s*:\s*)(.+)$`)
	// A command-line flag naming a credential: --api-key sk-…, -token=…
	flagged = regexp.MustCompile(`(?i)(--?[a-z0-9][a-z0-9-]*(?:key|token|secret|password|auth)[a-z0-9-]*[=\s]+)(\S+)`)
	// An Authorization header's credentials.
	authorization = regexp.MustCompile(`(?i)(authorization["']?\s*[:=]\s*["']?(?:bearer|basic)\s+)([^\s"']+)`)
	// NAME=value where NAME looks secret: HF_TOKEN=hf_…
	variable = regexp.MustCompile(`(?i)\b([A-Z0-9_]*(?:key|token|secret|password|passwd|credential)[A-Z0-9_]*)=([^\s"',]+)`)
	// A password in a URL: https://user:pass@host
	urlPassword = regexp.MustCompile(`([a-z][a-z0-9+.-]*://[^/\s:@]+:)([^/\s@]+)(@)`)
	// Values that look like keys, wherever they are.
	tokens = regexp.MustCompile(`hf_[A-Za-z0-9]{16,}|sk-[A-Za-z0-9_+/=-]{20,}|gh[oprsu]_[A-Za-z0-9]{20,}|xox[baprs]-[A-Za-z0-9-]{10,}|AKIA[0-9A-Z]{16}`)
	// A reference to an environment variable, which isn't a secret.
	envRef = regexp.MustCompile(`^["']?\$\{env\.[A-Za-z0-9_]+\}["']?$`)
	hint   = regexp.MustCompile(`(?i)` + secretHint)
	// The items of an inline list: quoted strings, or bare words.
	listItem = regexp.MustCompile(`"[^"]*"|'[^']*'|[^,\[\]\s][^,\[\]]*`)
)

// Redactor removes secrets from text.
type Redactor struct {
	known []string // exact secret values, longest first
}

// NewRedactor returns a Redactor that also removes these exact values
// wherever they appear. Values shorter than 4 characters are ignored, as
// they'd match too much.
func NewRedactor(known ...string) *Redactor {
	r := &Redactor{}
	for _, k := range known {
		if len(k) >= 4 {
			r.known = append(r.known, k)
		}
	}
	// Longest first, so a secret that contains another goes as a whole.
	sort.Slice(r.known, func(i, j int) bool { return len(r.known[i]) > len(r.known[j]) })
	return r
}

// Text redacts text, line by line. It tracks YAML indentation, so that
// under a key named like a secret with nothing after it (apiKeys:, or
// secrets:), every list item and value is redacted too.
func (r *Redactor) Text(text string) string {
	lines := strings.Split(text, "\n")
	block := -1 // indentation of the secret-named key whose block this is
	for i, line := range lines {
		indent := len(line) - len(strings.TrimLeft(line, " \t"))
		trimmed := strings.TrimSpace(line)
		if block >= 0 && trimmed != "" && !strings.HasPrefix(trimmed, "#") {
			if indent > block || (indent == block && strings.HasPrefix(trimmed, "- ")) {
				lines[i] = line[:indent] + redactBlockLine(trimmed)
				lines[i] = r.Line(lines[i])
				continue
			}
			block = -1
		}
		if strings.HasSuffix(trimmed, ":") && hint.MatchString(trimmed) && !strings.HasPrefix(trimmed, "#") {
			block = indent
		}
		lines[i] = r.Line(line)
	}
	return strings.Join(lines, "\n")
}

// redactBlockLine redacts a line inside a secret-named block: a list
// item, or a key's value. References to environment variables stay.
func redactBlockLine(trimmed string) string {
	if item, ok := strings.CutPrefix(trimmed, "- "); ok {
		if envRef.MatchString(item) {
			return trimmed
		}
		return "- " + Redacted
	}
	if key, value, ok := strings.Cut(trimmed, ":"); ok && strings.TrimSpace(value) != "" && !envRef.MatchString(strings.TrimSpace(value)) {
		return key + ": " + Redacted
	}
	return trimmed
}

// Line redacts one line.
func (r *Redactor) Line(line string) string {
	for _, k := range r.known {
		line = strings.ReplaceAll(line, k, Redacted)
	}
	if m := keyed.FindStringSubmatchIndex(line); m != nil {
		value := strings.TrimSpace(line[m[4]:m[5]])
		switch {
		case value == "" || value == `""` || value == "''" || value == `"",` || value == Redacted || envRef.MatchString(value):
			// Nothing there, or nothing secret.
		case strings.HasPrefix(value, "["):
			// An inline list: each item, but not references.
			line = line[:m[4]] + listItem.ReplaceAllStringFunc(value, func(item string) string {
				if envRef.MatchString(strings.TrimSpace(item)) {
					return item
				}
				return Redacted
			})
		case strings.HasPrefix(value, "{"):
			// An inline map: the layers below catch what looks secret.
		default:
			line = line[:m[4]] + Redacted
		}
	}
	line = flagged.ReplaceAllString(line, "${1}"+Redacted)
	line = authorization.ReplaceAllString(line, "${1}"+Redacted)
	line = variable.ReplaceAllString(line, "${1}="+Redacted)
	line = urlPassword.ReplaceAllString(line, "${1}"+Redacted+"${3}")
	line = tokens.ReplaceAllString(line, Redacted)
	return line
}

// Anonymize replaces the home folder with "~", and the user's name
// wherever else it appears as a word (in another path, say). A name of
// fewer than 4 characters is left, as it would match too much.
func Anonymize(text, home, user string) string {
	if home != "" {
		text = strings.ReplaceAll(text, home, "~")
	}
	if len(user) >= 4 {
		text = regexp.MustCompile(`\b`+regexp.QuoteMeta(user)+`\b`).ReplaceAllString(text, "<user>")
	}
	return text
}

// File is one file in the zip.
type File struct {
	Name string
	Data []byte
}

// Zip writes files into a zip, inside a folder named dir, readable only by
// its owner once unzipped.
func Zip(w io.Writer, dir string, files []File) error {
	z := zip.NewWriter(w)
	for _, f := range files {
		h := &zip.FileHeader{Name: dir + "/" + f.Name, Method: zip.Deflate, Modified: time.Now()}
		h.SetMode(0o600)
		fw, err := z.CreateHeader(h)
		if err != nil {
			return err
		}
		if _, err := fw.Write(f.Data); err != nil {
			return err
		}
	}
	return z.Close()
}
