// INPUT: 提交选项（amend、签名等）与 workspace 路径。
// OUTPUT: Commit 工具窗口的标题行（Commit / Stash 标签、⋮、隐藏）、提交工具栏（刷新、回滚、显示差异、储藏、视图选项、定位、展开 / 折叠）与选项弹出按钮。
// POS: Commit 工具窗口顶部操作条。
// PROTOCOL: 变更时更新此头部，然后检查本包 doc.go

package app

import (
	"path/filepath"
	"time"

	"github.com/egoist/mygo/ui"

	"revtrail/internal/glyph"
	"revtrail/internal/theme"
	"revtrail/internal/widget"
	"revtrail/internal/workspace"
	"revtrail/locales"
)

// commitHeading is the Commit tool window's header, as IntelliJ's: its
// Commit and Stash tabs, then its ⋮ menu and Hide.
func (a *App) commitHeading(c *ui.Context) {
	pal := theme.Of(c)
	ui.Row(c).FillWidth().Height(editorTabBarHeight).Padding(0, 8, 0, sidebarInset).Gap(2).AlignItems(ui.Center).Children(func() {
		for _, tab := range []struct {
			label string
			stash bool
		}{{locales.MsgCommit(), false}, {locales.GitStashAction(), true}} {
			on := a.git.stashTab == tab.stash
			b := ui.ButtonBase(c.Key(tab.label)).Label(tab.label).Height(24).Padding(0, 6).Radius(4).Center().Shrink(0).KeepFocus()
			color := pal.Muted
			if on {
				color = pal.Text
			} else if b.Hovered() {
				b.Background(pal.Pill)
			}
			b.Children(func() {
				ui.Text(c, tab.label).FontSize(widget.FontSize(c, 13)).TextColor(color).SingleLine()
			})
			if b.Clicked() {
				a.git.stashTab = tab.stash
			}
		}
		ui.Box(c).Grow(1)
		ui.Row(c).Gap(toolWindowActionGap).Shrink(0).AlignItems(ui.Center).Children(func() {
			toolWindowAction(c, glyph.MoreVertical, locales.ToolOptions(), locales.ToolOptions(), 0, false).Menu(func(m *ui.Menu) {
				if !a.git.stashTab {
					a.commitViewOptions(m)
					return
				}
				if m.Item(locales.GitClearAll()).Disabled(a.git.busy || len(a.git.stash.entries) == 0).Chosen() {
					a.clearStashes(len(a.git.stash.entries))
				}
			})
			a.hideToolWindowAction(c)
		})
	})
}

