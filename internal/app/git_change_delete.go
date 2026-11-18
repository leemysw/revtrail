// INPUT: 改动列表中选中的文件与工作区。
// OUTPUT: 删除改动文件、可撤销的删除与恢复。
// POS: Commit 工具窗口里的删除操作。
// PROTOCOL: 变更时更新此头部，然后检查本包 doc.go

package app

import (
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"

	"revtrail/internal/workspace"
	"revtrail/locales"
)

// A folder in the changes tree represents only the leaves in its status group.
type changeDeleteTarget struct {
	path   string
	folder string
}

func (a *App) deleteSelectedChanges() {
	if a.git.busy || a.actionConfirm != nil {
		return
	}
	targets := make(map[string]changeDeleteTarget)
	for key := range a.git.selected {
		row := a.git.rows.byKey[key]
		for _, ch := range row.files {
			if ch.Work == 'D' || (ch.Index == 'D' && ch.Work == ' ') {
				continue
			}
			dst := workspace.DocPath(a.root, a.realRoot, filepath.Join(ch.Root, filepath.FromSlash(ch.Path)))
			if dst == a.root || !workspace.Within(a.root, dst) {
				continue
			}
			t := targets[dst]
			t.path = dst
			if row.dir != "" {
				folder := workspace.DocPath(a.root, a.realRoot, filepath.Join(row.root, filepath.FromSlash(row.dir)))
				if workspace.Within(a.root, folder) && folder != a.root && (t.folder == "" || workspace.Within(folder, t.folder)) {
					t.folder = folder
				}
			}
			targets[dst] = t
		}
	}
	if len(targets) == 0 {
		return
	}
	files := slices.Collect(maps.Values(targets))
	slices.SortFunc(files, func(a, b changeDeleteTarget) int { return strings.Compare(a.path, b.path) })
	var dirty []*doc
	for _, d := range a.dirtyDocs() {
		if _, ok := targets[d.path]; ok {
			dirty = append(dirty, d)
		}
	}
	message := locales.TrashFilesQuestion(len(files))
	if len(files) == 1 {
		message = locales.TrashQuestion(filepath.Base(files[0].path))
	}
	if len(a.git.selected) == 1 {
		for key := range a.git.selected {
			if dir := a.git.rows.byKey[key].dir; dir != "" {
				message = locales.TrashFolderChanges(len(files), dir)
			}
		}
	}
	ask := func() {
		q := &confirmation{title: locales.TrashDialogTitle(), message: message, detail: locales.ChangeDeleteRecovery()}
		q.actions = []action{
			{locales.MoveToTrash(), func() {
				a.actionConfirm = nil
				a.git.refocusTree = true
				a.runChangeDeletion(files)
			}},
			{locales.Cancel(), func() { a.actionConfirm = nil; a.git.refocusTree = true }},
		}
		a.actionConfirm = q
	}
	if !a.guardDirty(dirty, ask) {
		ask()
	}
}

// The desktop trash API does not return a restore token. Keep a session copy
// of each successfully trashed leaf, preserving symbolic links as links.
type changeDeletion struct {
	backup         string
	root, realRoot string
	files          map[string]string      // original path -> private copy
	dirs           map[string]fs.FileMode // directories removed after their last changed leaf
}

func (a *App) runChangeDeletion(files []changeDeleteTarget) {
	gen, post, trash := a.gen, a.post.Post, a.host.TrashItem
	root, realRoot := a.root, a.realRoot
	a.git.busy = true
	go func() {
		batch, err := trashChangedFiles(root, realRoot, files, trash)
		post(func() {
			if gen != a.gen {
				batch.cleanup()
				return
			}
			a.git.busy = false
			if len(batch.files) > 0 {
				a.git.deletions = append(a.git.deletions, batch)
				for path := range batch.files {
					a.removed(path)
				}
				for dir := range batch.dirs {
					a.removed(dir)
				}
				a.refreshChangedPaths(batch)
			}
			if err != nil {
				a.note(locales.MsgCouldNotMoveToTrash()+err.Error(), true)
			}
		})
	}()
}

func (b changeDeletion) cleanup() {
	if b.backup != "" {
		_ = os.RemoveAll(b.backup)
	}
}

