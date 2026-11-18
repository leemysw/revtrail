// INPUT: commands() 命令表与 mygo 编辑器命令。
// OUTPUT: IntelliJ 按键表（mac / pc）、命令别名、编辑器绑定与 Copy Path、Select in Project View 等小操作。
// POS: 所有快捷键的唯一来源；改键只改这里。
// PROTOCOL: 变更时更新此头部，然后检查本包 doc.go

package app

import (
	"runtime"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

// The keymap is IntelliJ's: its macOS keymap on macOS, its default one
// elsewhere. The editor's own keys come first (ideaEditorBindings), then
// the app's commands (ideaCommandKeys), each with the keys it had before
// that IntelliJ leaves free as aliases.

// keyCombo is a key with its modifiers, and how a menu writes them.
type keyCombo struct {
	mods, key string
	m         ui.Modifiers
	k         ui.Key
}

func combo(mods, key string, m ui.Modifiers, k ui.Key) keyCombo {
	return keyCombo{mods, key, m, k}
}

// commandKeys are a command's keys: the one menus show, and aliases.
type commandKeys struct {
	primary keyCombo
	aliases []keyCombo
}

var isMac = runtime.GOOS == "darwin"

// ideaCommandKeys maps commands to IntelliJ's keys for them. A primary
// key with no ui.Key is shown but handled elsewhere.
func ideaCommandKeys() map[string]commandKeys {
	if isMac {
		return map[string]commandKeys{
			"Go to Line":                 {primary: combo("Cmd+", "L", ui.Cmd, ui.KeyL)},
			"Go to Definition":           {primary: combo("Cmd+", "B", ui.Cmd, ui.KeyB)},
			"Find Usages":                {primary: combo("Alt+", "F7", ui.Alt, ui.KeyF7), aliases: []keyCombo{combo("Shift+", "F12", ui.Shift, ui.KeyF12)}},
			"Go Back":                    {primary: combo("Cmd+", "[", ui.Cmd, ui.KeyBracketLeft), aliases: []keyCombo{combo("Alt+Cmd+", "←", ui.Alt|ui.Cmd, ui.KeyLeft), combo("Ctrl+", "-", ui.Ctrl, ui.KeyMinus)}},
			"Go Forward":                 {primary: combo("Cmd+", "]", ui.Cmd, ui.KeyBracketRight), aliases: []keyCombo{combo("Alt+Cmd+", "→", ui.Alt|ui.Cmd, ui.KeyRight), combo("Ctrl+Shift+", "-", ui.Ctrl|ui.Shift, ui.KeyMinus)}},
			"Next Problem":               {primary: combo("", "F2", 0, ui.KeyF2), aliases: []keyCombo{combo("", "F8", 0, ui.KeyF8)}},
			"Previous Problem":           {primary: combo("Shift+", "F2", ui.Shift, ui.KeyF2), aliases: []keyCombo{combo("Shift+", "F8", ui.Shift, ui.KeyF8)}},
			"Show or Hide the Navigator": {primary: combo("Cmd+", "1", ui.Cmd, ui.Key1)},
			"Show Problems":              {primary: combo("Cmd+", "6", ui.Cmd, ui.Key6), aliases: []keyCombo{combo("Cmd+Shift+", "M", ui.Cmd|ui.Shift, ui.KeyM)}},
			"Git: Show Log":              {primary: combo("Cmd+", "9", ui.Cmd, ui.Key9)},
			"Show Git":                   {primary: combo("Cmd+", "0", ui.Cmd, ui.Key0), aliases: []keyCombo{combo("Cmd+", "K", ui.Cmd, ui.KeyK)}},
			"Add Editor on Right":        {primary: combo("Cmd+", "\\", 0, 0)},
			"Add Editor Below":           {primary: combo("Cmd+Shift+", "\\", 0, 0)},
			"Open in External Terminal":  {},
			"Copy Path":                  {primary: combo("Cmd+Shift+", "C", ui.Cmd|ui.Shift, ui.KeyC)},
			"Select in Project View":     {primary: combo("Alt+", "F1", ui.Alt, ui.KeyF1)},
			"Update Project":             {primary: combo("Cmd+", "T", 0, 0)},
		}
	}
	return map[string]commandKeys{
		"Go to Definition":           {primary: combo("Cmd+", "B", ui.Cmd, ui.KeyB)},
		"Find Usages":                {primary: combo("Alt+", "F7", ui.Alt, ui.KeyF7), aliases: []keyCombo{combo("Shift+", "F12", ui.Shift, ui.KeyF12)}},
		"Go Back":                    {primary: combo("Ctrl+Alt+", "←", ui.Ctrl|ui.Alt, ui.KeyLeft), aliases: []keyCombo{combo("Ctrl+", "-", ui.Ctrl, ui.KeyMinus)}},
		"Go Forward":                 {primary: combo("Ctrl+Alt+", "→", ui.Ctrl|ui.Alt, ui.KeyRight), aliases: []keyCombo{combo("Ctrl+Shift+", "-", ui.Ctrl|ui.Shift, ui.KeyMinus)}},
		"Next Problem":               {primary: combo("", "F2", 0, ui.KeyF2), aliases: []keyCombo{combo("", "F8", 0, ui.KeyF8)}},
		"Previous Problem":           {primary: combo("Shift+", "F2", ui.Shift, ui.KeyF2), aliases: []keyCombo{combo("Shift+", "F8", ui.Shift, ui.KeyF8)}},
		"Show or Hide the Navigator": {primary: combo("Alt+", "1", ui.Alt, ui.Key1)},
		"Show Problems":              {primary: combo("Alt+", "6", ui.Alt, ui.Key6), aliases: []keyCombo{combo("Cmd+Shift+", "M", ui.Cmd|ui.Shift, ui.KeyM)}},
		"Git: Show Log":              {primary: combo("Alt+", "9", ui.Alt, ui.Key9)},
		"Show Git":                   {primary: combo("Alt+", "0", ui.Alt, ui.Key0), aliases: []keyCombo{combo("Cmd+", "K", ui.Cmd, ui.KeyK)}},
		"Close Tab":                  {primary: combo("Cmd+", "F4", ui.Cmd, ui.KeyF4), aliases: []keyCombo{combo("Cmd+", "W", ui.Cmd, ui.KeyW)}},
		"Add Editor on Right":        {primary: combo("Cmd+", "\\", 0, 0)},
		"Add Editor Below":           {primary: combo("Cmd+Shift+", "\\", 0, 0)},
		"Open in External Terminal":  {},
		"Copy Path":                  {primary: combo("Cmd+Shift+", "C", ui.Cmd|ui.Shift, ui.KeyC)},
		"Select in Project View":     {primary: combo("Alt+", "F1", ui.Alt, ui.KeyF1)},
		"Update Project":             {primary: combo("Cmd+", "T", 0, 0)},
	}
}

// commandAliases are the commands' keys besides the one menus show.
var commandAliases = func() map[string][]keyCombo {
	out := map[string][]keyCombo{}
	for name, ks := range ideaCommandKeys() {
		if len(ks.aliases) > 0 {
			out[name] = ks.aliases
		}
	}
	return out
}()

// applyKeymap gives the commands IntelliJ's keys, and drops the keys of
// those IntelliJ has none for (choosing a tab by its number).
func applyKeymap(cmds []command) []command {
	keys := ideaCommandKeys()
	for i := range cmds {
		cmd := &cmds[i]
		if ks, ok := keys[cmd.name]; ok {
			cmd.mods, cmd.key = ks.primary.mods, ks.primary.key
			cmd.shortcutMods, cmd.shortcutKey = ks.primary.m, ks.primary.k
		} else if cmd.shortcutMods == ui.Cmd && cmd.shortcutKey >= ui.Key1 && cmd.shortcutKey <= ui.Key9 {
			cmd.mods, cmd.key, cmd.shortcutMods, cmd.shortcutKey = "", "", 0, 0
		}
	}
	return cmds
}

// ideaEditorBindings are IntelliJ's editing keys, which replace the
// editor's defaults (Zed's and VS Code's) for those keys.
func ideaEditorBindings() []editor.Binding {
	b := func(m ui.Modifiers, k ui.Key, cmd editor.Command) editor.Binding {
		return editor.Binding{Mods: m, Key: k, Command: cmd}
	}
	cmd, alt, ctrl, sh := ui.Cmd, ui.Alt, ui.Ctrl, ui.Shift
	if isMac {
		return []editor.Binding{
			b(cmd, ui.KeyD, editor.DuplicateSelection),
			b(cmd, ui.KeyBackspace, editor.DeleteLine),
			b(alt, ui.KeyUp, editor.SelectLarger),
			b(alt, ui.KeyDown, editor.SelectSmaller),
			b(alt|sh, ui.KeyUp, editor.MoveLinesUp),
			b(alt|sh, ui.KeyDown, editor.MoveLinesDown),
			b(cmd|sh, ui.KeyUp, editor.MoveLinesUp), // Move Statement Up
			b(cmd|sh, ui.KeyDown, editor.MoveLinesDown),
			b(ctrl, ui.KeyG, editor.SelectNext), // Add Selection for Next Occurrence
			b(ctrl|sh, ui.KeyG, editor.UndoSelection),
			b(ctrl|cmd, ui.KeyG, editor.SelectAllMatches),
			b(cmd, ui.KeyR, editor.FindReplace),
			b(alt|cmd, ui.KeySlash, editor.ToggleBlockComment),
			b(0, ui.KeyF1, editor.ShowHover), // Quick Documentation
			b(sh, ui.KeyEnter, editor.NewlineBelow),
			b(alt|cmd, ui.KeyEnter, editor.NewlineAbove),
			b(ctrl|sh, ui.KeyJ, editor.JoinLines),
			b(cmd, ui.KeyMinus, editor.Fold),
			b(cmd, ui.KeyEqual, editor.Unfold),
			b(cmd|sh, ui.KeyMinus, editor.FoldAll),
			b(cmd|sh, ui.KeyEqual, editor.UnfoldAll),
			// The app's: Go to Line, Back and Forward, Project, Problems,
			// Commit, Find Usages, Select In, Copy Path.
			b(cmd, ui.KeyL, editor.NoCommand),
			b(cmd, ui.KeyBracketLeft, editor.NoCommand),
			b(cmd, ui.KeyBracketRight, editor.NoCommand),
			b(alt|cmd, ui.KeyLeft, editor.NoCommand),
			b(alt|cmd, ui.KeyRight, editor.NoCommand),
			b(cmd, ui.Key0, editor.NoCommand),
			b(cmd, ui.KeyK, editor.NoCommand),
			b(0, ui.KeyF2, editor.NoCommand),
			b(sh, ui.KeyF2, editor.NoCommand),
			b(alt, ui.KeyF7, editor.NoCommand),
			b(alt, ui.KeyF1, editor.NoCommand),
		}
	}
	return []editor.Binding{
		b(ctrl, ui.KeyD, editor.DuplicateSelection),
		b(ctrl, ui.KeyY, editor.DeleteLine),
		b(ctrl, ui.KeyW, editor.SelectLarger),
		b(ctrl|sh, ui.KeyW, editor.SelectSmaller),
		b(alt|sh, ui.KeyUp, editor.MoveLinesUp),
		b(alt|sh, ui.KeyDown, editor.MoveLinesDown),
		b(ctrl|sh, ui.KeyUp, editor.MoveLinesUp),
		b(ctrl|sh, ui.KeyDown, editor.MoveLinesDown),
		b(alt, ui.KeyJ, editor.SelectNext),
		b(alt|sh, ui.KeyJ, editor.UndoSelection),
		b(ctrl|alt|sh, ui.KeyJ, editor.SelectAllMatches),
		b(ctrl, ui.KeyR, editor.FindReplace),
		b(ctrl|sh, ui.KeySlash, editor.ToggleBlockComment),
		b(ctrl, ui.KeyQ, editor.ShowHover),
		b(sh, ui.KeyEnter, editor.NewlineBelow),
		b(ctrl|alt, ui.KeyEnter, editor.NewlineAbove),
		b(ctrl|sh, ui.KeyJ, editor.JoinLines),
		b(ctrl|alt, ui.KeyLeft, editor.NoCommand),
		b(ctrl|alt, ui.KeyRight, editor.NoCommand),
		b(ctrl, ui.KeyF4, editor.NoCommand),
		b(0, ui.KeyF2, editor.NoCommand),
		b(sh, ui.KeyF2, editor.NoCommand),
		b(alt, ui.KeyF7, editor.NoCommand),
		b(alt, ui.KeyF1, editor.NoCommand),
		b(alt, ui.Key1, editor.NoCommand),
		b(alt, ui.Key6, editor.NoCommand),
		b(alt, ui.Key9, editor.NoCommand),
		b(alt, ui.Key0, editor.NoCommand),
	}
}

// copyActivePath copies the path of the file shown, as IntelliJ's Copy
// Path (Shift+Cmd+C).
func (a *App) copyActivePath() {
	if a.focus == nil {
		return
	}
	if p := a.tabFile(a.focus, a.focus.active); p != "" {
		a.services.WriteClipboard(p)
	}
}

// selectActiveInProject shows the file shown in the project tree, as
// IntelliJ's Select In › Project View (Alt+F1).
func (a *App) selectActiveInProject() {
	if a.focus == nil {
		return
	}
	if p := a.tabFile(a.focus, a.focus.active); p != "" && a.root != "" {
		a.selectInProject(p)
	}
}

// selectInProject shows the project view with the row of p chosen.
func (a *App) selectInProject(p string) {
	a.sidebarOff, a.navigator = false, navFiles
	if p == a.root {
		a.reveal = p
		return
	}
	a.revealInTree(p)
}
