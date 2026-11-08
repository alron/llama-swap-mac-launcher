package macos

import "testing"

// A click's identifier and action, split as notify.m joins them: at the
// last newline, so a model ID with one in it stays whole (TODO item 84).
func TestParseResponse(t *testing.T) {
	for _, tt := range []struct {
		in   string
		want NotificationResponse
	}{
		{"gpu-fault:qwen\nunload", NotificationResponse{ID: "gpu-fault:qwen", Action: "unload"}},
		{"failed\ndefault", NotificationResponse{ID: "failed", Action: "default"}},
		{"gpu-fault:a\nb\nunload", NotificationResponse{ID: "gpu-fault:a\nb", Action: "unload"}},
		{"update", NotificationResponse{ID: "update"}},
		{"", NotificationResponse{}},
	} {
		if got := parseResponse(tt.in); got != tt.want {
			t.Errorf("%q: %+v, want %+v", tt.in, got, tt.want)
		}
	}
}