func trashChangedFiles(root, realRoot string, targets []changeDeleteTarget, trash func(string) error) (changeDeletion, error) {
	backup, err := os.MkdirTemp("", "revtrail-undo-")
	if err != nil {
		return changeDeletion{}, err
	}
	b := changeDeletion{backup: backup, root: root, realRoot: realRoot, files: map[string]string{}, dirs: map[string]fs.FileMode{}}
	dirs := map[string]bool{}
	var failure error
	for i, t := range targets {
		if err = b.checkParent(t.path); err != nil {
			failure = err
			break
		}
		info, err := os.Lstat(t.path)
		if err != nil {
			failure = err
			break
		}
		if !info.Mode().IsRegular() && info.Mode()&os.ModeSymlink == 0 {
			failure = fmt.Errorf("%s: %w", t.path, errors.ErrUnsupported)
			break
		}
		snapshot := filepath.Join(backup, fmt.Sprint(i))
		if err = linkOrCopyChange(t.path, snapshot); err == nil {
			err = trash(t.path)
		}
		if err != nil {
			failure = err
			break
		}
		b.files[t.path] = snapshot
		if t.folder != "" {
			for dir := filepath.Dir(t.path); workspace.Within(t.folder, dir); dir = filepath.Dir(dir) {
				dirs[dir] = true
				if dir == t.folder {
					break
				}
			}
		}
	}
	for _, dir := range slices.Backward(slices.Sorted(maps.Keys(dirs))) {
		if err := b.checkParent(dir); err != nil {
			failure = errors.Join(failure, err)
			continue
		}
		info, err := os.Lstat(dir)
		if err != nil {
			continue
		}
		if err = syscall.Rmdir(dir); err == nil {
			b.dirs[dir] = info.Mode().Perm()
		} else if !errors.Is(err, syscall.ENOTEMPTY) && !errors.Is(err, fs.ErrExist) {
			failure = errors.Join(failure, err)
		}
	}
	if len(b.files) == 0 {
		b.cleanup()
	}
	return b, failure
}

// A directory may have been replaced by a symlink since status or deletion.
func (b changeDeletion) checkParent(path string) error {
	parent := filepath.Dir(path)
	actual, err := filepath.EvalSymlinks(parent)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(b.root, parent)
	if err != nil {
		return err
	}
	if actual != filepath.Join(b.realRoot, rel) {
		return fmt.Errorf("%s: %w", parent, fs.ErrInvalid)
	}
	return nil
}

func linkOrCopyChange(src, dst string) error {
	// On the same volume a hard link retains permissions and extended attributes.
	if err := os.Link(src, dst); err == nil || errors.Is(err, fs.ErrExist) {
		return err
	}
	return copyEntry(src, dst)
}

func (g *gitState) clearDeletions() {
	for _, batch := range g.deletions {
		batch.cleanup()
	}
	g.deletions = nil
}

func (a *App) undoChangeDeletion() {
	if a.git.busy || len(a.git.deletions) == 0 {
		return
	}
	i := len(a.git.deletions) - 1
	b := a.git.deletions[i]
	a.git.deletions = a.git.deletions[:i]
	gen, post := a.gen, a.post.Post
	a.git.busy = true
	go func() {
		pending, err := b.restore()
		post(func() {
			if gen != a.gen {
				b.cleanup()
				return
			}
			a.git.busy = false
			if len(pending.files) > 0 {
				a.git.deletions = append(a.git.deletions, pending)
			} else {
				b.cleanup()
			}
			for path := range b.files {
				if _, failed := pending.files[path]; !failed {
					if d := a.docs[path]; d != nil {
						a.reloadDoc(d)
					}
				}
			}
			a.refreshChangedPaths(b)
			if err != nil {
				a.note(err.Error(), true)
			}
		})
	}()
}

func (b changeDeletion) restore() (changeDeletion, error) {
	pending := b
	pending.files = maps.Clone(b.files)
	for _, dir := range slices.Sorted(maps.Keys(b.dirs)) {
		if err := b.checkParent(dir); err != nil {
			return pending, err
		}
		if err := os.Mkdir(dir, b.dirs[dir]); err != nil {
			info, statErr := os.Lstat(dir)
			if !errors.Is(err, fs.ErrExist) || statErr != nil || !info.IsDir() {
				return pending, err
			}
		}
	}
	var failures []error
	for path, snapshot := range b.files {
		err := b.checkParent(path)
		if err == nil {
			err = linkOrCopyChange(snapshot, path)
		}
		if err != nil {
			failures = append(failures, err)
			continue
		}
		delete(pending.files, path)
	}
	return pending, errors.Join(failures...)
}

func (a *App) refreshChangedPaths(b changeDeletion) {
	parents := map[string]bool{}
	for path := range b.files {
		parents[filepath.Dir(path)] = true
	}
	for path := range b.dirs {
		parents[filepath.Dir(path)] = true
	}
	for dir := range parents {
		a.refreshDir(dir)
	}
	a.quick.files = nil
	a.filter.forQuery = ""
	a.refreshGit()
}
