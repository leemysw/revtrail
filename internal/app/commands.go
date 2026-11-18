// INPUT: App 的各项操作与 keymap.go 的按键表。
// OUTPUT: commands() 命令表、commandNames、命令面板入口与 shell 命令安装。
// POS: Find Action、菜单栏、快捷键共用的命令真相源。
// PROTOCOL: 变更时更新此头部，然后检查本包 doc.go

package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"revtrail/internal/tasks"
	"revtrail/locales"
)

type command struct {
	name, mods, key string
	shortcutMods    ui.Modifiers
	shortcutKey     ui.Key
	run             func(*App)
}

func commands() []command {
	cmds := []command{
		{"New File", "Cmd+", "N", ui.Cmd, ui.KeyN, func(a *App) { a.newFile() }},
		{"Find Action", "Cmd+Shift+", "A", ui.Cmd | ui.Shift, ui.KeyA, func(a *App) { a.openPalette() }},
		{"New Window", "Cmd+Shift+", "N", ui.Cmd | ui.Shift, ui.KeyN, func(a *App) {
			a.host.OpenProjectWindow("")
		}},
		{"Open Folder", "Cmd+", "O", ui.Cmd, ui.KeyO, func(a *App) { a.openFolderDialog() }},
		{"Search Everywhere", "", "", 0, 0, func(a *App) { a.searchEverywhere() }},
		{"Go to File", "Cmd+Shift+", "O", ui.Cmd | ui.Shift, ui.KeyO, func(a *App) { a.openQuickly() }},
		{"Recent Files", "Cmd+", "E", ui.Cmd, ui.KeyE, func(a *App) { a.openRecentFiles() }},
		{"Show Problems", "Cmd+Shift+", "M", ui.Cmd | ui.Shift, ui.KeyM, func(a *App) { a.sidebarOff, a.navigator = false, navProblems }},
		{"Next Problem", "", "F8", 0, ui.KeyF8, func(a *App) { a.goToProblem(1) }},
		{"Previous Problem", "Shift+", "F8", ui.Shift, ui.KeyF8, func(a *App) { a.goToProblem(-1) }},
		{"Go to Line", "Ctrl+", "G", ui.Ctrl, ui.KeyG, func(a *App) {
			if a.activeEditor() != nil {
				a.openQuickPanel(":")
			}
		}},
		{"File Structure", "Cmd+", "F12", ui.Cmd, ui.KeyF12, func(a *App) { a.openQuickPanel("@") }},
		{"Go to Symbol", "Alt+Cmd+", "O", ui.Alt | ui.Cmd, ui.KeyO, func(a *App) { a.openQuickPanel("#") }},
		{"Open Markdown Preview", "Cmd+Shift+", "V", ui.Cmd | ui.Shift, ui.KeyV, func(a *App) { a.toggleMarkdownPreview(a.focus) }},
		{"Find in Files", "Cmd+Shift+", "F", ui.Cmd | ui.Shift, ui.KeyF, func(a *App) {
			a.openFindPopup()
		}},
		{"Show Git", "Cmd+", "K", ui.Cmd, ui.KeyK, func(a *App) { a.showCommitNavigator() }},
		{"Git: Resolve Conflicts", "", "", 0, 0, func(a *App) { a.showMergeConflicts() }},
		{"Git: Show Log", "", "", 0, 0, func(a *App) { a.showGitLogPanel() }},
		{"Toggle Inline Diff", "", "", 0, 0, func(a *App) { a.toggleDiffLayout() }},
		{"Next Change", "", "", 0, 0, func(a *App) { a.goToChange(1) }},
		{"Previous Change", "", "", 0, 0, func(a *App) { a.goToChange(-1) }},
		{"Git: Fetch", "", "", 0, 0, func(a *App) { a.gitFetch() }},
		{"Git: Pull", "", "", 0, 0, func(a *App) { a.gitPull() }},
		{"Git: Push", "Shift+Cmd+", "K", ui.Shift | ui.Cmd, ui.KeyK, func(a *App) { a.reviewPush() }},
		{"Git: Switch Branch…", "", "", 0, 0, func(a *App) { a.openBranchPicker(a.git.picker.anchor) }},
		{"Git: Continue Operation", "", "", 0, 0, func(a *App) { a.continueGitOp() }},
		{"Git: Skip Operation", "", "", 0, 0, func(a *App) { a.skipGitOp() }},
		{"Git: Abort Operation", "", "", 0, 0, func(a *App) { a.abortGitOp() }},
		{"Git: Reset Branch to HEAD…", "", "", 0, 0, func(a *App) { a.promptResetBranch("HEAD", "HEAD") }},
		{"Git: Stash Changes…", "", "", 0, 0, func(a *App) { a.promptStashChanges() }},
		{"Git: Unstash Changes…", "", "", 0, 0, func(a *App) { a.openStashesModal() }},
		{"Git: Annotate with Git Blame", "", "", 0, 0, func(a *App) { a.toggleGitBlame() }},
		{"Git: Show File History", "", "", 0, 0, func(a *App) { a.openFileHistory() }},
		{"Git: Compare with Branch…", "", "", 0, 0, func(a *App) { a.promptCompareBranches() }},
		{"Git: Copy Patch from Changes", "", "", 0, 0, func(a *App) { a.copyWorkingPatch() }},
		{"Git: Apply Patch…", "", "", 0, 0, func(a *App) { a.promptApplyPatch() }},
		{"Filter Files", "Alt+Cmd+", "J", ui.Alt | ui.Cmd, ui.KeyJ, func(a *App) {
			a.filter.speed = treeSearch{}
			a.sidebarOff, a.navigator, a.filter.focus = false, navFiles, true
		}},
		{"Show or Hide the Navigator", "Cmd+", "B", ui.Cmd, ui.KeyB, func(a *App) {
			// As IntelliJ's Project tool window key: it shows the project
			// view, and hides it when it is showing.
			if a.sidebarOff || a.navigator != navFiles {
				a.sidebarOff, a.navigator = false, navFiles
			} else {
				a.sidebarOff = true
			}
		}},
		{"Go Back", "Ctrl+", "-", ui.Ctrl, ui.KeyMinus, func(a *App) { a.goBack() }},
		{"Go Forward", "Ctrl+Shift+", "-", ui.Ctrl | ui.Shift, ui.KeyMinus, func(a *App) { a.goForward() }},
		{"Focus Next Editor", "", "F6", 0, ui.KeyF6, func(a *App) { a.cycleEditorGroup(1) }},
		{"Focus Previous Editor", "Shift+", "F6", ui.Shift, ui.KeyF6, func(a *App) { a.cycleEditorGroup(-1) }},
		{"Move Tab to Next Editor", "", "", 0, 0, func(a *App) { a.moveTabToNextEditor() }},
		{"Next Tab", "Ctrl+", "Tab", ui.Ctrl, ui.KeyTab, func(a *App) { a.cycleTab(1) }},
		{"Previous Tab", "Ctrl+Shift+", "Tab", ui.Ctrl | ui.Shift, ui.KeyTab, func(a *App) { a.cycleTab(-1) }},
		{"Save", "", "", 0, 0, func(a *App) { a.saveActive() }},
		{"Save All", "Cmd+", "S", ui.Cmd, ui.KeyS, func(a *App) { a.saveAll() }},
		{"Close Tab", "Cmd+", "W", ui.Cmd, ui.KeyW, func(a *App) {
			// As Xcode: the tab, and the window once no tab is left.
			if p := a.focus.active; p != "" {
				a.closeDoc(p)
			} else {
				a.host.CloseWindow()
			}
		}},
		{"Close Other Tabs", "", "", 0, 0, func(a *App) { a.closeTabs(a.focus, a.focus.active, closeOtherTabs) }},
		{"Close Tabs to the Right", "", "", 0, 0, func(a *App) { a.closeTabs(a.focus, a.focus.active, closeRightTabs) }},
		{"Close Tabs to the Left", "", "", 0, 0, func(a *App) { a.closeTabs(a.focus, a.focus.active, closeLeftTabs) }},
		{"Close Unmodified Tabs", "", "", 0, 0, func(a *App) { a.closeTabs(a.focus, a.focus.active, closeSavedTabs) }},
		{"Close All Tabs", "", "", 0, 0, func(a *App) { a.closeTabs(a.focus, a.focus.active, closeAllTabs) }},
		{"Close All but Pinned", "", "", 0, 0, func(a *App) { a.closeTabs(a.focus, a.focus.active, closeUnpinnedTabs) }},
		{"Pin Tab", "", "", 0, 0, func(a *App) { a.togglePin(a.focus, a.focus.active) }},
		{"Add Editor on Right", "Cmd+", "D", ui.Cmd, ui.KeyD, func(a *App) { a.splitActive(sideRight) }},
		{"Add Editor Below", "Cmd+Shift+", "D", ui.Cmd | ui.Shift, ui.KeyD, func(a *App) { a.splitActive(sideBottom) }},
		{"Show or Hide the Terminal", "Alt+", "F12", ui.Alt, ui.KeyF12, func(a *App) {
			if a.term.open && a.term.focused {
				a.hideTerminal()
			} else {
				a.showTerminal()
			}
		}},
		{"Go: Build Workspace", "Cmd+Shift+", "B", ui.Cmd | ui.Shift, ui.KeyB, func(a *App) { a.runGoTask(tasks.Build) }},
		{"Go: Test Workspace", "", "", 0, 0, func(a *App) { a.runGoTask(tasks.Test) }},
		{"Tasks: Show", "", "", 0, 0, func(a *App) { a.showTasks() }},
		{"Tasks: Stop", "", "", 0, 0, func(a *App) { a.stopGoTask() }},
		{"Show Output", "", "", 0, 0, func(a *App) { a.term.open = true; a.term.view = panelOutput; a.term.focus = nil }},
		{"New Terminal", "Ctrl+Shift+", "`", ui.Ctrl | ui.Shift, ui.KeyBackquote, func(a *App) { a.newTerminalTab() }},
		{"Open in External Terminal", "Cmd+Shift+", "C", ui.Cmd | ui.Shift, ui.KeyC, func(a *App) { a.openExternalTerminal() }},
		{"Settings", "Cmd+", ",", ui.Cmd, ui.KeyComma, func(a *App) {
			a.host.OpenSettings()
		}},
		{"Keep Preview Tab", "", "", 0, 0, func(a *App) {
			if a.focus != nil {
				a.focus.preview = ""
			}
		}},
		{"Shell Command: Install 'revtrail' Command in PATH", "", "", 0, 0, func(a *App) { a.installRevtrail() }},
		{"Shell Command: Uninstall 'revtrail' Command from PATH", "", "", 0, 0, func(a *App) { a.uninstallRevtrail() }},
	}
	cmds = append(cmds,
		command{"Find in File", "Cmd+", "F", 0, 0, func(a *App) {
			if st := a.activeEditor(); st != nil {
				st.Run(editor.Find)
			}
		}},
		command{"Find Usages", "Shift+", "F12", ui.Shift, ui.KeyF12, func(a *App) { a.findUsages(false) }},
		command{"Find Callers", "", "", 0, 0, func(a *App) { a.findUsages(true) }},
		command{"Go to Definition", "", "F12", 0, 0, func(a *App) {
			if st := a.activeEditor(); st != nil && st.OnDefinition != nil {
				st.OnDefinition(st.Buffer.Offset(st.CursorPos()))
			}
		}},
	)
	for n := 1; n <= 9; n++ {
		label := fmt.Sprintf("Select Tab %d", n)
		if n == 9 {
			label = "Select Last Tab"
		}
		cmds = append(cmds, command{label, "Cmd+", fmt.Sprint(n), ui.Cmd, ui.Key1 + ui.Key(n-1), func(a *App) { a.activateTabNumber(n) }})
	}
	cmds = append(cmds,
		command{"Copy Path", "", "", 0, 0, func(a *App) { a.copyActivePath() }},
		command{"Select in Project View", "", "", 0, 0, func(a *App) { a.selectActiveInProject() }},
		command{"Update Project", "", "", 0, 0, func(a *App) { a.reviewUpdate() }},
	)
	return applyKeymap(cmds)

}

