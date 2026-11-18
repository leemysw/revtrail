package git

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestApplyMergePreservesOtherPathsAndDetectsStaleResult(t *testing.T) {
	ctx, root := context.Background(), t.TempDir()
	run := func(args ...string) string {
		t.Helper()
		out, err := Command(ctx, root, args...)
		if err != nil {
			t.Fatal(args, err)
		}
		return string(out)
	}
	write := func(name, text string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(root, name), []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
	}
	run("init", "-q", "-b", "main")
	run("config", "user.name", "Test")
	run("config", "user.email", "test@example.com")
	run("config", "commit.gpgsign", "false")
	write("a", "base\n")
	write("gone", "old\n")
	write("bin", "\x00base")
	run("add", ".")
	run("commit", "-qm", "base")
	run("switch", "-c", "topic")
	write("a", "right\n")
	write("bin", "\x00right")
	run("rm", "gone")
	run("commit", "-qam", "incoming")
	run("switch", "main")
	write("a", "left\n")
	write("gone", "changed\n")
	write("bin", "\x00left")
	run("commit", "-qam", "local")
	if _, err := Command(ctx, root, "merge", "topic"); err == nil {
		t.Fatal("expected conflict")
	}
	read := func(name string) *MergeFile {
		t.Helper()
		m, err := ReadMerge(ctx, root, name)
		if err != nil {
			t.Fatal(err)
		}
		return m
	}
	m := read("a")
	if len(m.Parts) != 1 || m.Parts[0].Base != "base\n" || m.Parts[0].Left != "left\n" || m.Parts[0].Right != "right\n" {
		t.Fatalf("wrong conflict: %+v", m.Parts)
	}
	write("a", "new manual edit")
	if m.Apply(ctx, root, "result", "stale") == nil {
		t.Fatal("overwrote external edit")
	}
	if err := read("a").Apply(ctx, root, "result", "merged\n"); err != nil {
		t.Fatal(err)
	}
	if run("show", ":a") != "merged\n" || run("ls-files", "-u", "--", "a") != "" || !strings.Contains(run("ls-files", "-u"), "bin") {
		t.Fatal("wrong staged resolution")
	}
	if err := read("bin").Apply(ctx, root, "right", ""); err != nil {
		t.Fatal(err)
	}
	if got := run("show", ":bin"); got != "\x00right" {
		t.Fatalf("binary: %q", got)
	}
	if err := read("gone").Apply(ctx, root, "right", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "gone")); !os.IsNotExist(err) || run("ls-files", "-u") != "" {
		t.Fatal("delete resolution failed")
	}
}
