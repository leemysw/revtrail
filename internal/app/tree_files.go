// INPUT: 项目树选择、平台剪贴板与 git。
// OUTPUT: 多选、剪贴、粘贴移动、Copy / Move 对话框、比较文件、Git 子菜单与项目树右键菜单。
// POS: IntelliJ 项目树文件管理。
// PROTOCOL: 变更时更新此头部，然后检查本包 doc.go

package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"revtrail/internal/git"
	"revtrail/internal/workspace"
	"revtrail/locales"
)

// setTreeRow chooses row i of the tree alone, as the app moving the
// choice: revealing a file, a search match, the row clicked.
func (a *App) setTreeRow(i int) {
	a.treeRow = i
	a.treeSel.Clear()
	if i >= 0 && i < a.tree.Rows() {
		a.treeSel.Add(a.tree.Item(i))
	}
}

// clickTreeRow chooses the row of p as a click with mods does: alone,
// Cmd adding it or taking it out, Shift choosing the rows from the one
// last clicked, as IntelliJ's project view.
func (a *App) clickTreeRow(p string, mods ui.Modifiers) {
	at := -1
	for i := range a.tree.Rows() {
		if a.tree.Item(i) == p {
			at = i
			break
		}
	}
	if at < 0 {
		return
	}
	anchor := -1
	for i := range a.tree.Rows() {
		if a.tree.Item(i) == a.treeAnchor {
			anchor = i
			break
		}
	}
	switch {
	case mods&ui.Shift != 0 && anchor >= 0:
		a.treeSel.Clear()
		for i := min(anchor, at); i <= max(anchor, at); i++ {
			if q := a.tree.Item(i); !isPlaceholder(q) {
				a.treeSel.Add(q)
			}
		}
		a.treeRow = at
		return
	case mods&ui.Cmd != 0:
		if a.treeSel.Len() == 0 && a.treeRow >= 0 && a.treeRow < a.tree.Rows() {
			a.treeSel.Add(a.tree.Item(a.treeRow))
		}
		if a.treeSel.Has(p) {
			a.treeSel.Remove(p)
		} else {
			a.treeSel.Add(p)
		}
		a.treeRow = at
	default:
		a.setTreeRow(at)
	}
	a.treeAnchor = p
}

// treeChosen returns the items chosen in the tree, in its order: those
// of the multiple choice, else the row chosen. The project's own folder
// is never one: cutting, moving or deleting it is not the tree's to do.
func (a *App) treeChosen() []string {
	var out []string
	for i := range a.tree.Rows() {
		if p := a.tree.Item(i); a.treeSel.Has(p) && !isPlaceholder(p) && p != a.root {
			out = append(out, p)
		}
	}
	if len(out) == 0 && a.treeRow >= 0 && a.treeRow < a.tree.Rows() {
		if p := a.tree.Item(a.treeRow); !isPlaceholder(p) && p != a.root {
			out = []string{p}
		}
	}
	return outermost(out)
}

// outermost drops the paths inside another of paths, which acting on
// that one covers.
func outermost(paths []string) []string {
	return slices.DeleteFunc(slices.Clone(paths), func(p string) bool {
		return slices.ContainsFunc(paths, func(q string) bool { return q != p && workspace.Within(q, p) })
	})
}

// treeTargets is what a tree menu opened on p acts on: the items chosen
// when p is one of them, else p alone, as IntelliJ's project view.
func (a *App) treeTargets(p string) []string {
	if chosen := a.treeChosen(); slices.Contains(chosen, p) {
		return chosen
	}
	return []string{p}
}

// copyFiles puts paths on the clipboard as files, which Paste copies.
func (a *App) copyFiles(paths []string) {
	a.cutFiles = nil
	if err := a.host.CopyFiles(paths...); err != nil {
		a.note(locales.MsgCouldNotCopyFile()+describeErr(err), true)
	}
}

