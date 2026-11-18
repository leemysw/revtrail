// INPUT: 编辑器状态、命令表与 git 改动。
// OUTPUT: 编辑器右键菜单中 App 追加的部分（Show Context Actions、Find Usages、Refactor、Go To、Copy Path/Reference、Compare with Clipboard、Open In、Git）。
// POS: 按 IntelliJ 顺序排列；mygo 自带的编辑区块固定在其前。
// PROTOCOL: 变更时更新此头部，然后检查本包 doc.go

package app

import (
	"path/filepath"
	"slices"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"revtrail/internal/git"
	"revtrail/internal/workspace"
	"revtrail/locales"
)

// commandItem is a menu item for the command named name, showing its key
// and running it when chosen.
func (a *App) commandItem(m *ui.Menu, name, label string, disabled bool, run func()) {
	item := m.Item(label).Disabled(disabled)
	for _, cmd := range commands() {
		if cmd.name == name && cmd.shortcutKey != 0 {
			item = item.Shortcut(cmd.shortcutMods, cmd.shortcutKey)
			break
		}
	}
	if item.Chosen() {
		run()
	}
}

// editorMenu is what Revtrail adds to the editor's menu after the
// editing commands, in the order of IntelliJ's: Show Context Actions,
// Find Usages, Refactor, Go To,
// Copy Path/Reference, Compare with Clipboard, Open In and Git.
func (a *App) editorMenu(menu *ui.Menu, g *group, d *doc, st *editor.State) {
	at := func(fn func()) func() { return func() { a.focus = g; fn() } }
	a.commandItem(menu, "Show Context Actions", commandLabel("Show Context Actions"), st.ReadOnly, at(a.showContextActions))
	menu.Separator()
	a.commandItem(menu, "Find Usages", commandLabel("Find Usages"), false, func() {
		a.focus = g
		a.findUsagesAt(false, g.pointerAnchor, g.pointerInEditor)
	})
	if menu.Item(commandLabel("Find Callers")).Chosen() {
		a.focus = g
		a.findUsagesAt(true, g.pointerAnchor, g.pointerInEditor)
	}
	menu.Submenu(locales.MenuRefactor(), func(sub *ui.Menu) {
		a.commandItem(sub, "Refactor This…", commandLabel("Refactor This…"), st.ReadOnly, at(a.refactorThis))
		sub.Separator()
		a.commandItem(sub, "Rename…", commandLabel("Rename…"), st.ReadOnly, at(a.renameSymbol))
		sub.Separator()
		sub.Submenu(locales.MenuExtract(), func(ex *ui.Menu) {
			a.commandItem(ex, "Extract Variable…", locales.MenuVariable(), st.ReadOnly, at(a.extractVariable))
			a.commandItem(ex, "Extract Method…", locales.MenuMethod(), st.ReadOnly, at(a.extractMethod))
		})
		a.commandItem(sub, "Inline…", commandLabel("Inline…"), st.ReadOnly, at(a.inlineSymbol))
	})
	menu.Submenu(locales.EditorGoTo(), func(sub *ui.Menu) {
		a.commandItem(sub, "Go to Definition", locales.EditorDeclaration(), st.OnDefinition == nil, func() {
			a.focus = g
			st.OnDefinition(st.Buffer.Offset(st.CursorPos()))
		})
		a.commandItem(sub, "Go to Line", locales.EditorLine(), false, func() {
			a.focus = g
			a.openQuickPanel(":")
		})
		sub.Separator()
		a.commandItem(sub, "Next Problem", locales.EditorNextError(), false, func() { a.focus = g; a.goToProblem(1) })
		a.commandItem(sub, "Previous Problem", locales.EditorPreviousError(), false, func() { a.focus = g; a.goToProblem(-1) })
		sub.Separator()
		a.commandItem(sub, "Go Back", commandLabel("Go Back"), len(a.hist.Back) == 0, a.goBack)
		a.commandItem(sub, "Go Forward", commandLabel("Go Forward"), len(a.hist.Forward) == 0, a.goForward)
	})
	menu.Separator()
	if !d.untitled {
		menu.Submenu(locales.TabCopyPathReference(), func(sub *ui.Menu) {
			a.copyReferenceMenuItems(sub, []string{d.path})
			sub.Separator()
			if sub.Item(locales.EditorCodeReference()).Chosen() {
				ref := a.codeReference(d, st)
				a.services.WriteClipboard(ref)
				a.note(locales.MsgCopied()+ref+".", false)
			}
			if link := a.webLink(d, st); link != "" {
				host := hostName(a.git.web)
				if sub.Item(locales.EditorHostLink(host)).Chosen() {
					a.services.WriteClipboard(link)
					a.note(locales.MsgCopiedThe()+host+locales.MsgLink(), false)
				}
			}
		})
	}
	if menu.Item(locales.EditorCompareWithClipboard()).Chosen() {
		text := st.SelectedText()
		what := locales.EditorSelection()
		if text == "" {
			text, what = st.Buffer.String(), d.name
		}
		a.focus = g
		a.compareTexts(what+" ↔ "+locales.EditorClipboard(), "clipboard:"+d.path, text, a.services.ReadClipboard())
	}
	if d.untitled {
		return
	}
	menu.Separator()
	a.openInMenu(menu, d.path, false, func(sub *ui.Menu) {
		if link := a.webLink(d, st); link != "" && sub.Item(hostName(a.git.web)).Chosen() {
			a.services.OpenURL(link)
		}
	})
	if workspace.Within(a.root, d.path) {
		a.commandItem(menu, "Select in Project View", locales.TabSelectInProject(), false, func() {
			a.sidebarOff, a.navigator = false, navFiles
			a.revealInTree(d.path)
		})
	}
	if _, _, ok := a.repoRelativePath(d.path); ok && a.git.root != "" {
		changes := slices.DeleteFunc(a.changesUnder([]string{d.path}), func(ch git.Change) bool { return changeID(ch) != d.path })
		menu.Submenu("Git", func(sub *ui.Menu) {
			if sub.Item(commandLabel("Git: Annotate with Git Blame")).Checked(d.blameOpen).Chosen() {
				a.focus = g
				a.activateIn(g, d.path)
				a.toggleGitBlame()
			}
			if sub.Item(locales.GitFileHistory()).Chosen() {
				a.openFileHistoryOf(d.path)
			}
			if sub.Item(locales.ProjectShowDiff()).Disabled(len(changes) == 0).Chosen() {
				a.showGitDiff(changes[0], false)
			}
			if sub.Item(commandLabel("Git: Compare with Branch…")).Chosen() {
				a.promptCompareBranches()
			}
			sub.Separator()
			if sub.Item(locales.EditorRollbackLines()).Disabled(len(changes) == 0 || st.ReadOnly).Chosen() {
				st.Run(editor.RevertHunk)
			}
			if sub.Item(locales.ProjectRollback()).Disabled(len(changes) == 0).Chosen() {
				a.discardGitChanges(changes)
			}
		})
	}
}

// copyReferenceMenuItems are the path items of Copy Path/Reference.
func (a *App) copyReferenceMenuItems(sub *ui.Menu, paths []string) {
	join := func(f func(string) string) string {
		out := ""
		for i, p := range paths {
			if i > 0 {
				out += "\n"
			}
			out += f(p)
		}
		return out
	}
	if sub.Item(locales.TabAbsolutePath()).Chosen() {
		a.services.WriteClipboard(join(func(p string) string { return p }))
	}
	if sub.Item(locales.TabPathFromRoot()).Chosen() {
		a.services.WriteClipboard(join(func(p string) string { return filepath.ToSlash(workspace.RelPath(a.root, p)) }))
	}
	if sub.Item(locales.TabFileName()).Chosen() {
		a.services.WriteClipboard(join(filepath.Base))
	}
}