// commandNames are the commands' labels, as the palette lists them.
func commandNames() []string {
	all := commands()
	names := make([]string, len(all))
	for i, cmd := range all {
		names[i] = commandLabel(cmd.name)
	}
	return names
}

func (a *App) openPalette() {
	a.openQuickPanel(">")
}

func (a *App) installRevtrail() {
	exe, err := os.Executable()
	if err != nil {
		a.note(locales.MsgCouldNotLocateTheApplication()+err.Error(), true)
		return
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		a.note(locales.MsgCouldNotLocateTheApplication()+err.Error(), true)
		return
	}
	if strings.Contains(filepath.ToSlash(exe), "/go-build") || strings.Contains(filepath.ToSlash(exe), "/.mygo/dev/") {
		a.note(locales.MsgBuildAPermanentExecutableBeforeInstallingTheRevtrail(), true)
		return
	}
	home, err := os.UserHomeDir()
	if err != nil {
		a.note(locales.MsgCouldNotLocateYourHomeDirectory()+err.Error(), true)
		return
	}
	dir := filepath.Join(home, ".local", "bin")
	path, already, err := installRevtrailLink(exe, dir)
	if err != nil {
		a.note(fmt.Sprintf(locales.MsgCouldNotInstallSV(), path, err), true)
		return
	}
	msg := "Installed revtrail at " + path
	if already {
		msg = "The revtrail command is already installed at " + path
	}
	found := false
	for _, p := range filepath.SplitList(os.Getenv("PATH")) {
		if filepath.Clean(p) == dir {
			found = true
		}
	}
	if !found {
		msg += ". Add " + dir + " to your PATH."
	}
	a.note(msg, false)
}

