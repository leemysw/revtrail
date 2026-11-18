package git

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func newRepo(t *testing.T) (string, func(args ...string) string) {
	t.Helper()
	root := t.TempDir()
	ctx := context.Background()
	run := func(args ...string) string {
		t.Helper()
		args = append([]string{"-c", "user.name=Revtrail", "-c", "user.email=revtrail@example.com", "-c", "commit.gpgsign=false", "-c", "init.defaultBranch=main"}, args...)
		out, err := Command(ctx, root, args...)
		if err != nil {
			t.Fatalf("git %v: %v", args, err)
		}
		return string(out)
	}
	run("init", "-q")
	return root, run
}

func write(t *testing.T, root, name, text string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, name), []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestBranchesSwitchAndCreate(t *testing.T) {
	root, run := newRepo(t)
	ctx := context.Background()
	write(t, root, "a", "a\n")
	run("add", "a")
	run("commit", "-qm", "first")
	run("branch", "feature")
	// A remote branch, as a fetch leaves it.
	run("remote", "add", "origin", "https://example.com/r.git")
	run("update-ref", "refs/remotes/origin/topic", "HEAD")
	run("symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/topic")

	bs, err := Branches(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, b := range bs {
		names = append(names, b.Name)
	}
	if !slices.Equal(names, []string{"main", "feature", "origin/topic"}) {
		t.Fatalf("branches %v", names)
	}
	if i := slices.IndexFunc(bs, func(b Branch) bool { return b.Current }); i < 0 || bs[i].Name != "main" {
		t.Fatalf("current: %+v", bs)
	}

	if err := Switch(ctx, root, Branch{Name: "feature"}, KeepChanges); err != nil {
		t.Fatal(err)
	}
	if _, b, _, _ := Read(ctx, root); b != "feature" {
		t.Fatalf("on %q after switching to feature", b)
	}
	// A remote branch checks out as a local one tracking it.
	if err := Switch(ctx, root, Branch{Name: "origin/topic", Remote: true, RemoteName: "origin"}, KeepChanges); err != nil {
		t.Fatal(err)
	}
	bs, _ = Branches(ctx, root)
	i := slices.IndexFunc(bs, func(b Branch) bool { return b.Name == "topic" })
	if i < 0 || !bs[i].Current || bs[i].Upstream != "origin/topic" {
		t.Fatalf("topic: %+v", bs)
	}

	if err := CreateBranch(ctx, root, "bad..name", "", true); err == nil {
		t.Fatal("made a branch of an invalid name")
	}
	if err := CreateBranch(ctx, root, "new/one", "", true); err != nil {
		t.Fatal(err)
	}
	if _, b, _, _ := Read(ctx, root); b != "new/one" {
		t.Fatalf("on %q after creating new/one", b)
	}
}

func TestIncomingAndUnpushedOfADivergedUpstream(t *testing.T) {
	root, run := newRepo(t)
	ctx := context.Background()
	run("commit", "--allow-empty", "-qm", "common")
	run("remote", "add", "origin", "https://example.com/repo.git")
	run("switch", "-qc", "upstream")
	run("commit", "--allow-empty", "-qm", "incoming")
	incoming := strings.TrimSpace(run("rev-parse", "HEAD"))
	run("update-ref", "refs/remotes/origin/main", incoming)
	run("switch", "-q", "main")
	run("branch", "--set-upstream-to=origin/main")
	run("commit", "--allow-empty", "-qm", "outgoing")
	outgoing := strings.TrimSpace(run("rev-parse", "HEAD"))
	run("switch", "-qc", "unrelated")
	run("commit", "--allow-empty", "-qm", "unrelated work")
	run("switch", "-q", "main")
	if got := Incoming(ctx, root); len(got) != 1 || !got[incoming] {
		t.Fatalf("incoming = %v", got)
	}
	if got := Unpushed(ctx, root); len(got) != 1 || !got[outgoing] {
		t.Fatalf("outgoing = %v", got)
	}
	run("branch", "--unset-upstream")
	if got := Incoming(ctx, root); len(got) != 0 {
		t.Fatalf("incoming without upstream = %v", got)
	}
}

func TestLogGraphAndShow(t *testing.T) {
	root, run := newRepo(t)
	ctx := context.Background()
	write(t, root, "a", "a\n")
	run("add", "a")
	run("commit", "-qm", "first")
	run("switch", "-qc", "side")
	write(t, root, "b", "b\n")
	run("add", "b")
	run("commit", "-qm", "side work")
	run("switch", "-q", "main")
	write(t, root, "a", "a\nmore\n")
	run("commit", "-qam", "main work")
	run("merge", "-q", "--no-ff", "-m", "merge side", "side")

	commits, err := FilteredLog(ctx, root, LogFilter{Ref: "HEAD"}, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	var subjects []string
	for _, c := range commits {
		subjects = append(subjects, c.Subject)
	}
	if len(commits) != 4 || subjects[0] != "merge side" || subjects[3] != "first" {
		t.Fatalf("subjects %q", subjects)
	}
	if !slices.Equal(commits[0].Refs, []string{"HEAD", "main"}) {
		t.Fatalf("refs %q", commits[0].Refs)
	}
	Graph(commits, nil)
	page1, err := FilteredLog(ctx, root, LogFilter{Ref: "HEAD"}, 0, 2)
	if err != nil {
		t.Fatal(err)
	}
	page2, err := FilteredLog(ctx, root, LogFilter{Ref: "HEAD"}, 2, 2)
	if err != nil {
		t.Fatal(err)
	}
	lanes := Graph(page1, nil)
	Graph(page2, lanes)
	paged := append(page1, page2...)
	for i, commit := range paged {
		want := commits[i]
		if commit.Hash != want.Hash || commit.Lane != want.Lane || !slices.Equal(commit.Before, want.Before) || !slices.Equal(commit.After, want.After) {
			t.Fatalf("page boundary changed topology for %s: %+v vs %+v", commit.Short, commit, want)
		}
	}
	// The merge opens a second lane, which ends in the first commit.
	if commits[0].Lane != 0 || len(commits[0].After) != 2 {
		t.Fatalf("merge: lane %d, after %q", commits[0].Lane, commits[0].After)
	}
	last := commits[3]
	if last.Lane != 0 || len(last.MergeFromLanes) != 1 || len(last.After) != 0 {
		t.Fatalf("first: lane %d, from %v, after %q", last.Lane, last.MergeFromLanes, last.After)
	}

	d, err := Show(ctx, root, commits[3].Hash)
	if err != nil || d.Body != "first" || len(d.Files) != 1 || d.Files[0].Path != "a" || d.Files[0].Added != 1 {
		t.Fatalf("first: %+v %v", d, err)
	}
	main := commits[slices.IndexFunc(commits, func(c Commit) bool { return c.Subject == "main work" })]
	patch, err := CommitFileDiff(ctx, root, main.Hash, FileStat{Path: "a"})
	if err != nil || !slices.Contains(splitLines(string(patch)), "+more") {
		t.Fatalf("patch %q %v", patch, err)
	}
}

func splitLines(s string) []string {
	var out []string
	start := 0
	for i := range len(s) {
		if s[i] == '\n' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	return out
}

func TestParseNumstatRename(t *testing.T) {
	files := ParseNumstat([]byte("1\t2\ta.go\x00-\t-\t\x00old.png\x00new.png\x00"))
	if len(files) != 2 || files[0].Path != "a.go" || files[0].Deleted != 2 ||
		files[1].Path != "new.png" || files[1].OldPath != "old.png" || files[1].Added != -1 {
		t.Fatalf("%+v", files)
	}
}

func TestCoAuthorsFromTheLog(t *testing.T) {
	root, run := newRepo(t)
	run("commit", "-q", "--allow-empty", "-m", "pair\n\nCo-authored-by: Ada <ada@x.org>\nCo-Authored-By: Bob <bob@x.org>\nco-authored-by: Ada again <ADA@x.org>\nCo-authored-by: Revtrail <revtrail@example.com>")
	commits, err := FilteredLog(context.Background(), root, LogFilter{Ref: "HEAD"}, 0, 1)
	if err != nil {
		t.Fatal(err)
	}
	got := commits[0].CoAuthors
	if len(got) != 2 || got[0] != (Person{"Ada", "ada@x.org"}) || got[1] != (Person{"Bob", "bob@x.org"}) {
		t.Fatalf("co-authors %+v, subject %q", got, commits[0].Subject)
	}
}

// CommitPatch is the patch of every file of a commit, with a few lines
// of context, the first commit's against the empty tree.
func TestCommitPatch(t *testing.T) {
	root, run := newRepo(t)
	ctx := context.Background()
	write(t, root, "a", "1\n2\n3\n4\n5\n6\n7\n8\n9\n10\n")
	run("add", "a")
	run("commit", "-qm", "first")
	write(t, root, "a", "1\n2\n3\n4\n5\n6\n7\n8\n9\nten\n")
	write(t, root, "b", "b\n")
	run("add", ".")
	run("commit", "-qm", "second")
	out, err := CommitPatch(ctx, root, "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	for _, want := range []string{"diff --git a/a b/a", "-10", "+ten", "diff --git a/b b/b", "+b"} {
		if !strings.Contains(s, want) {
			t.Fatalf("no %q in\n%s", want, s)
		}
	}
	if strings.Contains(s, " 2\n") {
		t.Fatalf("the whole file, not a few lines around the change:\n%s", s)
	}
	out, err = CommitPatch(ctx, root, "HEAD~1")
	if err != nil || !strings.Contains(string(out), "+1\n") {
		t.Fatalf("first commit: %v\n%s", err, out)
	}
}

func TestParseNameStatus(t *testing.T) {
	got := parseNameStatus([]byte("M\x00a.go\x00R100\x00old.go\x00new.go\x00A\x00b.go\x00"))
	if got["a.go"] != 'M' || got["new.go"] != 'R' || got["b.go"] != 'A' || len(got) != 3 {
		t.Fatalf("parseNameStatus = %v", got)
	}
}

// The log window's path filter and IntelliSort switch, Show My Branches,
// Go to Hash, and Revert Selected Changes of a commit's file.
func TestLogPathsOrderMineAndRevert(t *testing.T) {
	ctx := context.Background()
	root, run := newRepo(t)
	run("config", "user.email", "revtrail@example.com")
	write(t, root, "a.txt", "a\n")
	if err := os.Mkdir(filepath.Join(root, "dir"), 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, root, "dir/b.txt", "b\n")
	run("add", ".")
	run("commit", "-qm", "base")
	write(t, root, "dir/b.txt", "b2\n")
	write(t, root, "a.txt", "a2\n")
	run("commit", "-qam", "both")
	write(t, root, "a.txt", "a3\n")
	run("commit", "-qam", "only a")
	run("branch", "other")
	commits, err := FilteredLog(ctx, root, LogFilter{Paths: "dir\n"}, 0, 10)
	if err != nil || len(commits) != 2 || commits[0].Subject != "both" {
		t.Fatalf("path filter = %+v (%v)", commits, err)
	}
	if commits, err = FilteredLog(ctx, root, LogFilter{DateOrder: true}, 0, 10); err != nil || len(commits) != 3 {
		t.Fatalf("date order = %+v (%v)", commits, err)
	}
	mine, err := MyBranches(ctx, root)
	if err != nil || !mine["refs/heads/main"] || !mine["refs/heads/other"] {
		t.Fatalf("MyBranches = %v (%v)", mine, err)
	}
	hash, err := ResolveCommit(ctx, root, "other~1")
	if err != nil || !strings.HasPrefix(run("log", "-1", "--format=%s", hash), "both") {
		t.Fatalf("ResolveCommit = %q (%v)", hash, err)
	}
	if _, err := ResolveCommit(ctx, root, "nope"); err == nil {
		t.Fatal("resolved an unknown revision")
	}
	if err := RevertFiles(ctx, root, hash, []string{"dir/b.txt"}); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(filepath.Join(root, "dir", "b.txt")); string(data) != "b\n" {
		t.Fatalf("dir/b.txt after revert = %q", data)
	}
	if data, _ := os.ReadFile(filepath.Join(root, "a.txt")); string(data) != "a3\n" {
		t.Fatalf("a.txt changed by the revert: %q", data)
	}
}
