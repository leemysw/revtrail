package app

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"revtrail/internal/tools"
)

func TestFindRipgrepUsesManagedCopy(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("APPDATA", filepath.Join(home, "config"))
	t.Setenv("REVTRAIL_RG", "")
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	t.Setenv("PATH", t.TempDir()) // no ripgrep on the path
	path := tools.RipgrepPath()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0700); err != nil {
		t.Fatal(err)
	}
	got, err := findRipgrep("")
	if err != nil || got != path {
		t.Fatalf("path %q, error %v", got, err)
	}
	// $REVTRAIL_RG still wins over the copy Revtrail installed.
	other := filepath.Join(t.TempDir(), "rg")
	if err := os.WriteFile(other, []byte("#!/bin/sh\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("REVTRAIL_RG", other)
	if got, err := findRipgrep(""); err != nil || got != other {
		t.Fatalf("path %q, error %v", got, err)
	}
}

func TestFindGoplsUsesManagedCopy(t *testing.T) {
	home := t.TempDir()
	t.Setenv("REVTRAIL_GOPLS", "")
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	t.Setenv("PATH", t.TempDir())
	t.Setenv("GOPATH", "")
	t.Setenv("GOBIN", "")
	t.Setenv("HOME", home)
	path := tools.GoplsPath()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0700); err != nil {
		t.Fatal(err)
	}
	if got, err := findGopls(""); err != nil || got != path {
		t.Fatalf("path %q, error %v", got, err)
	}
}

// The download itself is tested in internal/tools; here Settings runs it,
// shows why it failed, and removes Revtrail's copy.
func TestInstallingRipgrepFromSettingsShowsFailureAndRemoves(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(srv.Close)
	old := tools.RipgrepReleaseURL
	tools.RipgrepReleaseURL = srv.URL
	t.Cleanup(func() { tools.RipgrepReleaseURL = old })
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("APPDATA", filepath.Join(home, "config"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	t.Setenv("REVTRAIL_RG", "")
	t.Setenv("PATH", t.TempDir())
	h := newHarness(t, t.TempDir())
	h.a.settings.pane = "search"
	st := ui.NewTester(h.a.settingsView, 900, 680)
	if err := st.Click("Install ripgrep"); err != nil {
		t.Fatalf("%v: %q", err, st.Texts())
	}
	h.settle(func() bool { return !h.a.serverJob })
	for range 5 {
		st.Frame()
	}
	if !strings.Contains(h.a.serverJobErr, "404") || !st.HasText(h.a.serverJobErr) {
		t.Fatalf("error %q not shown: %q", h.a.serverJobErr, st.Texts())
	}
	path := tools.RipgrepPath()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0700); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		st.Frame()
	}
	if err := st.Click("Remove managed ripgrep"); err != nil {
		t.Fatalf("%v: %q", err, st.Texts())
	}
	h.settle(func() bool { return !h.a.serverJob })
	if tools.RipgrepExists() {
		t.Fatal("ripgrep was not removed")
	}
}

func TestInstallingGoplsFromSettingsBuildsIt(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	t.Setenv("REVTRAIL_GOPLS", "")
	t.Setenv("GOPATH", "")
	t.Setenv("GOBIN", "")
	bin := t.TempDir()
	script := "#!/bin/sh\nmkdir -p \"$GOBIN\"\nprintf gopls > \"$GOBIN/gopls\"\n"
	if err := os.WriteFile(filepath.Join(bin, "go"), []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+"/usr/bin:/bin")
	h := newHarness(t, writeTree(t, map[string]string{"a.go": "package a\n"}))
	h.a.settings.pane = "intelligence"
	st := ui.NewTester(h.a.settingsView, 900, 680)
	if !st.HasText("Install gopls") {
		t.Fatalf("no install button: %q", st.Texts())
	}
	if err := st.Click("Install gopls"); err != nil {
		t.Fatal(err)
	}
	h.settle(func() bool { return !h.a.serverJob })
	for range 5 {
		st.Frame()
	}
	if _, err := os.Stat(tools.GoplsPath()); err != nil {
		t.Fatalf("gopls was not installed: %q", st.Texts())
	}
	if !st.HasText("Remove managed gopls") {
		t.Fatalf("missing installed state: %v", st.Texts())
	}
	if got, err := findGopls(h.a.root); err != nil || got != tools.GoplsPath() {
		t.Fatalf("path %q, error %v", got, err)
	}
}
