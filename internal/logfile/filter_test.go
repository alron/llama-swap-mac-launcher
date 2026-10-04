package logfile

import (
	"bytes"
	"testing"
)

func TestDropLines(t *testing.T) {
	var out bytes.Buffer
	w := DropLines(&out, "healthcheck")
	// Lines arrive split across writes, as pipe reads deliver them.
	for _, chunk := range []string{
		"keep one\nGET /health 200 \"llama-swap-launcher-heal",
		"thcheck\"\nkeep ",
		"two\npartial",
	} {
		if n, err := w.Write([]byte(chunk)); err != nil || n != len(chunk) {
			t.Fatalf("Write(%q) = %d, %v", chunk, n, err)
		}
	}
	if got, want := out.String(), "keep one\nkeep two\n"; got != want {
		t.Errorf("got %q, want %q (the unfinished line is held back)", got, want)
	}
	w.Write([]byte(" line\n"))
	if got, want := out.String(), "keep one\nkeep two\npartial line\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
