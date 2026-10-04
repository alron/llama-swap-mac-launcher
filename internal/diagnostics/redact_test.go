package diagnostics

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"
)

// A config with keys in every odd place, and lines that must survive.
const config = `# my key, before I moved it: sk-proj-abcdefghijklmnopqrstuv
apiKeys:
  - "${env.LLSL_ADMIN}"
  - "my-own-plain-key-1234"
  - sk-VWiW1+0KSLuiVYZBUQAOA7iGHSUOT2NDQ0xyW+g8PtestAbCd/12
peerApiKeys: ["${env.LLSL_X}", "another-plain-key", 'third']
label: abc
macros:
  "auth": "--api-key private-macro-key"
  "my_secret": "verysecretvalue"
  "model_dir": "/Volumes/Models"
peers:
  studio:
    proxy: http://studio.local:8080
    apiKey: peer-secret-key-999
  other:
    proxy: https://alice:hunter2pass@other.local:8080
models:
  gemma:
    cmd: llama-server --port ${PORT} -m /models/gemma.gguf --ctx-size 8192 --api-key cmd-secret-123
    env:
      - HF_TOKEN=hf_abcdefghijklmnopqrstuvwx
      - CUDA_VISIBLE_DEVICES=0
    proxy: http://127.0.0.1:${PORT}
    headers:
      Authorization: "Bearer header-secret-xyz"
secrets:
  db: plaintext-in-a-block
  ref: ${env.SOMETHING}
ttl: 300
aws: AKIAABCDEFGHIJKLMNOP
github: ghp_abcdefghijklmnopqrstuvwxyz0123
`

func TestRedactConfig(t *testing.T) {
	got := NewRedactor("known-secret-value", "ab").Text(config + "note: known-secret-value in a comment\n")
	for _, secret := range []string{
		"sk-proj-abcdefghijklmnopqrstuv", "my-own-plain-key-1234", "sk-VWiW1", "PtestAbCd/12",
		"another-plain-key", "third", "private-macro-key", "verysecretvalue", "peer-secret-key-999",
		"hunter2pass", "cmd-secret-123", "hf_abcdefghijklmnopqrstuvwx", "header-secret-xyz",
		"plaintext-in-a-block", "AKIAABCDEFGHIJKLMNOP", "ghp_abcdefghijklmnopqrstuvwxyz0123", "known-secret-value",
	} {
		if strings.Contains(got, secret) {
			t.Errorf("%q survived", secret)
		}
	}
	for _, keep := range []string{
		`- "${env.LLSL_ADMIN}"`, `"${env.LLSL_X}"`, `"model_dir": "/Volumes/Models"`,
		"proxy: http://studio.local:8080", "https://alice:" + Redacted + "@other.local:8080",
		"llama-server --port ${PORT} -m /models/gemma.gguf --ctx-size 8192 --api-key " + Redacted,
		"- CUDA_VISIBLE_DEVICES=0", "proxy: http://127.0.0.1:${PORT}", "ref: ${env.SOMETHING}",
		"ttl: 300", "studio:", "gemma:", `"auth": ` + Redacted, "apiKey: " + Redacted, "Authorization: " + Redacted,
		`"my_secret": ` + Redacted,
	} {
		if !strings.Contains(got, keep) {
			t.Errorf("%q didn't survive", keep)
		}
	}
	if t.Failed() {
		t.Logf("redacted:\n%s", got)
	}
	// A two-character "secret" is ignored: it would match too much.
	if !strings.Contains(got, "label: abc") {
		t.Error("a two-character known value was redacted everywhere")
	}
}

func TestRedactLogLines(t *testing.T) {
	r := NewRedactor()
	for in, want := range map[string]string{
		`[launcher] started llama-swap (pid 1): /opt/homebrew/bin/llama-swap -config /x.yaml -listen 127.0.0.1:8080`: `[launcher] started llama-swap (pid 1): /opt/homebrew/bin/llama-swap -config /x.yaml -listen 127.0.0.1:8080`,
		`request with "Authorization: Bearer sk-abcdefghijklmnopqrstuvwx" failed`:                                    `request with "Authorization: Bearer ` + Redacted + `" failed`,
		`env HF_TOKEN=hf_abc`:    `env HF_TOKEN=` + Redacted,
		`plain line, no secrets`: `plain line, no secrets`,
	} {
		if got := r.Line(in); got != want {
			t.Errorf("Line(%q)\n  = %q\nwant %q", in, got, want)
		}
	}
}

