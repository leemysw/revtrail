package git

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCommitSelectedPreservesUnrelatedIndex(t *testing.T) {
	ctx, root := context.Background(), t.TempDir()
	git := func(args ...string) string {
		t.Helper()
		out, err := Command(ctx, root, args...)
		if err != nil {
			t.Fatal(err)
		}
		return strings.TrimSpace(string(out))
	}
	write := func(p, s string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(root, p), []byte(s+"\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	git("init", "-q")
	git("config", "user.name", "Revtrail")
	git("config", "user.email", "test@example.com")
	git("config", "commit.gpgsign", "false")
	write("other", "excluded index")
	git("add", "other")
	write("other", "excluded working copy")
	write("new file", "selected")
	selected := []Change{{Path: "new file", Index: '?', Work: '?'}}
	patch, err := CommitDiff(ctx, root, selected, false)
	if err != nil || !strings.Contains(patch, "+selected") || strings.Contains(patch, "excluded") {
		t.Fatalf("AI patch %q: %v", patch, err)
	}
	args := []string{"commit", "-m", "selected files"}
	if err := CommitSelected(ctx, root, selected, false, false, args); err != nil {
		t.Fatal(err)
	}
	if git("ls-tree", "--name-only", "HEAD") != "new file" || git("show", ":other") != "excluded index" || git("diff", "--cached", "--name-only") != "other" {
		t.Fatal("committed or lost an excluded staged file")
	}
	write("new file", "staged")
	git("add", "new file")
	write("new file", "latest working text")
	selected[0] = Change{Path: "new file", Index: 'M', Work: 'M'}
	if err := CommitSelected(ctx, root, selected, true, true, args); err != nil {
		t.Fatal(err)
	}
	if git("show", "HEAD:new file") != "latest working text" || git("diff", "--cached", "--name-only") != "other" {
		t.Fatal("partial stage did not commit selected working copy")
	}
	head := git("rev-parse", "HEAD")
	write("bad.go", "invalid Go")
	if err := CommitSelected(ctx, root, []Change{{Path: "bad.go", Index: '?'}}, false, true, args); err == nil {
		t.Fatal("check accepted invalid Go")
	}
	if git("rev-parse", "HEAD") != head || git("diff", "--cached", "--name-only") != "other" || git("ls-files", "--", "bad.go") != "" {
		t.Fatal("failed check modified HEAD or index")
	}
	if err := CommitSelected(ctx, root, nil, false, false, []string{"commit", "--amend", "-m", "message only"}); err != nil {
		t.Fatal(err)
	}
	if git("diff", head, "HEAD") != "" || git("diff", "--cached", "--name-only") != "other" {
		t.Fatal("amend included an excluded file")
	}
}