// cutTreeFiles puts paths on the clipboard as files Paste moves, as
// IntelliJ's Cut in the project view.
func (a *App) cutTreeFiles(paths []string) {
	a.copyFiles(paths)
	a.cutFiles = slices.Clone(paths)
}

// pasteInto copies the clipboard's files into dir, or moves them when
// they are the ones cut.
func (a *App) pasteInto(dir string, paths []string) {
	if len(paths) == 0 {
		return
	}
	if len(a.cutFiles) > 0 && slices.Equal(slices.Sorted(slices.Values(paths)), slices.Sorted(slices.Values(a.cutFiles))) {
		a.cutFiles = nil
		a.moveInto(dir, paths)
		return
	}
	a.copyInto(dir, paths)
}

// clipboardFiles returns the files the clipboard holds.
func clipboardFiles(services ui.Services) []string {
	paths, _ := services.ReadClipboardData(transfer.FileList, transfer.URIList).Files()
	return paths
}

// moveInto moves paths into dir, as IntelliJ's Move and a drag in the
// project view, keeping the tabs of the files moved.
func (a *App) moveInto(dir string, paths []string) {
	var moves [][2]string
	for _, src := range paths {
		src = filepath.Clean(src)
		dst := filepath.Join(dir, filepath.Base(src))
		if dst == src || src == a.root {
			continue
		}
		moves = append(moves, [2]string{src, dst})
	}
	if len(moves) == 0 {
		return
	}
	var dirty []*doc
	for _, d := range a.dirtyDocs() {
		if slices.ContainsFunc(moves, func(m [2]string) bool { return workspace.Within(m[0], d.path) }) {
			dirty = append(dirty, d)
		}
	}
	if a.guardDirty(dirty, func() { a.moveInto(dir, paths) }) {
		return
	}
	gen, post := a.gen, a.post.Post
	go func() {
		var done [][2]string
		var failures []string
		for _, m := range moves {
			if err := moveEntry(m[0], m[1]); err != nil {
				failures = append(failures, fmt.Sprintf("%s: %v", filepath.Base(m[0]), err))
				continue
			}
			done = append(done, m)
		}
		post(func() {
			if a.gen != gen {
				return
			}
			for _, m := range done {
				a.renamed(m[0], m[1])
				a.refreshDir(filepath.Dir(m[0]))
			}
			a.refreshDir(dir)
			a.tree.Open.Add(dir)
			a.quick.files = nil
			a.filter.forQuery = ""
			a.refreshGit()
			if len(done) == 1 {
				a.revealInTree(done[0][1])
			}
			if len(failures) > 0 {
				a.note(locales.ProjectCouldNotMove(strings.Join(failures, "; ")), true)
			}
		})
	}()
}

// moveEntry moves src to dst, which must not exist, and never into src.
func moveEntry(src, dst string) error {
	if workspace.Within(src, filepath.Dir(dst)) {
		return errors.New("cannot move a folder into itself")
	}
	if _, err := os.Lstat(dst); err == nil {
		return errors.New("an item with that name already exists")
	} else if !os.IsNotExist(err) {
		return err
	}
	return os.Rename(src, dst)
}