func TestAnonymize(t *testing.T) {
	in := "/Users/jdoe/ai/x.yaml and /private/tmp/claude-501/-Users-jdoe-proj/bin, jdoes stays"
	want := "~/ai/x.yaml and /private/tmp/claude-501/-Users-<user>-proj/bin, jdoes stays"
	if got := Anonymize(in, "/Users/jdoe", "jdoe"); got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
	if got := Anonymize("mac and macos", "", "mac"); got != "mac and macos" {
		t.Errorf("a short name was replaced: %q", got)
	}
}

func TestRedactLeavesEmptyValues(t *testing.T) {
	for _, line := range []string{`  "appApiKey": "",`, `  "appApiKey": ""`, `token: ''`} {
		if got := NewRedactor().Line(line); got != line {
			t.Errorf("Line(%q) = %q", line, got)
		}
	}
}

func TestZip(t *testing.T) {
	var b bytes.Buffer
	if err := Zip(&b, "diag", []File{{"a.txt", []byte("hello")}, {"b.log", []byte("world")}}); err != nil {
		t.Fatal(err)
	}
	r, err := zip.NewReader(bytes.NewReader(b.Bytes()), int64(b.Len()))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, f := range r.File {
		names = append(names, f.Name)
	}
	if strings.Join(names, ",") != "diag/a.txt,diag/b.log" {
		t.Errorf("got %v", names)
	}
}

// The samples the 2026-10-04 security assessment got past the first
// version, and lines that must survive.
func TestRedactAssessmentSamples(t *testing.T) {
	in := `# apiKey: old-commented-key
apiKey: |
  block-scalar-key-1234
  second-line-of-it
ttl: 60
headers:
  x-api-key: header-dash-key
cmd: curl -H "x-api-key: curl-header-key" -u alice:curl-user-pass http://host/v1
key: plain-key-value
peer_key: peer-key-value
pass: pass-value-1
pwd: pwd-value-1
peerKey: camel-key-value
env: MYPASS=env-pass-value WANDB_API_KEY=wandb-value
json: {"api_key":"json-inline-key","model":"m"}
tokens: github_pat_11ABCDEFG0123456789_abcdefghij glpat-abcdefghij0123456789 AIzaSyA1234567890abcdefghijklmnopqrstuv
models:
  monkey-7b:
    cmd: llama-server --port ${PORT}
  turkey:
    ttl: 300
log: slot update_slots: n_tokens = 512, n_past = 48
macro: "innocent-name-known-secret"
`
	got := NewRedactor("innocent-name-known-secret").Text(in)
	for _, secret := range []string{
		"old-commented-key", "block-scalar-key-1234", "second-line-of-it", "header-dash-key", "curl-header-key",
		"curl-user-pass", "plain-key-value", "peer-key-value", "pass-value-1", "pwd-value-1", "camel-key-value",
		"env-pass-value", "wandb-value", "json-inline-key", "github_pat_11ABCDEFG", "glpat-abcdefghij", "AIzaSyA1234567890",
		"innocent-name-known-secret",
	} {
		if strings.Contains(got, secret) {
			t.Errorf("%q survived", secret)
		}
	}
	for _, keep := range []string{
		"ttl: 60", "alice:", `"model":"m"`, "monkey-7b:", "turkey:", "    ttl: 300",
		"cmd: llama-server --port ${PORT}", "n_tokens = 512, n_past = 48", "http://host/v1",
	} {
		if !strings.Contains(got, keep) {
			t.Errorf("%q didn't survive", keep)
		}
	}
	if t.Failed() {
		t.Logf("redacted:\n%s", got)
	}
}
