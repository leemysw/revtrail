// INPUT: commands() 命令表。
// OUTPUT: 菜单栏的 View / Navigate / Git 菜单。
// POS: 菜单项只调用同名命令，不带快捷键（避免抢编辑器按键）。
// PROTOCOL: 变更时更新此头部，然后检查本包 doc.go

package app

import (
	"github.com/egoist/mygo"

	"revtrail/locales"
)

// The menu bar's View, Navigate and Git menus, after IntelliJ's. Their
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
	return &mygo.MenuItem{Label: label, Click: func(*mygo.MenuItem, *mygo.Window) {
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

func (a *App) gitMenu(run func(func())) *mygo.MenuItem {
	item := func(name string) *mygo.MenuItem { return a.commandMenuItem(run, name, "") }
	return &mygo.MenuItem{Label: "Git", Submenu: []*mygo.MenuItem{
		item("Show Git"),
		item("Git: Push"),
		item("Update Project"),
		item("Git: Pull"),
		item("Git: Fetch"),
		mygo.Separator(),
		item("Git: Switch Branch…"),
		item("Git: Reset Branch to HEAD…"),
		item("Git: Stash Changes…"),
		item("Git: Unstash Changes…"),
		mygo.Separator(),
		item("Git: Show Log"),
		item("Git: Annotate with Git Blame"),
		item("Git: Show File History"),
		item("Git: Compare with Branch…"),
		mygo.Separator(),
		item("Git: Resolve Conflicts"),
		item("Git: Continue Operation"),
		item("Git: Skip Operation"),
		item("Git: Abort Operation"),
		mygo.Separator(),
		item("Git: Copy Patch from Changes"),
		item("Git: Apply Patch…"),
	}}
}
