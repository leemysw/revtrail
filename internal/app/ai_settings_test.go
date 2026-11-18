package app

import (
	"strings"
	"testing"
)

// A saved key shows in the empty key field as its head and tail with
// dots between, never whole; a short one as dots alone.
func TestSavedKeyShowsMasked(t *testing.T) {
	for key, want := range map[string]string{
		"":                                       "",
		"short-key":                              "•••••••••",
		" sk-1234567890abcdefghijwxyz\n":         "sk-1" + strings.Repeat("•", 19) + "wxyz",
		"sk-" + strings.Repeat("x", 80) + "tail": "sk-x" + strings.Repeat("•", 32) + "tail",
	} {
		if got := maskKey(key); got != want {
			t.Errorf("maskKey(%q) = %q, want %q", key, got, want)
		}
	}
	h := newHarness(t, t.TempDir())
	h.a.cfg.AIBaseURL = "https://example.com/v1"
	s := &aiSettings{base: h.a.cfg.AIBaseURL + "/", saved: "sk-1••••wxyz"}
	if got := h.a.savedKeyHint(s); got != "sk-1••••wxyz" {
		t.Fatalf("hint %q", got)
	}
	s.base = "https://other.example.com/v1"
	if got := h.a.savedKeyHint(s); got != "" {
		t.Fatalf("another address shows the saved key of this one: %q", got)
	}
}
