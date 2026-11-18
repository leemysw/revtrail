// INPUT: 仓库根与工作树路径。
// OUTPUT: Worktree、Worktrees、AddWorktree、RemoveWorktree：git worktree 的列出、新建与删除。
// POS: Git 工具窗口 Worktrees 标签页的数据来源。
// PROTOCOL: 变更时更新此头部，然后检查本包 doc.go

package git

import (
	"context"
	"path/filepath"
	"strings"
)

// Worktree is one of a repository's working trees; the first Worktrees
// lists is the main one.
type Worktree struct {
	Path, Head, Branch string // Branch: the short name, "" when detached
	Main, Locked       bool
	Prunable           bool // its folder is gone
}

// Worktrees lists the repository's working trees, the main one first.
func Worktrees(ctx context.Context, root string) ([]Worktree, error) {
	out, err := Command(ctx, root, "worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}
	var list []Worktree
	for block := range strings.SplitSeq(strings.TrimSpace(string(out)), "\n\n") {
		var w Worktree
		bare := false
		for line := range strings.SplitSeq(block, "\n") {
			key, value, _ := strings.Cut(line, " ")
			switch key {
			case "worktree":
				w.Path = filepath.FromSlash(value)
			case "HEAD":
				w.Head = value
			case "branch":
				w.Branch = strings.TrimPrefix(value, "refs/heads/")
			case "bare":
				bare = true
			case "locked":
				w.Locked = true
			case "prunable":
				w.Prunable = true
			}
		}
		if w.Path == "" || bare {
			continue
		}
		w.Main = len(list) == 0
		list = append(list, w)
	}
	return list, nil
}

// AddWorktree checks branch out in a new working tree at path; create
// makes the branch, from HEAD, first.
func AddWorktree(ctx context.Context, root, path, branch string, create bool) error {
	if err := checkRevision(branch); err != nil {
		return err
	}
	if create {
		return local(ctx, root, "worktree", "add", "-b", branch, "--", path)
	}
	return local(ctx, root, "worktree", "add", "--", path, branch)
}

// RemoveWorktree deletes a linked working tree; force also removes one
// with changes.
func RemoveWorktree(ctx context.Context, root, path string, force bool) error {
	args := []string{"worktree", "remove"}
	if force {
		args = append(args, "--force")
	}
	return local(ctx, root, append(args, "--", path)...)
}
