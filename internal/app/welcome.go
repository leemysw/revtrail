// INPUT: config 最近项目与 assets 图标。
// OUTPUT: 欢迎窗口：介绍、操作与最近项目列表。
// POS: 未打开文件夹时的窗口。
// PROTOCOL: 变更时更新此头部，然后检查本包 doc.go

package app

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo/ui"

	"revtrail/internal/assets"
	"revtrail/internal/glyph"
	"revtrail/internal/theme"
	"revtrail/internal/widget"
	"revtrail/internal/workspace"
	"revtrail/locales"
)

// The welcome window, after Xcode's: the app's icon, name and actions on
// the left, the folders opened lately on the right.

var (
	appIcon = assets.Icon
)

// welcomeWindow fills the welcome window (welcomeWindowSize), its title
// bar hidden behind the left side, as Xcode's.
func (a *App) welcomeWindow(c *ui.Context) {
	ui.Row(c).Fill().DragWindow().Children(func() {
		a.welcomeIntro(c)
		a.welcomeRecent(c)
	})
}

// The welcome window's size, which does not change, as Xcode's.
const welcomeWidth, welcomeHeight = 740, 440

// welcomeIntro is the left side: the icon, the name, the version and what
// to do.
func (a *App) welcomeIntro(c *ui.Context) {
	ui.Column(c).Grow(1).Shrink(1).FillHeight().Background(theme.Of(c).WelcomeLeft).Padding(16, 28, 18, 28).Children(func() {
		ui.Box(c).Height(26) // under the window's buttons
		ui.Column(c).Grow(1).FillWidth().AlignItems(ui.Center).Gap(6).Children(func() {
			ui.Image(c, appIcon).Size(120, 120)
			ui.Text(c, "Revtrail").FontSize(widget.FontSize(c, 28)).Bold().TextColor(theme.Of(c).Text).TextAlign(ui.Center)
			if a.version != "" {
				ui.Text(c, locales.MsgVersion()+a.version).FontSize(widget.FontSize(c, 13)).TextColor(theme.Of(c).Muted)
			}
			ui.Box(c).Height(12)
			ui.Column(c).Gap(2).Width(300).Children(func() {
				a.welcomeAction(c, glyph.WelcomeOpen, "Open Folder…", "Read the source of a folder", widget.ShortcutLabel("O"), a.openFolderDialog)
				a.welcomeAction(c, glyph.WelcomeOpen, "Open File…", "Read a single file", "", a.openFileDialog)
				if last := a.lastFolder(); last != "" {
					a.welcomeAction(c, glyph.WelcomeReopen, "Reopen "+filepath.Base(last), tildePath(last), "", func() { a.openRecent(last) })
				}
			})
			if a.welcomeNote != "" {
				ui.Text(c, a.welcomeNote).FontSize(widget.FontSize(c, 12)).TextColor(theme.Of(c).Err).MaxWidth(380).TextAlign(ui.Center).Padding(8, 0, 0, 0)
			}
		})
		showWelcome := !a.cfg.ReopenLast
		ui.Checkbox(c, &showWelcome, "Show this window when Revtrail launches").FontSize(widget.FontSize(c, 12)).TextColor(theme.Of(c).Muted).OnClick(func() {
			a.cfg.ReopenLast = !showWelcome
			a.saveConfig()
		})
	})
}

// lastFolder is the folder opened last, unless it is the one open now.
func (a *App) lastFolder() string {
	if len(a.cfg.Recent) == 0 || a.cfg.Recent[0] == a.root {
		return ""
	}
	return a.cfg.Recent[0]
}

func (a *App) welcomeAction(c *ui.Context, icon *ui.SVG, title, detail, shortcut string, fn func()) {
	row := ui.Row(c).FillWidth().Padding(6, 8).Gap(10).AlignItems(ui.Center).Radius(8).Cursor(ui.CursorPointer).Label(title)
	if row.Hovered() {
		row.Background(theme.Of(c).WelcomeHover)
	}
	if shortcut != "" {
		row.Tooltip(shortcut)
	}
	row.OnClick(fn).Children(func() {
		ui.Box(c).Size(30, 30).Shrink(0).Radius(7).Background(theme.Of(c).WelcomeTile).Center().Children(func() {
			glyph.Draw(c, icon, 16, theme.Of(c).Text)
		})
		ui.Column(c).Grow(1).Shrink(1).Gap(1).Children(func() {
			ui.Text(c, title).FontSize(widget.FontSize(c, 13)).Bold().TextColor(theme.Of(c).Text).SingleLine()
			ui.Text(c, detail).FontSize(widget.FontSize(c, 11)).TextColor(theme.Of(c).Muted).SingleLine()
		})
	})
}

// welcomeRecent is the right side: the folders opened lately, newest
// first.
func (a *App) welcomeRecent(c *ui.Context) {
	ui.Column(c).Width(280).Shrink(0).FillHeight().Background(theme.Of(c).WelcomeRight).
		BorderWidth(0, 0, 0, 1).BorderColor(theme.Of(c).Separator).Padding(10, 8).Children(func() {
		recent := a.cfg.Recent
		if len(recent) == 0 {
			ui.Column(c).Grow(1).FillWidth().Center().Children(func() {
				ui.Text(c, locales.MsgNoRecentFolders()).FontSize(widget.FontSize(c, 13)).TextColor(theme.Of(c).Dim)
			})
			return
		}
		var forget string
		ui.Scroll(c).Grow(1).FillWidth().Children(func() {
			ui.Column(c).FillWidth().Gap(2).Children(func() {
				for i, p := range recent {
					a.recentRow(c.Key(i), p, &forget)
				}
			})
		})
		if forget != "" {
			a.cfg.RemoveRecent(forget)
			a.saveConfig()
		}
	})
}

func (a *App) recentRow(c *ui.Context, p string, forget *string) {
	current := p == a.root
	row := ui.Row(c).FillWidth().Padding(7, 10).Gap(10).AlignItems(ui.Center).Radius(7).Cursor(ui.CursorPointer).Label(p).Tooltip(p)
	switch {
	case current:
		row.Background(a.theme.Selection)
	case row.Hovered():
		row.Background(theme.Of(c).WelcomeHover)
	}
	row.OnClick(func() { a.openRecent(p) })
	row.ContextMenu(func(m *ui.Menu) {
		if m.Item("Open").Chosen() {
			a.openRecent(p)
		}
		m.Separator()
		if m.Item("Remove from Recents").Chosen() {
			*forget = p
		}
		if m.Item("Clear Recents").Chosen() {
			a.cfg.ClearRecent()
		}
	})
	row.Children(func() {
		glyph.Draw(c, glyph.WelcomeFolder, 22, theme.Of(c).Folder)
		ui.Column(c).Grow(1).Shrink(1).Gap(1).Children(func() {
			ui.Text(c, filepath.Base(p)).FontSize(widget.FontSize(c, 13)).Bold().TextColor(theme.Of(c).Text).SingleLine()
			ui.Text(c, tildePath(filepath.Dir(p))).FontSize(widget.FontSize(c, 11)).TextColor(theme.Of(c).Muted).SingleLine()
		})
	})
}

// openRecent opens a recent folder, or goes back to it when it is open.
func (a *App) openRecent(p string) {
	if p == a.root {
		a.showWelcome = false
		return
	}
	a.openProject(p)
}

// tildePath shows a path in the home directory with ~.
func tildePath(p string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" || !workspace.Within(home, p) {
		return p
	}
	if p == home {
		return "~"
	}
	return "~" + string(filepath.Separator) + strings.TrimPrefix(p, home+string(filepath.Separator))
}
