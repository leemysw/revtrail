package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"revtrail/internal/langserver"
	"revtrail/internal/languages"
)

// TestRealGopls jumps across files with a real gopls, through Unicode
// before the symbol on both sides. (Größe名字 is exported, as 名字 alone
// would not be, which gopls rightly finds no definition for.) It runs when
// REVTRAIL_GOPLS names gopls:
//
//	REVTRAIL_GOPLS=$(which gopls) go test -run RealGopls ./internal/app
func TestRealGopls(t *testing.T) {
	if os.Getenv("REVTRAIL_GOPLS") == "" {
		t.Skip("set REVTRAIL_GOPLS to the path of gopls")
	}
	root := writeTree(t, map[string]string{
		"go.mod": "module example.com/w\n\ngo 1.21\n",
		"a.go":   "package main\n\nimport \"example.com/w/b\"\n\nfunc main() {\n\t_ = \"🎉中\"; b.Größe名字()\n}\n",
		"b/b.go": "package b\n\n// Größe名字 is 🎉.\nvar _ = \"😀\"; func Größe名字() {}\n",
	})
	h := newHarness(t, root)
	h.a.startLSP = languages.StartGopls // the real one, found through REVTRAIL_GOPLS
	h.a.langs[langGo].server = langserver.New(langserver.Options{
		Name: "gopls", Root: h.a.root, Start: h.a.startLSP(testLogf(t)),
		OnState: func(s langserver.State, err error) {
			h.a.post.Post(func() { h.a.langs[langGo].state, h.a.langs[langGo].err = s, err })
		},
	})
	defer h.a.langs[langGo].server.StopWait(5e9)
	h.a.cfg.Trust(h.a.root)

	from := h.open("a.go")
	off := strings.Index(from.buf.String(), "b.Größe名字")
	if off < 0 {
		t.Fatal("the call is not in a.go")
	}
	off += len("b.")
	h.a.activeEditor().GoTo(from.buf.Pos(off))
	h.a.goToDefinition(h.a.focus, from, off)
	target := filepath.Join(root, "b", "b.go")
	h.settle(func() bool {
		d := h.a.docs[target]
		return h.a.activePath() == target && d != nil && !d.loading && h.a.activeEditor() != nil
	})
	targetDoc := h.a.docs[target]
	wantOffset := strings.Index(targetDoc.buf.String(), "Größe名字()")
	if wantOffset < 0 || h.a.activeEditor().Cursor() != wantOffset {
		t.Fatalf("definition cursor=%v want byte offset %d", h.a.activeEditor().CursorPos(), wantOffset)
	}

	// Back returns to the call.
	h.a.goBack()
	h.settle(func() bool { return h.a.activePath() == filepath.Join(root, "a.go") && h.a.activeEditor() != nil })
	if h.a.activeEditor().Cursor() != off {
		t.Fatalf("Go Back cursor=%v want %+v", h.a.activeEditor().CursorPos(), from.buf.Pos(off))
	}
}

// TestRealGoplsRefactoring renames across files, optimizes imports,
// reformats and extracts a variable with a real gopls.
func TestRealGoplsRefactoring(t *testing.T) {
	if os.Getenv("REVTRAIL_GOPLS") == "" {
		t.Skip("set REVTRAIL_GOPLS to the path of gopls")
	}
	root := writeTree(t, map[string]string{
		"go.mod": "module example.com/w\n\ngo 1.21\n",
		"a.go":   "package main\n\nimport (\n\t\"fmt\"\n\t\"os\"\n)\n\nfunc greet() string {\n\treturn \"hi\" + \"!\"\n}\n\nfunc  main() {\n\tfmt.Println(greet())\n}\n",
		"b.go":   "package main\n\nvar _ = greet\n",
	})
	h := newHarness(t, root)
	h.a.startLSP = languages.StartGopls
	h.a.langs[langGo].server = langserver.New(langserver.Options{
		Name: "gopls", Root: h.a.root, Start: h.a.startLSP(testLogf(t)),
		OnState: func(s langserver.State, err error) {
			h.a.post.Post(func() { h.a.langs[langGo].state, h.a.langs[langGo].err = s, err })
		},
	})
	defer h.a.langs[langGo].server.StopWait(5e9)
	h.a.cfg.Trust(h.a.root)
	d := h.open("a.go")
	st := h.a.activeEditor()
	has := func(s string) bool { return strings.Contains(d.buf.String(), s) }
	idle := func(cond func() bool) {
		t.Helper()
		deadline := time.Now().Add(20 * time.Second)
		stop := make(chan struct{})
		defer close(stop)
		go func() { // checks cond now and then, for the deadline
			for {
				select {
				case <-stop:
					return
				case <-time.After(200 * time.Millisecond):
					h.q <- func() {}
				}
			}
		}()
		h.settle(func() bool {
			if time.Now().After(deadline) {
				t.Fatalf("gave up: notice %q, text\n%s", h.a.notice, d.buf.String())
			}
			return !h.a.finding && cond()
		})
	}

	off := strings.Index(d.buf.String(), "greet()")
	st.Select(off, off)
	h.a.runCommand("Rename…")
	idle(func() bool { h.tt.Frame(); return h.a.git.prompt != nil })
	h.a.git.prompt.value = "welcome"
	h.tt.Frame()
	if err := h.tt.Click("Refactor"); err != nil {
		t.Fatal(err)
	}
	idle(func() bool { return has("func welcome()") && has("Println(welcome())") })
	if b, err := os.ReadFile(filepath.Join(root, "b.go")); err != nil || !strings.Contains(string(b), "var _ = welcome") {
		t.Fatalf("b.go %q, %v", b, err)
	}

	h.a.runCommand("Optimize Imports")
	idle(func() bool { return !has("\"os\"") })

	h.a.runCommand("Reformat Code")
	idle(func() bool { return has("func main()") })

	off = strings.Index(d.buf.String(), "\"hi\" + \"!\"")
	st.Select(off, off+len("\"hi\" + \"!\""))
	h.a.runCommand("Extract Variable…")
	idle(func() bool { h.tt.Frame(); return h.a.refactorPopup != nil || has("= \"hi\" + \"!\"\n") })
	if p := h.a.refactorPopup; p != nil {
		p.items[p.sel].run()
		h.a.refactorPopup = nil
		idle(func() bool { return has("= \"hi\" + \"!\"\n") })
	}
}