func (a *App) commitToolbar(c *ui.Context) {
	ui.ScrollHorizontal(c).FillWidth().Height(editorTabBarHeight).BorderWidth(0, 0, 1, 0).BorderColor(theme.Of(c).Hairline).Padding(0, sidebarInset).Gap(toolWindowActionGap).AlignItems(ui.Center).Children(func() {
		if toolWindowAction(c, glyph.Refresh, "Refresh", locales.CommitRefresh(), 0, a.git.busy).Clicked() {
			a.git.scanned = time.Time{}
			a.refreshGit()
		}
		selected := a.git.selectedFiles()
		var selectedFile string
		if len(a.git.selected) == 1 {
			for key := range a.git.selected {
				row := a.git.rows.byKey[key]
				if row.dir != "" {
					selectedFile = filepath.Join(row.root, filepath.FromSlash(row.dir))
				} else if !row.group && len(row.files) == 1 {
					selectedFile = filepath.Join(row.root, filepath.FromSlash(row.files[0].Path))
				}
			}
		}
		if toolWindowAction(c, glyph.GitDiscard, locales.DiscardChanges(), locales.DiscardChanges(), 0, a.git.busy || len(selected) == 0).Clicked() {
			a.discardGitChanges(selected)
		}
		if toolWindowAction(c, glyph.ShowDiff, locales.CommitShowDiff(), locales.CommitShowDiff(), 0, len(selected) == 0).Clicked() {
			a.showGitDiff(selected[0], false)
		}
		if toolWindowAction(c, glyph.Stash, locales.GitStashChanges(), locales.GitStashChanges(), 0, a.git.busy || len(a.git.changes) == 0).Clicked() {
			a.promptStashChanges()
		}
		opts := toolWindowAction(c, glyph.CommitView, locales.CommitViewOptions(), locales.CommitViewOptions(), 0, false)
		opts.Menu(a.commitViewOptions)
		ui.Box(c).Size(1, 14).Background(theme.Of(c).Separator)
		if toolWindowAction(c, glyph.LocateChange, locales.CommitRevealFile(), locales.CommitRevealFile(), 0, selectedFile == "" || !workspace.Within(a.root, selectedFile)).Clicked() {
			a.navigator = navFiles
			a.revealInTree(selectedFile)
		}
		if toolWindowAction(c, glyph.ExpandChanges, locales.CommitExpandAll(), locales.CommitExpandAll(), 0, len(a.git.changes) == 0).Clicked() {
			a.git.changeTreeExpansion = changesExpanded
			clear(a.git.foldedChanges)
		}
		if toolWindowAction(c, glyph.CollapseChanges, locales.CommitCollapseAll(), locales.CommitCollapseAll(), 0, len(a.git.changes) == 0).Clicked() {
			a.git.changeTreeExpansion = changesCollapsed
			clear(a.git.foldedChanges)
		}
		if a.git.syncing != "" {
			ui.Box(c).Grow(1)
			ui.Spinner(c).FontSize(10).Tooltip(a.git.syncing)
		}
	})
}

// commitViewOptions are the change tree's view options, under the eye
// and the header's ⋮.
func (a *App) commitViewOptions(m *ui.Menu) {
	if m.Item(locales.CommitGroupDirectory()).Checked(a.git.groupByDirectory).Chosen() {
		a.git.groupByDirectory = !a.git.groupByDirectory
		a.saveConfigSoon()
	}
	if m.Item(locales.CommitIgnored()).Checked(a.git.showIgnored).Chosen() {
		a.git.showIgnored = !a.git.showIgnored
		a.saveConfigSoon()
		a.refreshGit()
	}
}

func (a *App) commitOptionsButton(c *ui.Context) {
	title := locales.CommitOptionsTitle()
	b := widget.GlyphButton(c, glyph.Gear, title, title, a.git.optionsOpen, a.git.busy)
	if b.Clicked() {
		a.git.optionsOpen = !a.git.optionsOpen
	}
	ui.PopoverBase(c, b, &a.git.optionsOpen, func(panel ui.Element) {
		pal := theme.Of(c)
		panel.Width(280).Padding(14).Radius(8).Background(pal.Popover).Border(1, pal.PopoverEdge).Shadow(0, 5, 18, 0, theme.Shade(.24))
		ui.Column(c).FillWidth().Gap(8).Children(func() {
			ui.Text(c, title).Bold()
			ui.Text(c, locales.CommitAuthor())
			ui.TextInput(c, &a.git.commitAuthor).FillWidth().Label(locales.CommitAuthor()).Placeholder("Name <email>")
			if ui.Checkbox(c, &a.git.signOff, locales.CommitSignOff()).Changed() {
				a.saveConfigSoon()
			}
			ui.Box(c).FillWidth().Height(1).Background(pal.Separator)
			ui.Text(c, locales.CommitChecks()).Bold()
			if ui.Checkbox(c, &a.git.checkWhitespace, locales.CommitCheckWhitespace()).Changed() {
				a.saveConfigSoon()
			}
			if ui.Checkbox(c, &a.git.checkGoFormat, locales.CommitCheckGoFormat()).Changed() {
				a.saveConfigSoon()
			}
		})
	})
}
