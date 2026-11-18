package app

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

func markdownHarness(t *testing.T, files map[string]string, mode markdownMode) (*harness, *doc) {
	t.Helper()
	root := gitRepo(t, files)
	h := newHarness(t, root)
	h.tt = ui.NewTester(h.a.view, 1100, 700)
	d := h.open("README.md")
	h.a.setMarkdownMode(h.a.focus, mode)
	for i := 0; i < 4; i++ {
		h.tt.Frame()
	}
	return h, d
}

func TestMarkdownPreviewShowsGitHubHTML(t *testing.T) {
	src := strings.Join([]string{
		"<div align=\"center\">",
		"  <h1>Plumb</h1>",
		"  <p>A <b>fast</b> tool</p>",
		"</div>",
		"",
		"<details>",
		"<summary>More info</summary>",
		"",
		"Hidden text.",
		"</details>",
		"",
		"<table><tr><th>Key</th></tr><tr><td>cell</td></tr></table>",
	}, "\n")
	h, _ := markdownHarness(t, map[string]string{"README.md": src}, markdownRendered)
	for _, want := range []string{"Plumb", "cell", "More info"} {
		if !h.tt.HasText(want) {
			t.Fatalf("preview lacks %q: %q", want, h.tt.Texts())
		}
	}
	if h.tt.HasText("<div") || h.tt.HasText("Hidden text.") {
		t.Fatalf("HTML shows as source, or a closed details shows its body: %q", h.tt.Texts())
	}
	if err := h.tt.Click("More info"); err != nil {
		t.Fatal(err)
	}
	h.tt.Frame()
	h.tt.Frame()
	if !h.tt.HasText("Hidden text.") {
		t.Fatalf("opening details shows nothing: %q", h.tt.Texts())
	}
}

func TestLinkedImagesAreBadges(t *testing.T) {
	h, d := markdownHarness(t, map[string]string{"README.md": "[![build](missing.png)](https://example.com) [![cov](missing.png)](https://example.com)\n"}, markdownRendered)
	if d.preview == nil || len(d.preview.images) != 1 {
		t.Fatalf("badge images were not loaded: %+v", d.preview)
	}
	if !h.tt.HasText("[build]") || !h.tt.HasText("[cov]") {
		t.Fatalf("badges show no images or descriptions: %q", h.tt.Texts())
	}
}

func TestMarkdownRelativeLinkOpensFile(t *testing.T) {
	h, d := markdownHarness(t, map[string]string{"README.md": "[guide](docs/guide.md)\n", "docs/guide.md": "# Guide\n"}, markdownRendered)
	h.a.markdownLink(d, "docs/guide.md#install")
	want := filepath.Join(h.a.root, "docs", "guide.md")
	if h.a.focus.active != want {
		t.Fatalf("active tab is %q, want %q", h.a.focus.active, want)
	}
}

func TestMarkdownSideBySideScrollsTogether(t *testing.T) {
	var sb strings.Builder
	for i := 0; i < 60; i++ {
		fmt.Fprintf(&sb, "## Section %d\n\nParagraph %d.\n\n", i, i)
	}
	h, d := markdownHarness(t, map[string]string{"README.md": sb.String()}, markdownSideBySide)
	st := h.a.focus.eds[h.a.focus.active]
	st.ScrollTo(120)
	for i := 0; i < 4; i++ {
		h.tt.Frame()
	}
	ps := d.preview
	if ps.scroll.Y <= 0 {
		t.Fatalf("scrolling the source left the preview at %v", ps.scroll.Y)
	}
	before := st.ScrollY()
	ps.scroll.Y /= 2
	for i := 0; i < 4; i++ {
		h.tt.Frame()
	}
	if st.ScrollY() >= before {
		t.Fatalf("scrolling the preview up left the source at %v (was %v)", st.ScrollY(), before)
	}
}