// requestTrashAll moves paths to the Trash after asking once, as
// IntelliJ's Delete in the project view.
func (a *App) requestTrashAll(paths []string) {
	paths = slices.DeleteFunc(slices.Clone(paths), func(p string) bool { return p == a.root || !workspace.Within(a.root, p) })
	switch len(paths) {
	case 0:
		return
	case 1:
		a.requestTrash(paths[0])
		return
	}
	if a.actionConfirm != nil {
		return
	}
	var dirty []*doc
	for _, d := range a.dirtyDocs() {
		if slices.ContainsFunc(paths, func(p string) bool { return workspace.Within(p, d.path) }) {
			dirty = append(dirty, d)
		}
	}
	if a.guardDirty(dirty, func() { a.requestTrashAll(paths) }) {
		return
	}
	q := &confirmation{title: locales.TrashDialogTitle(), message: locales.TrashItemsQuestion(len(paths)), detail: locales.TrashRecoveryDetail()}
	q.actions = []action{
		{locales.MoveToTrash(), func() {
			if a.actionConfirm != q {
				return
			}
			a.actionConfirm = nil
			gen, trash, post := a.gen, a.host.TrashItem, a.post.Post
			go func() {
				var gone []string
				var failures []string
				for _, p := range paths {
					if err := trash(p); err != nil {
						failures = append(failures, fmt.Sprintf("%s: %v", filepath.Base(p), err))
						continue
					}
					gone = append(gone, p)
				}
				post(func() {
					if a.gen != gen {
						return
					}
					for _, p := range gone {
						a.removed(p)
						a.refreshDir(filepath.Dir(p))
					}
					a.refreshGit()
					a.quick.files = nil
					a.filter.forQuery = ""
					if len(failures) > 0 {
						a.note(locales.MsgCouldNotMoveToTrash()+strings.Join(failures, "; "), true)
					}
				})
			}()
		}},
		{locales.Cancel(), func() {
			if a.actionConfirm == q {
				a.actionConfirm = nil
			}
		}},
	}
	a.actionConfirm = q
}

// promptCopyTo is IntelliJ's Copy (F5) of one item: a new name, or a
// path from the workspace, to copy it to.
func (a *App) promptCopyTo(p string) {
	if p == a.root || !workspace.Within(a.root, p) {
		return
	}
	rel := filepath.ToSlash(workspace.RelPath(a.root, filepath.Dir(p)))
	name := filepath.Base(p)
	value := name
	if rel != "." {
		value = rel + "/" + name
	}
	a.ask(&gitPrompt{
		title: locales.ProjectCopyTitle(name), detail: locales.ProjectCopyDetail(), value: value, submitLabel: locales.Copy(),
		submit: func(to string, _ bool) {
			dst, ok := a.workspacePath(to)
			if !ok {
				a.note(locales.ProjectPathOutside(), true)
				return
			}
			a.copyAs(p, dst)
		},
	})
}

// promptMoveTo is IntelliJ's Move (F6): a folder of the workspace to move
// the items into.
func (a *App) promptMoveTo(paths []string) {
	paths = slices.DeleteFunc(slices.Clone(paths), func(p string) bool { return p == a.root || !workspace.Within(a.root, p) })
	if len(paths) == 0 {
		return
	}
	title := locales.ProjectMoveTitle(filepath.Base(paths[0]))
	if len(paths) > 1 {
		title = locales.ProjectMoveItemsTitle(len(paths))
	}
	rel := filepath.ToSlash(workspace.RelPath(a.root, filepath.Dir(paths[0])))
	if rel == "." {
		rel = ""
	}
	a.ask(&gitPrompt{
		title: title, detail: locales.ProjectMoveDetail(), value: rel, placeholder: locales.ProjectRootPlaceholder(), optional: true, submitLabel: locales.ProjectMove(),
		submit: func(to string, _ bool) {
			dir, ok := a.workspacePath(to)
			if !ok {
				a.note(locales.ProjectPathOutside(), true)
				return
			}
			if info, err := os.Stat(dir); err != nil || !info.IsDir() {
				a.note(locales.ProjectNotAFolder(to), true)
				return
			}
			a.moveInto(dir, paths)
		},
	})
}

// workspacePath resolves a path a dialog took, relative to the
// workspace, reporting whether it stays inside it.
func (a *App) workspacePath(rel string) (string, bool) {
	rel = strings.TrimSpace(rel)
	p := rel
	if !filepath.IsAbs(p) {
		p = filepath.Join(a.root, filepath.FromSlash(rel))
	}
	p = filepath.Clean(p)
	return p, workspace.Within(a.root, p)
}

