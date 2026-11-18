// INPUT: commands() 命令表。
// OUTPUT: 菜单栏的 View / Navigate / Code / Refactor / Git 菜单；Git 菜单按 IntelliJ 的顺序、文案与子菜单，进行中的操作项只在有操作时出现。
// POS: 菜单项只调用同名命令，不带快捷键（避免抢编辑器按键）。
// PROTOCOL: 变更时更新此头部，然后检查本包 doc.go

package app

import (
	"fmt"

	"github.com/egoist/mygo"

	"revtrail/internal/git"
	"revtrail/locales"
)

// The menu bar's View, Navigate, Code, Refactor and Git menus, after
// IntelliJ's. Their
// items run the commands of the same names. They show no accelerator:
// a menu's key equivalent would take the key from the view, which gives
// it a meaning of its own where it is pressed (Cmd+D in the editor, the
// terminal and the project tree), and the palette lists the keys.

// commandMenuItem is an item running the command named name, labelled
// label, or the command's label when label is "".
func (a *App) commandMenuItem(run func(func()), name, label string) *mygo.MenuItem {
	if label == "" {
		label = commandLabel(name)
	}
	return &mygo.MenuItem{ID: name, Label: label, Click: func(*mygo.MenuItem, *mygo.Window) {
		run(func() { a.runCommand(name) })
	}}
}

// runCommand runs the command named name.
func (a *App) runCommand(name string) {
	for _, cmd := range commands() {
		if cmd.name == name {
			cmd.run(a)
			return
		}
	}
}

func (a *App) viewMenu(run func(func())) *mygo.MenuItem {
	item := func(name, label string) *mygo.MenuItem { return a.commandMenuItem(run, name, label) }
	return &mygo.MenuItem{Label: locales.View(), Submenu: []*mygo.MenuItem{
		{Label: locales.MenuToolWindows(), Submenu: []*mygo.MenuItem{
			item("Show or Hide the Navigator", locales.MenuProject()),
			item("Show Git", locales.MenuCommit()),
			item("Show Problems", locales.MenuProblems()),
			item("Git: Show Log", locales.MenuGitLog()),
			item("Show or Hide the Terminal", locales.MenuTerminal()),
		}},
		mygo.Separator(),
		item("Find Action", locales.CommandPalette()),
		item("Recent Files", ""),
		mygo.Separator(),
		item("Open Markdown Preview", ""),
		item("Toggle Inline Diff", ""),
		mygo.Separator(),
		{Label: locales.Appearance(), Submenu: appearanceMenu(a)},
	}}
}

// codeMenu is IntelliJ's Code menu, as far as the language servers go.
func (a *App) codeMenu(run func(func())) *mygo.MenuItem {
	item := func(name string) *mygo.MenuItem { return a.commandMenuItem(run, name, "") }
	return &mygo.MenuItem{Label: locales.MenuCode(), Submenu: []*mygo.MenuItem{
		item("Show Context Actions"),
		mygo.Separator(),
		item("Reformat Code"),
		item("Optimize Imports"),
	}}
}

// refactorMenu is IntelliJ's Refactor menu, as far as the language
// servers go.
func (a *App) refactorMenu(run func(func())) *mygo.MenuItem {
	item := func(name, label string) *mygo.MenuItem { return a.commandMenuItem(run, name, label) }
	return &mygo.MenuItem{Label: locales.MenuRefactor(), Submenu: []*mygo.MenuItem{
		item("Refactor This…", ""),
		mygo.Separator(),
		item("Rename…", ""),
		mygo.Separator(),
		{Label: locales.MenuExtract(), Submenu: []*mygo.MenuItem{
			item("Extract Variable…", locales.MenuVariable()),
			item("Extract Method…", locales.MenuMethod()),
		}},
		item("Inline…", ""),
	}}
}

