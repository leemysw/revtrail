// INPUT: 工作区目录。
// OUTPUT: Roots：工作区内的 git 仓库根。
// POS: 多仓库发现。
// PROTOCOL: 变更时更新此头部，然后检查本包 doc.go

package git

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"revtrail/internal/workspace"
)

// Roots includes the enclosing worktree as well as nested repositories. A .git
// file is a marker too (submodules and linked worktrees).
func Roots(ctx context.Context, dir string) []string {
	if resolved, err := filepath.EvalSymlinks(dir); err == nil {
		dir = resolved
	}
	var roots []string
	if out, err := Command(ctx, dir, "rev-parse", "--show-toplevel"); err == nil {
		roots = append(roots, filepath.FromSlash(strings.TrimSpace(string(out))))
	}
	filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if ctx.Err() != nil {
			return fs.SkipAll
		}
		if err != nil {
			return nil
		}
		if path != dir && d.Name() == ".git" {
			root := filepath.Dir(path)
			if !slices.Contains(roots, root) && d.Type()&os.ModeSymlink == 0 {
				if out, err := Command(ctx, root, "rev-parse", "--show-toplevel"); err == nil && filepath.FromSlash(strings.TrimSpace(string(out))) == root {
					roots = append(roots, root)
				}
			}
			if d.IsDir() {
				return fs.SkipDir
			}
		}
		if path != dir && d.IsDir() && workspace.Ignored(d.Name()) {
			return fs.SkipDir
		}
		return nil
	})
	return roots
}