// copyAs copies src to dst, which must not exist.
func (a *App) copyAs(src, dst string) {
	gen, post := a.gen, a.post.Post
	go func() {
		err := os.MkdirAll(filepath.Dir(dst), 0o755)
		if err == nil {
			err = copyEntry(src, dst)
		}
		post(func() {
			if a.gen != gen {
				return
			}
			if err != nil {
				a.note(locales.MsgCouldNotCopy()+describeErr(err), true)
				return
			}
			a.refreshDir(filepath.Dir(dst))
			a.tree.Open.Add(filepath.Dir(dst))
			a.quick.files = nil
			a.filter.forQuery = ""
			a.refreshGit()
			a.revealInTree(dst)
		})
	}()
}

// compareFiles shows the difference of two files in a diff tab, as
// IntelliJ's Compare Files.
func (a *App) compareFiles(left, right string) {
	a.compareIn(filepath.Base(left)+" ↔ "+filepath.Base(right), "files:"+left+"\x00"+right, func() (string, string, func(), error) {
		return left, right, func() {}, nil
	})
}

// compareTexts shows the difference of two texts in a diff tab, as
// IntelliJ's Compare with Clipboard.
func (a *App) compareTexts(name, key, left, right string) {
	a.compareIn(name, key, func() (string, string, func(), error) {
		dir, err := os.MkdirTemp("", "revtrail-compare-")
		if err != nil {
			return "", "", nil, err
		}
		l, r := filepath.Join(dir, "a"), filepath.Join(dir, "b")
		err = errors.Join(os.WriteFile(l, []byte(left), 0o600), os.WriteFile(r, []byte(right), 0o600))
		return l, r, func() { os.RemoveAll(dir) }, err
	})
}

// compareIn diffs the two files files gives in a diff tab of the focused
// group named name, at key.
func (a *App) compareIn(name, key string, files func() (string, string, func(), error)) {
	g := a.focus
	if g == nil {
		return
	}
	gen, ctx, root := a.gen, a.wsCtx, a.root
	go func() {
		left, right, done, err := files()
		var out []byte
		if err == nil {
			out, err = git.DiffFiles(ctx, root, left, right)
		}
		if done != nil {
			done()
		}
		patches, parseErr := editor.ParsePatch(string(out))
		a.post.Post(func() {
			if a.gen != gen || a.layout.find(g) == nil {
				return
			}
			if err != nil {
				a.note(locales.MsgGitDiff()+gitErrLine(err), true)
				return
			}
			if len(out) == 0 {
				a.note(locales.ProjectFilesIdentical(), false)
				return
			}
			if parseErr != nil || len(patches) == 0 || patches[0].Binary {
				a.note(locales.MsgNoTextDiffAvailableFor()+name, true)
				return
			}
			if g.diffs == nil {
				g.diffs = map[string]*gitDiffTab{}
			}
			key := diffTabPrefix + key
			v := a.newGitDiffTab(patches[0], name, false)
			v.root = ""
			g.diffs[key] = v
			g.addTab(key, len(g.order))
			a.activateIn(g, key)
		})
	}()
}

// changesUnder returns the Git changes of the files at or under paths.
func (a *App) changesUnder(paths []string) []git.Change {
	var out []git.Change
	for _, ch := range a.git.changes {
		if ch.Status() == '!' {
			continue
		}
		if slices.ContainsFunc(paths, func(p string) bool { return workspace.Within(p, changeID(ch)) }) {
			out = append(out, ch)
		}
	}
	return out
}

// commitPaths opens the commit view with the changes under paths alone
// included, as IntelliJ's Commit File… of the project view.
func (a *App) commitPaths(paths []string) {
	if a.git.included == nil {
		a.git.included = map[string]bool{}
	}
	for _, ch := range a.git.changes {
		if ch.Status() == '!' {
			continue
		}
		a.git.included[changeID(ch)] = slices.ContainsFunc(paths, func(p string) bool { return workspace.Within(p, changeID(ch)) })
	}
	a.showCommitNavigator()
}

