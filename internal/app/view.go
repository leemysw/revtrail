// INPUT: App 全部状态。
// OUTPUT: view：每帧的总绘制、全局快捷键分派与编辑器按键绑定。
// POS: 界面的根；新面板从这里挂入。
// PROTOCOL: 变更时更新此头部，然后检查本包 doc.go

package app

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"revtrail/internal/widget"
	"revtrail/locales"
)

// editorBindings take from the editor the keys that are the app's.
func editorBindings() []editor.Binding {
	none := func(m ui.Modifiers, k ui.Key) editor.Binding {
		return editor.Binding{Mods: m, Key: k, Command: editor.NoCommand}
	}
	bs := []editor.Binding{
		none(ui.Cmd|ui.Shift, ui.KeyM),
		none(ui.Alt|ui.Cmd, ui.KeyO),
		none(ui.Cmd|ui.Shift, ui.KeyB),
		none(0, ui.KeyF6),
		none(ui.Shift, ui.KeyF6),
		none(0, ui.KeyF8),
		none(ui.Shift, ui.KeyF8),
		none(ui.Ctrl, ui.KeyG),
		none(ui.Cmd, ui.KeyW),
		none(ui.Cmd, ui.KeyS),
		none(ui.Cmd|ui.Shift, ui.KeyS),
		none(ui.Cmd, ui.KeyO),
		none(ui.Cmd, ui.KeyE), // Recent Files, as IntelliJ
		none(ui.Cmd, ui.KeyN),
		none(ui.Cmd|ui.Shift, ui.KeyN),
		none(ui.Cmd, ui.KeyP),
		none(ui.Cmd, ui.KeyB),
		none(ui.Cmd|ui.Shift, ui.KeyP),
		none(ui.Cmd|ui.Shift, ui.KeyA),
		none(ui.Cmd, ui.KeyF12),
		none(ui.Cmd|ui.Shift, ui.KeyO),
		none(ui.Cmd|ui.Shift, ui.KeyF),
		none(ui.Cmd|ui.Shift, ui.KeyV),
		none(ui.Cmd|ui.Shift, ui.KeyC),
		none(ui.Cmd, ui.KeyBackslash),
		none(ui.Cmd|ui.Shift, ui.KeyD),
		none(ui.Ctrl, ui.KeyTab),
		none(ui.Ctrl|ui.Shift, ui.KeyTab),
		none(ui.Ctrl, ui.KeyMinus),
		none(ui.Ctrl|ui.Shift, ui.KeyMinus),
	}
	for k := ui.Key1; k <= ui.Key9; k++ {
		bs = append(bs, none(ui.Cmd, k))
	}
	// IntelliJ's keys replace these.
	idea := ideaEditorBindings()
	bs = slices.DeleteFunc(bs, func(b editor.Binding) bool {
		return slices.ContainsFunc(idea, func(i editor.Binding) bool { return i.Mods == b.Mods && i.Key == b.Key })
	})
	bs = append(bs, idea...)
	if runtime.GOOS != "darwin" {
		// Back and forward, as VS Code on Windows; the editor would move
		// by subwords.
		bs = append(bs, none(ui.Alt, ui.KeyLeft), none(ui.Alt, ui.KeyRight),
			none(ui.Ctrl, ui.KeyPageUp), none(ui.Ctrl, ui.KeyPageDown))
	}
	return bs
}

// startTime is when the process started, for the startup trace.
var startTime = time.Now()

var startTrace = os.Getenv("REVTRAIL_TRACE_STARTUP") != ""

