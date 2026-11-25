// INPUT: 文件系统事件与 git 忽略规则。
// OUTPUT: 工作区监听（含各仓库 .git 的 HEAD、index、refs，按 IntelliJ 不轮询 Git 状态）、事件应用、未被监听的目录（git 忽略的）在窗口回到前台时重读，与树的 git 状态。
// POS: 磁盘变化进入 App 的入口。
// PROTOCOL: 变更时更新此头部，然后检查本包 doc.go

package app

import (
	"context"
	"errors"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo"

	"revtrail/internal/git"
	"revtrail/internal/workspace"
)

// watchFiles starts a watch; it is mygo.WatchFiles in the app, and a
// test's own in tests.
var watchFiles = func(ctx context.Context, root string, opts mygo.FileWatchOptions) (fileWatcher, error) {
	return mygo.WatchFiles(ctx, root, opts)
}

type fileWatcher interface {
	Next(ctx context.Context) ([]mygo.FileEvent, error)
	Close() error
}

// startWatch follows changes in the workspace in the background. A tree
// too large to watch in full falls back to coarse hints; when watching
// fails altogether, reading still works and Refresh reads again.
func (a *App) startWatch() {
	if watchFiles == nil {
		return
	}
	root, ctx, gen := a.root, a.wsCtx, a.gen
	go func() {
		// What git ignores is left out first, as build output, so that a
		// large project stays within what a watch can follow file by file.
		// Single files git ignores are still watched, as an .env open in
		// a tab.
		ignored := readIgnored(ctx, root)
		a.post.Post(func() {
			if a.gen == gen {
				a.ignored = ignored
			}
		})
		ignore := func(rel string, isDir bool) bool {
			return isDir && watchIgnored(rel, ignored)
		}
		w, err := watchFiles(ctx, root, mygo.FileWatchOptions{Recursive: true, Ignore: ignore})
		if errors.Is(err, mygo.ErrWatchLimit) {
			a.logf("watch: too many entries, falling back to hints")
			w, err = watchFiles(ctx, root, mygo.FileWatchOptions{Recursive: true, Ignore: ignore, HintsOnly: true})
		}
		a.post.Post(func() {
			if a.gen == gen {
				a.watching, a.watchErr = err == nil, err
			}
		})
		if err != nil {
			return
		}
		defer w.Close()
		for {
			evs, err := w.Next(ctx)
			if err != nil {
				if ctx.Err() == nil {
					if errors.Is(err, io.EOF) {
						err = errors.New("the folder was removed or moved")
					}
					a.post.Post(func() {
						if a.gen == gen {
							a.watching, a.watchErr = false, err
						}
					})
				}
				return
			}
			a.post.Post(func() {
				if a.gen == gen {
					a.applyEvents(evs)
				}
			})
		}
	}()
}

// applyEvents refreshes what changes on disk touch: the loaded
// directories they are in, and the open documents they name.
func (a *App) applyEvents(evs []mygo.FileEvent) {
	if len(evs) > 0 {
		a.git.touched = true
		a.refreshGit()
		a.invalidateHover()
	}
	dirs := map[string]bool{}
	for _, ev := range evs {
		if path.Base(ev.Path) == ".gitignore" || path.Base(ev.Path) == "exclude" && strings.Contains(ev.Path, ".git/info") {
			a.rereadIgnored()
		}
		p := filepath.Join(a.root, filepath.FromSlash(ev.Path))
		switch ev.Op {
		case mygo.FileHint:
			// Something under p changed: read again what is loaded there,
			// and the documents in it.
			for dir := range a.dirs {
				if workspace.Within(p, dir) {
					dirs[dir] = true
				}
			}
			for _, d := range a.docs {
				if workspace.Within(p, d.path) {
					a.reloadDoc(d)
				}
			}
			continue
		case mygo.FileRename:
			old := filepath.Join(a.root, filepath.FromSlash(ev.OldPath))
			dirs[filepath.Dir(old)] = true
			a.removed(old)
		case mygo.FileRemove:
			a.removed(p)
		case mygo.FileCreate, mygo.FileWrite:
			if d := a.docs[p]; d != nil {
				a.reloadDoc(d)
			}
		}
		if ev.Path != "." {
			dirs[filepath.Dir(p)] = true
		}
		if ev.IsDir {
			dirs[p] = true
		}
	}
	for dir := range dirs {
		a.refreshDir(dir)
	}
	if len(evs) > 0 {
		a.quick.files = nil // Open Quickly lists them again when next opened
		a.filter.forQuery = ""
	}
}