func (a *App) uninstallRevtrail() {
	exe, err := os.Executable()
	if err == nil {
		exe, err = filepath.EvalSymlinks(exe)
	}
	if err != nil {
		a.note(locales.MsgCouldNotLocateTheApplication()+err.Error(), true)
		return
	}
	home, err := os.UserHomeDir()
	if err != nil {
		a.note(locales.MsgCouldNotLocateYourHomeDirectory()+err.Error(), true)
		return
	}
	path, removed, err := uninstallRevtrailLink(exe, filepath.Join(home, ".local", "bin"))
	if err != nil {
		a.note(fmt.Sprintf(locales.MsgCouldNotUninstallSV(), path, err), true)
	} else if !removed {
		a.note(locales.MsgTheRevtrailCommandIsNotInstalledAt()+path, false)
	} else {
		a.note(locales.MsgUninstalledRevtrailFrom()+path, false)
	}
}

func uninstallRevtrailLink(exe, dir string) (path string, removed bool, err error) {
	path = filepath.Join(dir, "revtrail")
	target, err := os.Readlink(path)
	if os.IsNotExist(err) {
		return path, false, nil
	}
	if err != nil {
		return path, false, err
	}
	if target != exe {
		return path, false, fmt.Errorf("link points to %s, not the running application", target)
	}
	if err := os.Remove(path); err != nil {
		return path, false, err
	}
	return path, true, nil
}

func installRevtrailLink(exe, dir string) (path string, already bool, err error) {
	path = filepath.Join(dir, "revtrail")
	if err = os.MkdirAll(dir, 0755); err == nil {
		err = os.Symlink(exe, path)
	}
	if err != nil {
		if target, e := os.Readlink(path); e == nil && target == exe {
			return path, true, nil
		}
		return path, false, err
	}
	return path, false, nil
}
