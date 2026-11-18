// INPUT: git 可执行文件与仓库根。
// OUTPUT: Command / Network / run：统一环境的 git 进程；状态解析与 Change。
// POS: 本包唯一启动 git 进程的地方（arch 测试约束）。
// PROTOCOL: 变更时更新此头部，然后检查本包 doc.go

package git

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Change is a path git status reports, with its status in the index and
// in the working tree as porcelain's two letters ('?' both for untracked).
// OldPath is where a renamed or copied path came from; both are relative
// to Root, the repository WorktreeStatus read it in.
type Change struct {
	Root          string
	Path, OldPath string
	Index, Work   byte
	Blocks        BlockSelection
}

// Command runs git in dir, without locks, prompts, pagers or pathspec
// magic, giving up after 30 seconds. Its error holds git's output.
func Command(ctx context.Context, dir string, args ...string) ([]byte, error) {
	return run(ctx, dir, 30*time.Second, args)
}

// Network runs git in dir as Command does, for a command that talks to a
// remote: it gives up after five minutes rather than thirty seconds.
func Network(ctx context.Context, dir string, args ...string) ([]byte, error) {
	return run(ctx, dir, 5*time.Minute, args)
}

func run(ctx context.Context, dir string, timeout time.Duration, args []string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	out, err := command(ctx, dir, args...).CombinedOutput()
	if err != nil {
		return out, fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	return out, nil
}

// input runs git in dir with input on its standard input, and returns
// its standard output alone.
func input(ctx context.Context, dir, input string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	cmd := command(ctx, dir, args...)
	cmd.Stdin = strings.NewReader(input)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return out, fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}

// command is git with the environment every call shares. Messages are
// English so the errors this package recognizes read the same under any
// locale, and an editor git would open accepts its default text.
func command(ctx context.Context, dir string, args ...string) *exec.Cmd {
	args = append([]string{"-c", "core.fsmonitor=false", "-C", dir}, args...)
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Env = append(cmd.Environ(), "GIT_OPTIONAL_LOCKS=0", "GIT_TERMINAL_PROMPT=0", "GIT_PAGER=cat", "GIT_LITERAL_PATHSPECS=1",
		"GIT_EDITOR=true", "LANGUAGE=en", "LC_ALL=en_US.UTF-8", "LANG=en_US.UTF-8")
	return cmd
}

// Read returns the root of the repository holding dir, its branch (or
// HEAD's short hash when detached) and its changes, untracked files among
// them.
func Read(ctx context.Context, dir string) (root, branch string, changes []Change, err error) {
	out, err := Command(ctx, dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", "", nil, err
	}
	root = filepath.FromSlash(strings.TrimSpace(string(out)))
	if branch, err = CurrentBranch(ctx, root); err != nil {
		return "", "", nil, err
	}
	if changes, err = WorktreeStatus(ctx, root); err != nil {
		return "", "", nil, err
	}
	return root, branch, changes, nil
}

// CurrentBranch is the branch HEAD is on, or its short hash when detached.
func CurrentBranch(ctx context.Context, root string) (string, error) {
	b, err := Command(ctx, root, "symbolic-ref", "--quiet", "--short", "HEAD")
	if err != nil {
		b, err = Command(ctx, root, "rev-parse", "--short", "HEAD")
	}
	return strings.TrimSpace(string(b)), err
}

// WorktreeStatus includes ignored entries as !!; --ignored=matching keeps
// ignored directories compact while reporting individually ignored files.
func WorktreeStatus(ctx context.Context, root string) ([]Change, error) {
	out, err := Command(ctx, root, "status", "--porcelain=v1", "-z", "--untracked-files=all", "--ignored=matching")
	if err != nil {
		return nil, err
	}
	changes := ParseStatus(out)
	for i := range changes {
		changes[i].Root = root
	}
	return changes, nil
}

// Head returns the commit HEAD names, or "" in a repository without
// commits yet.
func Head(ctx context.Context, root string) string {
	out, err := Command(ctx, root, "rev-parse", "--verify", "--quiet", "HEAD")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// ParseStatus parses the output of git status --porcelain=v1 -z.
func ParseStatus(out []byte) []Change {
	parts := strings.Split(string(out), "\x00")
	var changes []Change
	for i := 0; i < len(parts); i++ {
		p := parts[i]
		if len(p) < 4 {
			continue
		}
		ch := Change{Path: p[3:], Index: p[0], Work: p[1]}
		if p[0] == 'R' || p[0] == 'C' || p[1] == 'R' || p[1] == 'C' {
			i++
			if i < len(parts) {
				ch.OldPath = parts[i]
			}
		}
		changes = append(changes, ch)
	}
	return changes
}

// Status is the one letter a file tree shows for the change: U for
// untracked, ! for a conflict, else the working tree's letter, or the
// index's when the working tree has none.
func (ch Change) Status() byte {
	if ch.Index == '?' {
		return 'U'
	}
	if ch.Index == 'U' || ch.Work == 'U' || (ch.Index == 'A' && ch.Work == 'A') || (ch.Index == 'D' && ch.Work == 'D') {
		return '!'
	}
	if ch.Work != ' ' {
		return ch.Work
	}
	return ch.Index
}

// Blob returns the contents of path at rev.
func Blob(ctx context.Context, root, rev, path string) ([]byte, error) {
	return Command(ctx, root, "cat-file", "blob", rev+":"+path)
}

// DiscardTrackedChange restores a tracked or staged change to HEAD.
func DiscardTrackedChange(ctx context.Context, root string, ch Change) error {
	if ch.Index == 'A' || ch.Index == 'C' {
		_, err := Command(ctx, root, "rm", "--cached", "-f", "--", ch.Path)
		return err
	}
	args := []string{"restore", "--source=HEAD", "--staged", "--worktree", "--", ch.Path}
	if ch.OldPath != "" {
		args = append(args, ch.OldPath)
	}
	_, err := Command(ctx, root, args...)
	return err
}
