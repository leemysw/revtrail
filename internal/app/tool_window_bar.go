// INPUT: 导航栏状态。
// OUTPUT: 左侧工具窗口条与导航切换。
// POS: 工具窗口条绘制。
// PROTOCOL: 变更时更新此头部，然后检查本包 doc.go

package app

import (
	"github.com/egoist/mygo/ui"

	"revtrail/internal/glyph"
	"revtrail/internal/theme"
	"revtrail/internal/widget"
	"revtrail/locales"
)

func (a *App) toggleNavigator(mode int) {
	shown := !a.sidebarOff && !a.term.maximized && a.navigator == mode
	a.sidebarOff = shown
	a.navigator = mode
	if !shown {
		a.term.maximized = false
		if mode == navGit {
			a.showCommitNavigator()
		}
	}
	a.saveConfigSoon()
}

func (a *App) toolWindowBar(c *ui.Context) {
	pal := theme.Of(c)
	ui.Column(c.Key("tool-windows")).Width(33).Shrink(0).FillHeight().Padding(3, 0).Gap(8).AlignItems(ui.Center).Background(pal.Sidebar).BorderWidth(0, 1, 0, 0).BorderColor(pal.Hairline).Children(func() {
		button := func(icon *ui.SVG, label, tip string, active bool) ui.Element {
			b := ui.ButtonBase(c.Key(label)).Size(24, 24).Center().KeepFocus().Radius(5).Label(label)
			tint := pal.Muted
			if active {
				b.Background(pal.ListSelectionInactive)
				tint = pal.Text
			} else if b.Hovered() {
				b.Background(pal.Pill)
				tint = pal.Text
			}
			b.Children(func() { ui.Icon(c, icon).Size(16, 16).TextColor(tint) })
			ui.TooltipBase(c, b, func(t ui.Element) {
				t.AttachTo(b, ui.AnchorRight, ui.AnchorLeft).Margin(0, 0, 0, 6).Padding(5, 8).Radius(4).Background(pal.Popover).Border(1, pal.PopoverEdge).Children(func() { ui.Text(c, tip).FontSize(widget.FontSize(c, 12)).SingleLine() })
			})
			return b
		}
		nav := func(mode int, icon *ui.SVG, label, tip string) {
			button(icon, label, tip, !a.sidebarOff && !a.term.maximized && a.navigator == mode).OnClick(func() { a.toggleNavigator(mode) })
		}
		nav(navFiles, glyph.WelcomeFolder, "Show the Project Navigator", locales.NavigatorProject())
		nav(navGit, glyph.CommitToolWindow, "Show the Git Navigator", locales.MsgCommit())
		ui.Box(c).Size(20, 1).Background(pal.Separator)
		button(glyph.MoreTools, locales.ToolMore(), locales.ToolMore(), false).Menu(func(m *ui.Menu) {
			if m.Item(locales.ToolStructure()).Chosen() {
				a.toggleNavigator(navStructure)
			}
			if m.Item(locales.ProblemsList()).Chosen() {
				a.toggleNavigator(navProblems)
			}
			if m.Item(taskMessage("tasksTitle")).Chosen() {
				a.showTasks()
			}
			m.Separator()
			if m.Item(locales.FindFilesTitle()).Chosen() {
				a.openFindPopup()
			}
			if m.Item(locales.MsgOutput()).Chosen() {
				a.term.open, a.term.view, a.term.focus = true, panelOutput, nil
			}
			if m.Item(locales.Settings()).Chosen() {
				a.host.OpenSettings()
			}
		})
		ui.Box(c).Grow(1)
		button(glyph.Prompt, "Show Terminal", locales.Terminal()+"  "+widget.KeyLabel("Alt+", "F12"), a.term.open && a.term.view == panelTerminal).OnClick(a.toggleTerminal)
		button(glyph.Git, locales.ToolGitLog(), locales.ToolGitLog(), a.term.open && a.term.view == panelGitLog).OnClick(func() {
			if a.term.open && a.term.view == panelGitLog {
				a.hideTerminal()
			} else {
				a.showGitLogPanel()
			}
		})
	})
}
