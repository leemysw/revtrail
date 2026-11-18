// INPUT: config 终端配置与 SSH 会话。
// OUTPUT: 终端标签头：配置切换、SSH 连接对话框。
// POS: 终端面板顶栏。
// PROTOCOL: 变更时更新此头部，然后检查本包 doc.go

package app

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"revtrail/internal/config"
	"revtrail/internal/glyph"
	"revtrail/internal/theme"
	"revtrail/internal/widget"
	"revtrail/locales"
)

type termProfile struct {
	name string
	cmd  []string
}

type termDialog struct {
	tab        *termTab // nil for a new SSH connection
	text, port string
}

func terminalProfiles(configured string) (shells, agents []termProfile) {
	names := []string{"bash", "zsh", "fish", "sh"}
	if runtime.GOOS == "windows" {
		names = []string{"pwsh", "powershell", "cmd"}
	}
	seen := map[string]bool{}
	for _, sh := range append(names, os.Getenv("SHELL"), configured) {
		path, err := exec.LookPath(sh)
		if err != nil || !filepath.IsAbs(path) || seen[path] {
			continue
		}
		seen[path] = true
		shells = append(shells, termProfile{filepath.Base(path), []string{path}})
	}
	for _, tool := range []struct{ name, bin string }{{"Claude Code", "claude"}, {"Codex CLI", "codex"}, {"GitHub Copilot CLI", "copilot"}} {
		if path, err := exec.LookPath(tool.bin); err == nil && filepath.IsAbs(path) {
			agents = append(agents, termProfile{tool.name, []string{path}})
		}
	}
	return
}

func (a *App) selectTerminal(t *termTab) {
	a.term.view, a.term.active, a.term.focus = panelTerminal, t, t.active
	a.focusEditor = nil
}

func (a *App) cycleTerminal(step int) {
	tp := &a.term
	if n := len(tp.tabs); n > 0 {
		a.selectTerminal(tp.tabs[(slices.Index(tp.tabs, tp.active)+step+n)%n])
	}
}

func (a *App) terminalHeader(c *ui.Context) {
	tp := &a.term
	if tp.profiles == nil {
		tp.profiles, tp.agents = terminalProfiles(a.cfg.TermShell)
		tp.agent = min(tp.agent, max(0, len(tp.agents)-1))
		if tp.profiles == nil {
			tp.profiles = []termProfile{}
		}
	}
	pal := theme.Of(c)
	ui.Row(c).FillWidth().Height(editorTabBarHeight).Padding(0, 6).Gap(toolWindowActionGap).Background(pal.TabBar).BorderWidth(0, 0, 1, 0).BorderColor(pal.Hairline).AlignItems(ui.Center).Children(func() {
		title := locales.Terminal()
		switch tp.view {
		case panelOutput:
			title = locales.MsgOutput()
		case panelTasks:
			title = taskMessage("tasksTitle")
		}
		ui.Text(c, title).Bold().FontSize(widget.FontSize(c, 12)).Padding(0, 6).SingleLine().Shrink(0)
		if len(tp.tabs) > 0 {
			ui.ScrollHorizontal(c).Height(editorTabBarHeight).Shrink(1).MinWidth(0).Children(func() {
				ui.Row(c).FillHeight().AlignItems(ui.Stretch).Children(func() {
					for _, tab := range tp.tabs {
						active := tp.view == panelTerminal && tp.active == tab
						tabRow := ui.Row(c.Key(tab)).Padding(0, 6).Gap(3).AlignItems(ui.Center)
						if active {
							tabRow.BorderWidth(0, 0, 2, 0).BorderColor(pal.Muted)
							if tp.lastShown != tab {
								tabRow.ScrollIntoView()
							}
						}
						tabRow.ContextMenu(func(m *ui.Menu) { a.terminalTabActions(m, tab) })
						tabRow.Children(func() {
							b := ui.ButtonBase(c).KeepFocus().Label(tab.name).Tooltip(tab.active.title).Padding(1, 3)
							b.Children(func() { ui.Text(c, tab.name).FontSize(widget.FontSize(c, 12)).MaxWidth(120).SingleLine() })
							b.OnClick(func() { a.selectTerminal(tab) })
							if active || tabRow.Hovered() {
								toolWindowAction(c, glyph.Close, locales.CommandCloseTab()+" "+tab.name, locales.CommandCloseTab(), 0, false).OnClick(func() { a.closeTermTab(tab) })
							}
						})
					}
				})
			})
		}
		tp.lastShown = tp.active
		toolWindowAction(c, glyph.Plus, locales.CommandNewTerminal(), locales.CommandNewTerminal(), 0, false).OnClick(a.newTerminalTab)
		toolWindowAction(c, glyph.Restore, locales.TermProfiles(), locales.TermProfiles(), 0, false).Menu(func(m *ui.Menu) {
			for _, p := range tp.profiles {
				if m.Item(p.name).Chosen() {
					a.startTerminal(p)
				}
			}
			m.Separator()
			if m.Item(locales.TermSSH()).Chosen() {
				tp.dialog = &termDialog{}
			}
			for _, s := range a.cfg.TerminalSSH {
				if m.Item(sshLabel(s)).Chosen() {
					a.startSSH(s)
				}
			}
			if len(tp.agents) > 0 {
				m.Separator()
			}
			for i, p := range tp.agents {
				if m.Item(p.name).Chosen() {
					tp.agent = i
					a.startTerminal(p)
				}
			}
			m.Separator()
			if m.Item(locales.Settings()).Chosen() {
				a.openTerminalSettings()
			}
		})
		ui.Box(c).Grow(1)
		if len(tp.agents) > 0 {
			p := tp.agents[tp.agent]
			b := ui.ButtonBase(c).Label(p.name).Tooltip(p.name).KeepFocus().Padding(2, 5)
			b.Children(func() { ui.Text(c, p.name).FontSize(widget.FontSize(c, 12)).SingleLine() })
			b.OnClick(func() { a.startTerminal(p) })
			toolWindowAction(c, glyph.Restore, locales.TermAgents(), locales.TermAgents(), 0, false).Menu(func(m *ui.Menu) {
				for i, p := range tp.agents {
					if m.Item(p.name).Checked(i == tp.agent).Chosen() {
						tp.agent = i
						a.startTerminal(p)
					}
				}
			})
		}
		toolWindowAction(c, glyph.MoreTools, locales.TermMore(), locales.TermMore(), 0, false).Menu(func(m *ui.Menu) {
			if tp.view == panelTerminal && tp.active != nil {
				a.terminalTabActions(m, tp.active)
				m.Separator()
			}
			if m.Item(locales.MsgOutput()).Chosen() {
				tp.view = panelOutput
				tp.focus = nil
			}
			if m.Item(taskMessage("tasksTitle")).Chosen() {
				a.showTasks()
			}
			m.Separator()
			maxTitle := locales.TermMaximize()
			if tp.maximized {
				maxTitle = locales.TermRestore()
			}
			if m.Item(maxTitle).Chosen() {
				tp.maximized = !tp.maximized
			}
			if m.Item(locales.Settings()).Chosen() {
				a.openTerminalSettings()
			}
		})
		toolWindowAction(c, glyph.Minus, locales.TermHide(), locales.TermHide(), 0, false).OnClick(a.hideTerminal)
	})
}

