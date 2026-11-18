package git

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBlameFileHistoryCompareAndPatch(t *testing.T) {
	ctx := context.Background()
	root, run := newRepo(t)
	write(t, root, "note.txt", "line1\nline2\n")
	run("add", "note.txt")
	run("commit", "-qm", "first note")

	run("checkout", "-qb", "feature")
	write(t, root, "note.txt", "line1\nline2\nline3\n")
	run("add", "note.txt")
	run("commit", "-qm", "add line3")

	blame, err := Blame(ctx, root, "note.txt")
	if err != nil || len(blame) != 3 {
		t.Fatalf("Blame = %+v (%v), want 3 lines", blame, err)
	}
	if blame[0].Summary != "first note" || blame[2].Summary != "add line3" || blame[2].Line != 3 {
		t.Fatalf("unexpected blame lines: %+v", blame)
	}

	hist, err := FileHistory(ctx, root, "note.txt", 0, 10)
	if err != nil || len(hist) != 2 || hist[0].Subject != "add line3" || hist[1].Subject != "first note" {
		t.Fatalf("FileHistory = %+v (%v)", hist, err)
	}

	cmp, err := CompareRefs(ctx, root, "main", "feature")
	if err != nil {
		t.Fatalf("CompareRefs: %v", err)
	}
	if len(cmp.Forward) != 1 || len(cmp.Backward) != 0 || len(cmp.Files) != 1 || cmp.Files[0].Path != "note.txt" {
		t.Fatalf("CompareRefs result = %+v", cmp)
	}

	patch, err := CommitPatch(ctx, root, cmp.Forward[0].Hash)
	if err != nil || !strings.Contains(string(patch), "+line3") {
		t.Fatalf("CommitPatch = %q (%v)", patch, err)
	}

	// Switch back to main and apply the patch to the working tree.
	run("checkout", "-q", "main")
	if err := ApplyPatchText(ctx, root, string(patch)); err != nil {
		t.Fatalf("ApplyPatchText: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(root, "note.txt"))
	if err != nil || string(data) != "line1\nline2\nline3\n" {
		t.Fatalf("note.txt after ApplyPatchText = %q (%v)", data, err)
	}

	// The working tree now matches feature, and differs from main.
	if cmp, err := CompareRefs(ctx, root, "feature", ""); err != nil || len(cmp.Files) != 0 || len(cmp.Forward)+len(cmp.Backward) != 0 {
		t.Fatalf("CompareRefs(feature, working tree) = %+v (%v), want no differences", cmp, err)
	}
	cmp, err = CompareRefs(ctx, root, "main", "")
	if err != nil || len(cmp.Files) != 1 || cmp.Files[0].Path != "note.txt" {
		t.Fatalf("CompareRefs(main, working tree) = %+v (%v)", cmp, err)
	}
	diff, err := CompareFileDiff(ctx, root, "main", "", cmp.Files[0])
	if err != nil || !strings.Contains(string(diff), "+line3") {
		t.Fatalf("CompareFileDiff(main, working tree) = %q (%v)", diff, err)
	}
}