// addToGitignore adds paths to the .gitignore of their repository, as
// IntelliJ's Add to .gitignore.
func (a *App) addToGitignore(paths []string) {
	gen, post := a.gen, a.post.Post
	type entry struct{ root, rel string }
	var entries []entry
	for _, p := range paths {
		root, rel, ok := a.repoRelativePath(p)
		if !ok {
			continue
		}
		if a.isTreeDir(p) {
			rel += "/"
		}
		entries = append(entries, entry{root, rel})
	}
	go func() {
		var err error
		for _, e := range entries {
			err = errors.Join(err, appendLine(filepath.Join(e.root, ".gitignore"), "/"+e.rel))
		}
		post(func() {
			if a.gen != gen {
				return
			}
			if err != nil {
				a.note(locales.ProjectCouldNotIgnore(describeErr(err)), true)
			}
			a.refreshGit()
			for _, e := range entries {
				a.refreshDir(e.root)
			}
		})
	}()
}

// appendLine adds line to the file at p, on a line of its own.
func appendLine(p, line string) error {
	old, err := os.ReadFile(p)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if slices.Contains(strings.Split(string(old), "\n"), line) {
		return nil
	}
	var b strings.Builder
	b.Write(old)
	if len(old) > 0 && !strings.HasSuffix(string(old), "\n") {
		b.WriteByte('\n')
	}
	b.WriteString(line + "\n")
	return os.WriteFile(p, []byte(b.String()), 0o644)
}

// copyReferenceMenu is IntelliJ's Copy Path/Reference… submenu.
func (a *App) copyReferenceMenu(m *ui.Menu, paths []string) {
	m.Submenu(locales.TabCopyPathReference(), func(sub *ui.Menu) { a.copyReferenceMenuItems(sub, paths) })
}

// openInMenu is IntelliJ's Open In submenu.
func (a *App) openInMenu(m *ui.Menu, p string, dir bool, extra func(*ui.Menu)) {
	m.Submenu(locales.ProjectOpenIn(), func(sub *ui.Menu) {
		label := locales.ProjectFileManager()
		if runtime.GOOS == "darwin" {
			label = "Finder"
		}
		if sub.Item(label).Chosen() {
			mygo.Shell.ShowItemInFolder(p)
		}
		if sub.Item(locales.Terminal()).Chosen() {
			d := p
			if !dir {
				d = filepath.Dir(p)
			}
			a.newTerminalTabIn(d)
		}
		if !dir && sub.Item(locales.ProjectDefaultApp()).Chosen() {
			if err := mygo.Shell.OpenPath(p); err != nil {
				a.note(locales.MsgCouldNotOpen()+describeErr(err), true)
			}
		}
		if extra != nil {
			extra(sub)
		}
	})
}