func (a *App) terminalTabActions(m *ui.Menu, tab *termTab) {
	if m.Item(locales.TermRename()).Chosen() {
		a.term.dialog = &termDialog{tab: tab, text: tab.name}
	}
	if m.Item(locales.TermSplitRight()).Chosen() {
		a.selectTerminal(tab)
		a.splitTerminalPane(false)
	}
	if m.Item(locales.TermSplitDown()).Chosen() {
		a.selectTerminal(tab)
		a.splitTerminalPane(true)
	}
	if m.Item(locales.TermClear()).Chosen() {
		tab.active.term.ClearScreen()
		tab.active.term.ClearScrollback()
	}
	if tab.layout.pane == nil && m.Item(locales.TermClosePane()).Chosen() {
		a.closePane(tab.active)
	}
	if m.Item(locales.CommandCloseTab()).Chosen() {
		a.closeTermTab(tab)
	}
}

func (a *App) openTerminalSettings() {
	a.host.OpenSettings()
	a.settingsGo("terminal")
	a.host.RefreshSettings()
}

func sshLabel(s config.SSHSession) string {
	if s.Port != "" {
		return fmt.Sprintf("%s:%s", s.Destination, s.Port)
	}
	return s.Destination
}

var sshName = regexp.MustCompile(`^[a-zA-Z0-9_][a-zA-Z0-9._-]*$`)

func sshDestinationValid(dest string) bool {
	user, host, hasUser := strings.Cut(dest, "@")
	if !hasUser {
		host = dest
	} else if !sshName.MatchString(user) {
		return false
	}
	if strings.HasPrefix(host, "[") && strings.HasSuffix(host, "]") {
		return net.ParseIP(host[1:len(host)-1]) != nil
	}
	return sshName.MatchString(host) || net.ParseIP(host) != nil
}

func (a *App) startSSH(s config.SSHSession) {
	args := []string{"ssh"}
	if s.Port != "" {
		args = append(args, "-p", s.Port)
	}
	args = append(args, "--", s.Destination)
	a.startTerminal(termProfile{sshLabel(s), args})
	recent := slices.DeleteFunc(a.cfg.TerminalSSH, func(old config.SSHSession) bool { return s == old })
	a.cfg.TerminalSSH = slices.Insert(recent[:min(len(recent), 11)], 0, s)
	a.saveConfigSoon()
}

func (a *App) terminalDialog(c *ui.Context) {
	d := a.term.dialog
	open := true
	ui.DialogBase(c.Key("terminal-dialog"), &open, func(back, panel ui.Element) {
		back.Background(theme.Shade(.14))
		panel.Width(350).Padding(14).Radius(8).Background(theme.Of(c).Popover).Border(1, theme.Of(c).PopoverEdge)
		ui.Column(c).FillWidth().Gap(8).Children(func() {
			title, label, action := locales.TermSSH(), locales.TermDestination(), locales.TermConnect()
			if d.tab != nil {
				title, label, action = locales.TermRename(), locales.TermRename(), locales.MsgRename()
			}
			ui.Text(c, title).Bold()
			field := ui.TextInput(c, &d.text).FillWidth().Label(label).Placeholder(label).AutoFocus()
			submit := field.Submitted()
			text := strings.TrimSpace(d.text)
			valid := text != ""
			if d.tab == nil {
				port := ui.TextInput(c, &d.port).FillWidth().Label(locales.TermPort()).Placeholder(locales.TermPort())
				submit = submit || port.Submitted()
				n, err := strconv.Atoi(d.port)
				valid = sshDestinationValid(text) && (d.port == "" || err == nil && n > 0 && n <= 65535)
			}
			ui.Row(c).FillWidth().Gap(6).Children(func() {
				ui.Box(c).Grow(1)
				if ui.Button(c, locales.Cancel()).Clicked() {
					open = false
				}
				if ui.PrimaryButton(c, action).Disabled(!valid).Clicked() || valid && submit {
					if d.tab != nil {
						d.tab.name = text
					} else {
						a.startSSH(config.SSHSession{Destination: text, Port: d.port})
					}
					open = false
				}
			})
		})
	})
	if !open {
		a.term.dialog = nil
	}
}
