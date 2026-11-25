package git

import (
	"context"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"
)

// As IntelliJ's vcs.root.detector.ignore.pattern, [.].*|vendor: clones in
// hidden folders or vendor are not repositories of the workspace.
func TestRootsSkipHiddenAndVendor(t *testing.T) {
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{"app", ".claude/plugins/tool", "vendor/lib", "src/.cache-like/repo"} {
		if out, err := exec.Command("git", "init", "-q", filepath.Join(dir, rel)).CombinedOutput(); err != nil {
			t.Fatalf("%s: %v %s", rel, err, out)
		}
	}
	got := Roots(context.Background(), dir)
	if want := []string{filepath.Join(dir, "app")}; !slices.Equal(got, want) {
		t.Fatalf("roots %q, want %q", got, want)
	}
}