// treeMenu is the project view's menu of an item, in IntelliJ's order.
func (a *App) treeMenu(m *ui.Menu, p string, isDir bool, services ui.Services) {
	targets := a.treeTargets(p)
	single := len(targets) == 1
	dir := p
	if !isDir {
		dir = filepath.Dir(p)
	}
	m.Submenu(locales.ProjectMenuNew(), func(sub *ui.Menu) {
		if sub.Item(locales.MsgNewFile()).Chosen() {
			a.beginEntry(dir, false)
		}
		if sub.Item(locales.ProjectNewFolder()).Chosen() {
			a.beginEntry(dir, true)
		}
	})
	m.Separator()
	if m.Item(locales.ProjectCut()).Chosen() {
		a.cutTreeFiles(targets)
	}
	if m.Item(locales.Copy()).Chosen() {
		a.copyFiles(targets)
	}
	a.copyReferenceMenu(m, targets)
	clip := clipboardFiles(services)
	if m.Item(locales.MsgPaste()).Disabled(len(clip) == 0).Chosen() {
		a.pasteInto(dir, clip)
	}
	m.Separator()
	if isDir && m.Item(locales.ProjectFindInFolder()).Chosen() {
		a.find.mode, a.find.editingScope = findProject, false
		a.openFindPopup()
	}
	m.Submenu(locales.ProjectRefactor(), func(sub *ui.Menu) {
		if sub.Item(locales.ProjectRename()).Disabled(!single).Chosen() {
			a.beginRename(p)
		}
		if sub.Item(locales.ProjectMoveEllipsis()).Chosen() {
			a.promptMoveTo(targets)
		}
		if sub.Item(locales.ProjectCopyEllipsis()).Disabled(!single).Chosen() {
			a.promptCopyTo(p)
		}
	})
	if m.Item(locales.ProjectDelete()).Chosen() {
		a.requestTrashAll(targets)
	}
	m.Separator()
	if !isDir && single && m.Item(locales.ProjectOpen()).Chosen() {
		a.openFromTree(p)
	}
	a.openInMenu(m, p, isDir, nil)
	files := slices.DeleteFunc(slices.Clone(targets), a.isTreeDir)
	if len(files) == 2 && len(targets) == 2 {
		if m.Item(locales.ProjectCompareFiles()).Chosen() {
			a.compareFiles(files[0], files[1])
		}
	} else if !isDir && single {
		if active := a.tabFile(a.focus, a.focus.active); active != "" && active != p && m.Item(locales.ProjectCompareWithEditor()).Chosen() {
			a.compareFiles(active, p)
		}
	}
	if _, _, ok := a.repoRelativePath(p); ok && a.git.root != "" {
		changes := a.changesUnder(targets)
		_, status := a.treeGitStatus(p)
		m.Submenu("Git", func(sub *ui.Menu) {
			if sub.Item(locales.ProjectCommitFiles()).Disabled(len(changes) == 0).Chosen() {
				a.commitPaths(targets)
			}
			if status == '?' && sub.Item(locales.ProjectAddToGitignore()).Chosen() {
				a.addToGitignore(targets)
			}
			sub.Separator()
			if !isDir && single && len(changes) == 1 && sub.Item(locales.ProjectShowDiff()).Chosen() {
				a.showGitDiff(changes[0], false)
			}
			if !isDir && single && sub.Item(locales.GitFileHistory()).Chosen() {
				a.openFileHistoryOf(p)
			}
			sub.Separator()
			if sub.Item(locales.ProjectRollback()).Disabled(len(changes) == 0).Chosen() {
				a.discardGitChanges(changes)
			}
		})
	}
	m.Separator()
	if m.Item(locales.ProjectReload()).Chosen() {
		for _, t := range targets {
			if a.isTreeDir(t) {
				a.refreshDir(t)
			} else {
				a.refreshDir(filepath.Dir(t))
			}
		}
	}
}

// treeShortcuts are the project view's keys of IntelliJ's macOS keymap.
func (a *App) treeShortcuts(c *ui.Context) {
	h := a.tree.List.Handle
	services := c.Services()
	with := func(act func([]string)) func() {
		return func() {
			if chosen := a.treeChosen(); len(chosen) > 0 {
				act(chosen)
			}
		}
	}
	h.OnShortcut(c, ui.Cmd, ui.KeyC, with(a.copyFiles))
	h.OnShortcut(c, ui.Cmd, ui.KeyX, with(a.cutTreeFiles))
	h.OnShortcut(c, ui.Cmd, ui.KeyV, func() {
		a.pasteInto(a.entryTarget(), clipboardFiles(services))
	})
	h.OnShortcut(c, ui.Shift|ui.Cmd, ui.KeyC, with(func(paths []string) {
		services.WriteClipboard(strings.Join(paths, "\n"))
	}))
	h.OnShortcut(c, 0, ui.KeyF5, with(func(paths []string) {
		if len(paths) == 1 {
			a.promptCopyTo(paths[0])
		}
	}))
	h.OnShortcut(c, 0, ui.KeyF6, with(a.promptMoveTo))
	h.OnShortcut(c, 0, ui.KeyDelete, with(a.requestTrashAll))
	h.OnShortcut(c, ui.Cmd, ui.KeyBackspace, with(a.requestTrashAll))
	h.OnShortcut(c, ui.Cmd, ui.KeyN, func() { a.beginEntry(a.entryTarget(), false) })
}
