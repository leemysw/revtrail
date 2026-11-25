package app

import "testing"

// An HTML file has IntelliJ's preview layouts; without a window to put a
// web view in, as in tests, the preview says it cannot show.
func TestHTMLFileOffersPreview(t *testing.T) {
	root := gitRepo(t, map[string]string{"page.html": "<h1>Hello</h1>\n"})
	h := newHarness(t, root)
	d := h.open("page.html")
	h.tt.Frame()
	if err := h.tt.Click("Preview Only"); err != nil {
		t.Fatalf("no Preview Only button: %v", err)
	}
	h.tt.Frame()
	if !h.tt.HasText("The preview cannot show here") {
		t.Fatalf("preview pane missing: %q", h.tt.Texts())
	}
	h.a.closeTab(h.a.focus, d.path)
	if d.html != nil {
		t.Fatal("closing the tab kept the preview")
	}
}
