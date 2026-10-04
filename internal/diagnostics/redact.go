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

// A name (a YAML or JSON key, a variable, a flag) holds a secret if it has
// a strong word anywhere in it, or ends with a weak one: "key" anywhere
// would catch model names that happen to contain it.
var (
	strongName = regexp.MustCompile(`(?i)token|secret|passw(?:or)?d|credential|api[-_]?key|authorization`) // #nosec G101 -- a pattern that finds credentials, not one
	// The weak words as the whole name or its last part: after a separator
	// (peer_key), in camel case (peerKey), or ending an all-capitals
	// variable (MYPASS).
	weakName = regexp.MustCompile(`(?i:(?:^|[_.-])(?:keys?|pass|pwd|auth|bearer))$|[a-z](?:Keys?|Pass|Pwd|Auth)$|^[A-Z0-9_]*[A-Z0-9](?:KEYS?|PASS|PWD)$`)
)

// secretName reports whether a key or variable name looks like it holds a
// secret: apiKey, x-api-key, HF_TOKEN, peer_key, MYPASS, pwd.
func secretName(name string) bool {
	name = strings.Trim(name, `"' `)
	return strongName.MatchString(name) || weakName.MatchString(name) || strings.EqualFold(name, "pass")
}

var (
	// A YAML key at the start of a line, commented out or not, and its
	// value: `apiKey: sk-…`, `- password: …`, `# token: old`.
	lineKey = regexp.MustCompile(`^(\s*(?:#+\s*)?(?:-\s+)?["']?)([\w.-]+)(["']?\s*:)(\s*)(.*)$`)
	// A quoted key anywhere, as in one-line JSON: {"api_key":"…"}.
	quotedKey = regexp.MustCompile(`(["'])([\w.-]+)(["']\s*:\s*)("[^"]*"|'[^']*'|[^\s,}\]]+)`)
	// NAME=value with no spaces, as in a shell or an env list: HF_TOKEN=…
	// (Spaces around "=" are llama-server's log style, n_tokens = 512, and
	// those numbers matter in a bug report.)
	variable = regexp.MustCompile(`\b([A-Za-z_][A-Za-z0-9_]*)=([^\s"',]+)`)
	// A command-line flag naming a credential: --api-key sk-…, -token=…
	// The flag starts a word: inside one (curl-user-pass) it isn't a flag.
	flagged = regexp.MustCompile(`(?i)((?:^|[\s"'])--?[a-z0-9][a-z0-9-]*(?:key|token|secret|pass(?:word)?|pwd|auth)[a-z0-9-]*[=\s]+)(\S+)`)
	// curl's -u / --user name:password.
	userPassword = regexp.MustCompile(`((?:^|\s)(?:-u|--user)[=\s]+["']?[^\s:"']+:)([^\s"']+)`)
	// A header with credentials, in a curl -H or a config: Authorization:
	// Bearer …, x-api-key: …
	header = regexp.MustCompile(`(?i)\b((?:proxy-)?authorization\s*:\s*["']?(?:(?:bearer|basic|token)\s+)?|(?:x-api-key|api-key|x-auth-token)\s*:\s*["']?)([^\s"',]+)`)
	// A password in a URL: https://user:pass@host
	urlPassword = regexp.MustCompile(`([a-z][a-z0-9+.-]*://[^/\s:@]+:)([^/\s@]+)(@)`)
	// Values shaped like keys, wherever they are: Hugging Face, OpenAI
	// style (with + / = for the app's own base64 keys), GitHub (classic and
	// fine-grained), GitLab, Slack, AWS, Google.
	tokens = regexp.MustCompile(`hf_[A-Za-z0-9]{16,}|sk-[A-Za-z0-9_+/=-]{20,}|gh[oprsu]_[A-Za-z0-9]{20,}|github_pat_[A-Za-z0-9_]{20,}|glpat-[A-Za-z0-9_-]{20,}|xox[baprs]-[A-Za-z0-9-]{10,}|AKIA[0-9A-Z]{16}|AIza[0-9A-Za-z_-]{35}`)
	// A reference to an environment variable, which isn't a secret.
	envRef = regexp.MustCompile(`^["']?\$\{env\.[A-Za-z0-9_]+\}["']?$`)
	// A YAML block scalar's indicator: | or >, with optional modifiers.
	blockScalar = regexp.MustCompile(`^[|>][-+0-9]*\s*(?:#.*)?$`)
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

// Text redacts text, line by line. A key named like a secret with nothing
// after it (apiKeys:, secrets:) or a block scalar (apiKey: |) starts a
// block, tracked by indentation, in which everything is redacted: list
// items, values, and a block scalar's lines.
func (r *Redactor) Text(text string) string {
	lines := strings.Split(text, "\n")
	block := -1 // indentation of the secret-named key whose block this is
	for i, line := range lines {
		indent := len(line) - len(strings.TrimLeft(line, " \t"))
		trimmed := strings.TrimSpace(line)
		if block >= 0 && trimmed != "" {
			if indent > block || (indent == block && strings.HasPrefix(trimmed, "- ")) {
				lines[i] = r.Line(line[:indent] + redactBlockLine(trimmed))
				continue
			}
			block = -1
		}
		if m := lineKey.FindStringSubmatch(line); m != nil && secretName(m[2]) {
			if value := strings.TrimSpace(m[5]); value == "" || blockScalar.MatchString(value) {
				block = indent
			}
		}
		lines[i] = r.Line(line)
	}
	return strings.Join(lines, "\n")
}

// redactBlockLine redacts a line inside a secret-named block: a list item,
// a key's value, or a block scalar's text. References to environment
// variables, and comments, stay.
func redactBlockLine(trimmed string) string {
	if item, ok := strings.CutPrefix(trimmed, "- "); ok {
		if envRef.MatchString(item) {
			return trimmed
		}
		return "- " + Redacted
	}
	if key, value, ok := strings.Cut(trimmed, ": "); ok && !strings.ContainsAny(key, " \t") {
		if v := strings.TrimSpace(value); v == "" || envRef.MatchString(v) {
			return trimmed
		}
		return key + ": " + Redacted
	}
	if strings.HasPrefix(trimmed, "#") {
		return trimmed
	}
	return Redacted
}

// Line redacts one line.
func (r *Redactor) Line(line string) string {
	for _, k := range r.known {
		line = strings.ReplaceAll(line, k, Redacted)
	}
	if m := lineKey.FindStringSubmatchIndex(line); m != nil && secretName(line[m[4]:m[5]]) {
		value := strings.TrimSpace(line[m[10]:m[11]])
		switch {
		case quiet(value) || blockScalar.MatchString(value):
		case strings.HasPrefix(value, "["):
			// An inline list: each item, but not references.
			line = line[:m[10]] + listItem.ReplaceAllStringFunc(value, func(item string) string {
				if envRef.MatchString(strings.TrimSpace(item)) {
					return item
				}
				return Redacted
			})
		case strings.HasPrefix(value, "{"):
			// An inline map: its quoted keys are handled below.
		default:
			line = line[:m[10]] + Redacted
		}
	}
	line = quotedKey.ReplaceAllStringFunc(line, func(s string) string {
		m := quotedKey.FindStringSubmatch(s)
		if !secretName(m[2]) || quiet(m[4]) {
			return s
		}
		return m[1] + m[2] + m[3] + `"` + Redacted + `"`
	})
	line = variable.ReplaceAllStringFunc(line, func(s string) string {
		name, value, _ := strings.Cut(s, "=")
		if !secretName(name) || quiet(value) {
			return s
		}
		return name + "=" + Redacted
	})
	line = flagged.ReplaceAllString(line, "${1}"+Redacted)
	line = userPassword.ReplaceAllString(line, "${1}"+Redacted)
	line = header.ReplaceAllString(line, "${1}"+Redacted)
	line = urlPassword.ReplaceAllString(line, "${1}"+Redacted+"${3}")
	line = tokens.ReplaceAllString(line, Redacted)
	return line
}

// quiet reports whether a value has nothing to hide: empty, already
// redacted, or a reference to an environment variable.
func quiet(value string) bool {
	v := strings.Trim(value, ` ,"'`)
	if v == "" || strings.HasPrefix(v, strings.TrimSuffix(Redacted, "]")) {
		return true
	}
	return envRef.MatchString(strings.TrimSuffix(value, ","))
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
