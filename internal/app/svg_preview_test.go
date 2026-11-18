package app

import (
	"testing"
)

func TestSVGPreviewFollowsItsText(t *testing.T) {
	root := gitRepo(t, map[string]string{"logo.svg": "<svg xmlns=\"http://www.w3.org/2000/svg\" width=\"64\" height=\"32\"><rect width=\"64\" height=\"32\" fill=\"red\"/></svg>\n"})
	h := newHarness(t, root)
	d := h.open("logo.svg")
	h.tt.Frame()
	if _, ok := h.tt.Find("Image preview"); ok {
		t.Fatal("an SVG opens as text")
	}
	if err := h.tt.Click("Preview Only"); err != nil {
		t.Fatalf("no Preview Only button: %v", err)
	}
	h.tt.Frame()
	if _, ok := h.tt.Find("Image preview"); !ok || !h.tt.HasText("64 × 32") {
		t.Fatalf("Preview Only shows no image: %q", h.tt.Texts())
	}
	d.buf.Set("<svg")
	h.tt.Frame()
	if !h.tt.HasText("Cannot read this SVG") {
		t.Fatalf("broken SVG text shows no message: %q", h.tt.Texts())
	}
}
