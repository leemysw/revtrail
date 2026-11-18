package git

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestWebURL(t *testing.T) {
	for in, want := range map[string]string{
		"git@github.com:example/revtrail.git":       "https://github.com/example/revtrail",
		"https://github.com/example/revtrail.git\n": "https://github.com/example/revtrail",
		"ssh://git@gitlab.com/group/sub/repo":       "https://gitlab.com/group/sub/repo",
		"https://user@github.com/o/r":               "https://github.com/o/r",
		"/srv/git/repo.git":                         "",
		"file:///srv/git/repo.git":                  "",
	} {
		if got := WebURL(in); got != want {
			t.Errorf("WebURL(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestTracking(t *testing.T) {
	root, run := newRepo(t)
	ctx := context.Background()
	write(t, root, "a", "a\n")
	run("add", "a")
	run("commit", "-qm", "first")
	if up, _, _ := Tracking(ctx, root); up != "" {
		t.Fatalf("upstream without one: %q", up)
	}
	run("remote", "add", "origin", "https://example.com/r.git")
	run("update-ref", "refs/remotes/origin/main", "HEAD")
	run("branch", "--set-upstream-to=origin/main")
	run("commit", "-qm", "second", "--allow-empty")
	up, ahead, behind := Tracking(ctx, root)
	if up != "origin/main" || ahead != 1 || behind != 0 {
		t.Fatalf("tracking %q ↑%d ↓%d", up, ahead, behind)
	}
}

func TestManageRemotes(t *testing.T) {
	root, _ := newRepo(t)
	ctx := context.Background()
	if list, err := RemoteList(ctx, root); err != nil || len(list) != 0 {
		t.Fatalf("a new repository's remotes: %v, %v", list, err)
	}
	if err := AddRemote(ctx, root, "upstream", "https://example.com/up.git"); err != nil {
		t.Fatal(err)
	}
	if err := AddRemote(ctx, root, "origin", "git@example.com:me/r.git"); err != nil {
		t.Fatal(err)
	}
	if err := AddRemote(ctx, root, "origin", "https://example.com/other.git"); err == nil {
		t.Fatal("added a remote whose name is taken")
	}
	list, err := RemoteList(ctx, root)
	want := []Remote{{"origin", "git@example.com:me/r.git"}, {"upstream", "https://example.com/up.git"}}
	if err != nil || !slices.Equal(list, want) {
		t.Fatalf("remotes %v, %v; want %v", list, err, want)
	}
	if err := EditRemote(ctx, root, "upstream", "fork", "https://example.com/fork.git"); err != nil {
		t.Fatal(err)
	}
	if err := EditRemote(ctx, root, "origin", "origin", "https://example.com/r.git"); err != nil {
		t.Fatal(err)
	}
	if err := RemoveRemote(ctx, root, "fork"); err != nil {
		t.Fatal(err)
	}
	list, err = RemoteList(ctx, root)
	if want := []Remote{{"origin", "https://example.com/r.git"}}; err != nil || !slices.Equal(list, want) {
		t.Fatalf("after edit and remove: %v, %v; want %v", list, err, want)
	}
	if _, err := RemoteList(ctx, t.TempDir()); err == nil {
		t.Fatal("listed the remotes of a folder outside a repository")
	}
}

func TestCloneAndAdd(t *testing.T) {
	src, run := newRepo(t)
	ctx := context.Background()
	write(t, src, "a", "a\n")
	run("add", "a")
	run("commit", "-qm", "first")
	dir := filepath.Join(t.TempDir(), "nested", CloneName(src+"/.git/"))
	if filepath.Base(dir) != filepath.Base(src) {
		t.Fatalf("clone name %q, want %q", filepath.Base(dir), filepath.Base(src))
	}
	if err := Clone(ctx, src, dir); err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(filepath.Join(dir, "a")); err != nil || string(b) != "a\n" {
		t.Fatalf("cloned file %q, %v", b, err)
	}
	if list, _ := RemoteList(ctx, dir); len(list) != 1 || list[0].Name != "origin" {
		t.Fatalf("the clone's remotes: %v", list)
	}
	if err := Clone(ctx, src, dir); err == nil {
		t.Fatal("cloned into a folder that is not empty")
	}
	write(t, dir, "b", "b\n")
	if err := Add(ctx, dir, "b"); err != nil {
		t.Fatal(err)
	}
	changes, err := WorktreeStatus(ctx, dir)
	if err != nil || len(changes) != 1 || changes[0].Path != "b" || changes[0].Status() != 'A' {
		t.Fatalf("after Add: %+v, %v", changes, err)
	}
}

func TestCloneName(t *testing.T) {
	for in, want := range map[string]string{
		"git@github.com:leemysw/revtrail.git": "revtrail",
		"https://github.com/o/r.git/":         "r",
		"https://example.com/group/project":   "project",
		"/srv/git/repo.git":                   "repo",
		"host:repo":                           "repo",
	} {
		if got := CloneName(in); got != want {
			t.Errorf("CloneName(%q) = %q, want %q", in, got, want)
		}
	}
}