func (a *App) view(c *ui.Context) {
	defer a.syncTerminalSelection()
	if startTrace {
		startTrace = false
		a.logf("startup: first frame built %v after process start", time.Since(startTime).Round(time.Millisecond))
	}
	a.syncIdleSaveSettings()
	a.applyAppearance(c)
	a.modifiers = c.Modifiers()
	c.SetTheme(a.theme)
	a.services = c.Services()
	vibrant := c.Vibrancy()
	if vibrant {
		defer c.Root().Background(ui.Transparent)
	}
	// Cmd held makes hovers links to definitions; letting it go ends one.
	a.hover.cmd = c.Modifiers()&ui.Cmd != 0
	if !a.hover.cmd {
		a.hideLink()
	}
	if a.root == "" && a.hasWelcome && len(a.docs) == 0 {
		// The primary main window hides without a folder; its welcome
		// window and folderless project windows show these actions.
		if c.Shortcut(ui.Cmd|ui.Shift, ui.KeyN) {
			a.host.OpenProjectWindow("")
		} else if c.Shortcut(ui.Cmd, ui.KeyO) {
			a.openFolderDialog()
		} else if c.Shortcut(ui.Cmd, ui.KeyW) {
			a.host.CloseWindow()
		}
		a.welcomeWindow(c)
		return
	}
	if a.git.push == nil && a.git.merge == nil && a.term.dialog == nil && a.actionConfirm == nil && a.confirm == nil && !a.trustAsk && !a.quick.open && !a.find.open && !a.git.picker.open && !a.projectPicker.open {
		a.shortcuts(c)
	}
	root := ui.Column(c).Fill()
	target := float32(1)
	if a.sidebarOff || a.root == "" {
		target = 0
	}
	progress := min(max(root.AnimateTo("sidebar", target, ui.Animation{Spring: ui.SpringOptions{Duration: 250 * time.Millisecond}}), 0), 1)
	root.Children(func() {
		a.toolbar(c)
		a.projectTabBar(c)
		ui.Row(c).Grow(1).FillWidth().AlignItems(ui.Stretch).Children(func() {
			if a.root != "" {
				a.toolWindowBar(c)
			}
			ui.Column(c).Grow(1).MinWidth(0).FillHeight().Children(func() {
				a.editorsAndPanel(c, func() {
					if progress == 0 {
						a.editorArea(c)
						return
					}
					shownWidth := a.sidebarW * progress
					if widget.SplitPanes(c, ui.Split, &shownWidth, func() { a.navigatorPane(c) }, func() { a.editorArea(c) }).Fill().Changed() && !a.sidebarOff && progress == 1 {
						a.sidebarW = shownWidth
						a.cfg.SidebarWidth = a.sidebarW
						a.saveConfigSoon()
					}
				})
			})
		})
		a.statusBar(c)
	})
	if a.git.push != nil {
		a.pushPanel(c, a.git.push)
	}
	if a.git.merge != nil {
		a.mergePanel(c, a.git.merge)
	}
	if a.term.dialog != nil {
		a.terminalDialog(c)
	}
	if a.quick.open || a.find.open {
		a.searchPopup(c)
	}
	if a.projectPicker.open {
		a.projectPickerPanel(c)
	} else if a.git.picker.open {
		a.branchPickerPanel(c)
	}
	switch {
	case a.confirm != nil:
		if a.confirm.inWindow {
			a.confirm.dialog(c.Key("unsaved-confirmation"))
		}
	case a.actionConfirm != nil:
		a.actionConfirm.dialog(c.Key("destructive-confirmation"))
	case a.git.prompt != nil:
		a.gitPromptModal(c)
	case a.refactorPopup != nil:
		a.refactorPopupView(c)
	case a.git.choice != nil:
		a.gitChoiceModal(c)
	case a.git.remoteDialog != nil:
		a.remotesModal(c)
	case a.git.clone != nil:
		a.cloneModal(c)
	case a.git.rebase != nil:
		a.rebaseModal(c)
	case a.git.stashes.open:
		a.stashesModal(c)
	case a.git.compare.open:
		a.compareBranchesModal(c)
	case a.git.fileHistory.open:
		a.fileHistoryModal(c)
	case a.trustAsk:
		q := confirmation{
			message: locales.TrustFolderQuestion(), detail: locales.TrustBanner(filepath.Base(a.root)),
			actions: []action{
				{locales.TrustFolder(), a.trust},
				{locales.NotNow(), func() {
					a.trustAsk, a.trustDismissed = false, true
					a.afterTrust = nil
				}},
			},
		}
		q.dialog(c.Key("trust-confirmation"))
	}
}