func (a *App) navigateMenu(run func(func())) *mygo.MenuItem {
	item := func(name string) *mygo.MenuItem { return a.commandMenuItem(run, name, "") }
	return &mygo.MenuItem{Label: locales.MenuNavigate(), Submenu: []*mygo.MenuItem{
		item("Go Back"),
		item("Go Forward"),
		mygo.Separator(),
		item("Search Everywhere"),
		item("Go to File"),
		item("Go to Symbol"),
		item("Go to Line"),
		mygo.Separator(),
		item("Go to Definition"),
		item("Find Usages"),
		item("Find Callers"),
		item("File Structure"),
		mygo.Separator(),
		item("Select in Project View"),
		mygo.Separator(),
		item("Next Problem"),
		item("Previous Problem"),
		item("Next Change"),
		item("Previous Change"),
	}}
}

// gitMenu is IntelliJ's Git menu, in its order and with its labels. The
// items of a merge, rebase, cherry-pick or revert in progress show while
// one is; the menu is built again when that changes.
func (a *App) gitMenu(run func(func())) *mygo.MenuItem {
	item := func(name, label string) *mygo.MenuItem { return a.commandMenuItem(run, name, label) }
	items := []*mygo.MenuItem{
		item("Show Git", locales.MenuGitCommit()),
		item("Git: Push", locales.MenuGitPush()),
		item("Update Project", locales.MenuGitUpdate()),
		item("Git: Pull", locales.MenuGitPull()),
		item("Git: Fetch", locales.MenuGitFetch()),
		mygo.Separator(),
		item("Git: Merge…", locales.MenuGitMerge()),
		item("Git: Rebase…", locales.MenuGitRebase()),
	}
	if op := a.git.op; op != git.OpNormal {
		items = append(items, item("Git: Resolve Conflicts", locales.MenuGitResolve()),
			item("Git: Continue Operation", fmt.Sprintf(locales.MenuGitContinue(), op.Label())))
		if op.CanSkip() {
			items = append(items, item("Git: Skip Operation", locales.MenuGitSkip()))
		}
		items = append(items, item("Git: Abort Operation", fmt.Sprintf(locales.MenuGitAbort(), op.Label())))
	}
	return &mygo.MenuItem{Label: "Git", Submenu: append(items,
		mygo.Separator(),
		item("Git: Switch Branch…", locales.MenuGitBranches()),
		item("Git: New Branch…", locales.BranchNew()),
		item("Git: New Tag…", locales.GitNewTag()),
		item("Git: Reset Branch to HEAD…", locales.MenuGitResetHead()),
		mygo.Separator(),
		item("Git: Show Log", locales.MenuGitShowLog()),
		&mygo.MenuItem{Label: locales.MenuGitPatch(), Submenu: []*mygo.MenuItem{
			item("Git: Copy Patch from Changes", locales.MenuGitCopyPatch()),
			item("Git: Apply Patch…", locales.MenuGitApplyPatch()),
			item("Git: Apply Patch from Clipboard", locales.MenuGitApplyClipboard()),
		}},
		&mygo.MenuItem{Label: locales.MenuGitUncommitted(), Submenu: []*mygo.MenuItem{
			item("Git: Stash Changes…", locales.GitStashChanges()),
			item("Git: Unstash Changes…", locales.MenuGitUnstash()),
			item("Git: Rollback…", locales.ProjectRollback()),
			item("Show Git", locales.MenuGitShowLocal()),
		}},
		&mygo.MenuItem{Label: locales.MenuGitCurrentFile(), Submenu: []*mygo.MenuItem{
			item("Git: Commit File…", locales.ProjectCommitFiles()),
			item("Git: Add File", locales.MenuGitAdd()),
			mygo.Separator(),
			item("Git: Annotate with Git Blame", locales.MenuGitBlame()),
			item("Git: Show Diff", locales.ProjectShowDiff()),
			item("Git: Compare with Branch…", locales.MenuGitCompareBranch()),
			item("Git: Show File History", locales.MenuGitShowHistory()),
			mygo.Separator(),
			item("Git: Rollback File…", locales.ProjectRollback()),
		}},
		mygo.Separator(),
		item("Git: Manage Remotes…", locales.MenuGitRemotes()),
		item("Git: Clone…", locales.MenuGitClone()),
	)}
}