// removed marks the documents at or under p as gone; their tabs stay
// until the user closes them.
func (a *App) removed(p string) {
	for _, d := range a.docs {
		if workspace.Within(p, d.path) {
			if d.buf != nil {
				d.gone = true
			} else {
				a.reloadDoc(d) // shows why it cannot be read
			}
		}
	}
	// The folder itself and those under it: their rows go once their
	// parent is read again.
	for dir := range a.dirs {
		if workspace.Within(p, dir) {
			delete(a.dirs, dir)
		}
	}
}

// unwatched reports whether changes under p come without events: the
// watch leaves out what git ignores, folders whole, or watching failed.
func (a *App) unwatched(p string) bool {
	if !a.watching {
		return true
	}
	rel, err := filepath.Rel(a.root, p)
	if err != nil || rel == "." {
		return false
	}
	return watchIgnored(filepath.ToSlash(rel), a.ignored)
}

// watchIgnored is whether the watch leaves the folder rel out. Of a
// repository's .git it keeps what IntelliJ's GitRepositoryUpdater
// follows: HEAD, the index and the other files at its top, refs and
// info; objects and logs stay out, as they change with every refresh.
func watchIgnored(rel string, ignored *git.Ignored) bool {
	if before, after, ok := strings.Cut(rel+"/", ".git/"); ok && (before == "" || strings.HasSuffix(before, "/")) {
		before = strings.TrimSuffix(before, "/")
		if before != "" && (workspace.IgnoredRel(before) || ignored.Dir(before)) {
			return true
		}
		after = strings.TrimSuffix(after, "/")
		return !(after == "" || after == "info" || after == "refs" || strings.HasPrefix(after, "refs/"))
	}
	return workspace.IgnoredRel(rel) || ignored.Dir(rel)
}

// gitUnwatched is whether a repository's HEAD or index can change with
// no event: its .git is left out of the watch, or is a file pointing
// elsewhere, as a worktree's or a submodule's.
func (a *App) gitUnwatched() bool {
	for _, r := range a.git.roots {
		meta := filepath.Join(r, ".git")
		if fi, err := os.Stat(meta); err != nil || !fi.IsDir() || a.unwatched(meta) {
			return true
		}
	}
	return len(a.git.roots) == 0
}

// refreshUnwatched reads again the loaded folders and open documents the
// watch does not follow, as IntelliJ refreshes its files when its window
// comes to the front.
func (a *App) refreshUnwatched() {
	for dir := range a.dirs {
		if a.unwatched(dir) {
			a.refreshDir(dir)
		}
	}
	for _, d := range a.docs {
		if !d.untitled && d.buf != nil && a.unwatched(filepath.Dir(d.path)) {
			a.reloadDoc(d)
		}
	}
}

// readIgnored is what git ignores in root, nothing outside a repository;
// tests leave git out of their watches.
var readIgnored = func(ctx context.Context, root string) *git.Ignored {
	ig, _ := git.ReadIgnored(ctx, root)
	return ig
}

// rereadIgnored reads what git ignores again after a .gitignore changed,
// for the tree; the watch keeps what it was started with until the
// workspace opens again.
func (a *App) rereadIgnored() {
	root, ctx, gen := a.root, a.wsCtx, a.gen
	go func() {
		ig := readIgnored(ctx, root)
		a.post.Post(func() {
			if a.gen == gen {
				a.ignored = ig
			}
		})
	}()
}

// gitIgnores reports whether git ignores p, a path of the workspace,
// which the tree dims, as VS Code does.
func (a *App) gitIgnores(p string) bool {
	if a.ignored == nil || !workspace.Within(a.root, p) || p == a.root {
		return false
	}
	rel, err := filepath.Rel(a.root, p)
	return err == nil && a.ignored.Has(filepath.ToSlash(rel))
}

// treeGitStatus resolves ownership before applying ignore rules. A parent
// repository can ignore the directory containing an entire child repository.
func (a *App) treeGitStatus(p string) (root string, status byte) {
	real := workspace.DocPath(a.realRoot, a.root, p)
	for dir := real; ; dir = filepath.Dir(dir) {
		if _, ok := a.git.repositories[dir]; ok {
			root = dir
			break
		}
		if dir == filepath.Dir(dir) {
			break
		}
	}
	repo, known := a.git.repositories[root]
	if !known {
		if a.gitIgnores(p) {
			return root, 'I'
		}
		return root, 0
	}
	rel, _ := filepath.Rel(root, real)
	name := filepath.ToSlash(rel)
	if status := repo.files[name]; status != 0 {
		return root, status
	}
	for dir := path.Dir(name); dir != "."; dir = path.Dir(dir) {
		if repo.files[dir] == 'I' {
			return root, 'I'
		}
	}
	return root, 0
}