// shortcuts handles the app's keys wherever the focus is.
func (a *App) shortcuts(c *ui.Context) {
	// While a terminal has the keyboard, the shell gets the keys with
	// Control and Alt: the app asks for none of them, as asking takes
	// the key (Alt+Left moves by words there, Control+- undoes).
	inTerm := a.term.open && a.term.focused
	if inTerm {
		sessionMods := ui.Cmd
		if runtime.GOOS != "darwin" {
			sessionMods = ui.Ctrl | ui.Shift
		}
		switch {
		case c.Shortcut(sessionMods, ui.KeyT):
			a.newTerminalTab()
			return
		case c.Shortcut(sessionMods, ui.KeyW):
			if a.term.active != nil {
				a.closePane(a.term.active.active)
			}
			return
		case c.Shortcut(ui.Cmd|ui.Shift, ui.KeyD):
			a.splitTerminalPane(true)
			return
		case c.Shortcut(ui.Cmd, ui.KeyD):
			a.splitTerminal()
			return
		case c.Shortcut(ui.Cmd|ui.Shift, ui.KeyBracketRight):
			a.cycleTerminal(1)
			return
		case c.Shortcut(ui.Cmd|ui.Shift, ui.KeyBracketLeft):
			a.cycleTerminal(-1)
			return
		case c.Shortcut(ui.Shift, ui.KeyEscape):
			a.hideTerminal()
			return
		}
	}
	switch {
	case c.Shortcut(ui.Cmd|ui.Shift, ui.KeyY):
		a.toggleTerminal()
	case c.Shortcut(ui.Cmd|ui.Shift, ui.Key1):
		a.showWelcome = true
	case c.Shortcut(ui.Cmd|ui.Shift, ui.KeyP):
		a.openPalette()
	case c.Shortcut(ui.Cmd, ui.KeyT):
		if a.git.root != "" {
			a.reviewUpdate()
		} else {
			a.openQuickPanel("#")
		}
	case c.Shortcut(ui.Alt|ui.Cmd, ui.KeyN):
		if a.git.root != "" {
			a.openBranchPicker(a.git.picker.anchor)
			a.branchPage("new")
		}
	case c.Shortcut(ui.Cmd, ui.KeyP):
		a.openQuickly()
	case c.Shortcut(ui.Ctrl, ui.KeyBackquote):
		a.toggleTerminal()
	case c.Shortcut(ui.Cmd, ui.KeyBackslash):
		if inTerm {
			a.splitTerminal()
		} else {
			a.splitActive(sideRight)
		}
	case c.Shortcut(ui.Cmd|ui.Shift, ui.KeyBackslash):
		a.splitActive(sideBottom)
	}
	for _, cmd := range commands() {
		if cmd.shortcutKey == 0 || (inTerm && ((cmd.shortcutMods&ui.Ctrl != 0 && cmd.shortcutKey != ui.KeyBackquote) || cmd.shortcutKey == ui.KeyW)) {
			continue
		}
		hit := c.Shortcut(cmd.shortcutMods, cmd.shortcutKey)
		for _, k := range commandAliases[cmd.name] {
			hit = hit || !inTerm && c.Shortcut(k.m, k.k)
		}
		if hit {
			cmd.run(a)
		}
	}
	if inTerm {
		return
	}
	back := c.Shortcut(ui.Ctrl|ui.Cmd, ui.KeyLeft)
	fwd := c.Shortcut(ui.Ctrl|ui.Cmd, ui.KeyRight)
	next := false
	prev := false
	if runtime.GOOS == "darwin" {
		next = c.Shortcut(ui.Cmd|ui.Shift, ui.KeyBracketRight) || next
		prev = c.Shortcut(ui.Cmd|ui.Shift, ui.KeyBracketLeft) || prev
	} else {
		back = c.Shortcut(ui.Alt, ui.KeyLeft) || back
		fwd = c.Shortcut(ui.Alt, ui.KeyRight) || fwd
		next = c.Shortcut(ui.Ctrl, ui.KeyPageDown) || next
		prev = c.Shortcut(ui.Ctrl, ui.KeyPageUp) || prev
	}
	switch {
	case back:
		a.goBack()
	case fwd:
		a.goForward()
	case next:
		a.cycleTab(1)
	case prev:
		a.cycleTab(-1)
	}
	if c.Shortcut(ui.Cmd, ui.KeyF) {
		if st := a.activeEditor(); st != nil {
			st.Run(editor.Find)
		}
	}
	if c.Shortcut(0, ui.KeyEscape) {
		if a.navigator == navFiles && !a.sidebarOff && !a.quick.open && !a.filter.speed.composing() {
			if a.filter.speed.active() || a.filter.field.Focused(c) || a.filtering() {
				a.filter.speed = treeSearch{}
				a.filter.query = ""
				a.filter.toList = true
			}
		}
		a.choices = nil
		if !a.quick.open {
			a.git.peek = nil
		}
	}
}
