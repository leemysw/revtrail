// INPUT: 工作区、语言服务器状态与项目列表。
// OUTPUT: 顶部工具栏（项目徽标、设置按钮）、状态栏（路径、光标、行分隔符、编码、缩进）、项目选择器与活动指示。
// POS: 窗口顶部与底部的条。
// PROTOCOL: 变更时更新此头部，然后检查本包 doc.go

package app

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"

	"github.com/egoist/mygo/ui/editor"

	"revtrail/internal/glyph"
	"revtrail/internal/langserver"
	"revtrail/internal/languages"
	"revtrail/internal/theme"
	"revtrail/internal/widget"
	"revtrail/internal/workspace"
	"revtrail/locales"
)

// The native title bar and its traffic lights share this logical height,
// GoLand's main toolbar's.
const toolbarHeight = 32

// toolbarFontSize is the project and branch names' size in GoLand's main
// toolbar, which stays as it is when the UI font grows.
const toolbarFontSize = 13

func (a *App) toolbar(c *ui.Context) {
	bar := c.TitleBar()
	pal := theme.Of(c)
	left := max(bar.Left, 6) + 6
	if h, ok := a.host.(*windowHost); ok && bar.Left > 0 && placeTrafficLights(h.win) {
		left = trafficLightsEnd
	}
	ui.Row(c).FillWidth().Height(max(bar.Height, toolbarHeight)).Padding(0, bar.Right+6, 0, left).
		Gap(4).AlignItems(ui.Center).Background(pal.TitleBar).BorderWidth(0, 0, 1, 0).BorderColor(pal.Hairline).DragWindow().Children(func() {
		if a.root != "" {
			a.projectTitle(c)
			a.branchButton(c)
		}
		if a.git.root != "" {
			if widget.IconButtonWithState(c, glyph.Pull, locales.BranchUpdate(), locales.BranchUpdate()+" ("+widget.KeyLabel("Cmd+", "T")+")", false, a.git.busy || len(a.tracked()) == 0, 5).Clicked() {
				a.reviewUpdate()
			}
			if widget.IconButtonWithState(c, glyph.Push, locales.BranchPush(), locales.BranchPush()+" ("+widget.KeyLabel("Shift+Cmd+", "K")+")", false, a.git.busy && !a.git.autoFetching, 5).Clicked() {
				a.reviewPush()
			}
		} else {
			if widget.IconButtonWithState(c, glyph.Back, "Go Back", "Go Back", false, len(a.hist.Back) == 0, 6).Clicked() {
				a.goBack()
			}
			if widget.IconButtonWithState(c, glyph.Forward, "Go Forward", "Go Forward", false, len(a.hist.Forward) == 0, 6).Clicked() {
				a.goForward()
			}
		}
		ui.Box(c).Grow(1)
		if widget.IconButton(c, glyph.Search, locales.Search(), commandLabel("Search Everywhere"), false).Clicked() {
			a.searchEverywhere()
		}
		if widget.IconButton(c, glyph.Gear, locales.Settings(), locales.Settings(), false).Clicked() {
			a.host.OpenSettings()
		}

	})
}

// statusBar is IntelliJ's: where the file is, as a path of folders that
// each choose their row in the project view, what the app is doing, and
// the editor's caret, line separator, encoding and indent.
func (a *App) statusBar(c *ui.Context) {
	pal := theme.Of(c)
	ui.Row(c).Height(24).FillWidth().Padding(0, 10).Gap(10).AlignItems(ui.Center).
		Background(pal.Toolbar).BorderWidth(1, 0, 0, 0).BorderColor(pal.Hairline).Children(func() {
		ui.Row(c).Grow(1).Shrink(1).MinWidth(0).Gap(2).AlignItems(ui.Center).Clip().Children(func() { a.statusPath(c) })
		a.activityView(c)
		st := a.activeEditor()
		d := a.docs[a.activePath()]
		if st == nil || d == nil || d.buf == nil {
			return
		}
		pos := st.CursorPos()
		line := st.Buffer.Line(pos.Line)
		column := utf8.RuneCountInString(line[:min(pos.Col, len(line))]) + 1
		if statusWidget(c, "caret", fmt.Sprintf("%d:%d", pos.Line+1, column), commandLabel("Go to Line")).Clicked() {
			a.openQuickPanel(":")
		}
		separator := "LF"
		if d.buf.LineEnding() == "\r\n" {
			separator = "CRLF"
		}
		statusWidget(c, "line-separator", separator, locales.StatusLineSeparator())
		encoding := "UTF-8"
		if d.bom {
			encoding = "UTF-8 BOM"
		}
		statusWidget(c, "encoding", encoding, locales.StatusEncoding())
		statusWidget(c, "indent", indentLabel(st), locales.StatusIndent())
	})
}

// statusPath is the status bar's path of the file, or of the row chosen
// in the project view while it has the keyboard.
func (a *App) statusPath(c *ui.Context) {
	pal := theme.Of(c)
	path := a.activePath()
	if a.navigator == navFiles && !a.sidebarOff && a.treeRow >= 0 && a.treeRow < a.tree.Rows() {
		selected := a.tree.Item(a.treeRow)
		if !isPlaceholder(selected) && (path == "" || a.tree.List.Handle.FocusWithin(c)) {
			path = selected
		}
	}
	if d := a.docs[path]; d != nil && d.untitled {
		ui.Text(c, d.name).FontSize(widget.FontSize(c, 12)).TextColor(pal.Muted).SingleLine()
		return
	}
	if a.root == "" || !workspace.Within(a.root, path) {
		if path != "" {
			ui.Text(c, tildePath(path)).FontSize(widget.FontSize(c, 12)).TextColor(pal.Muted).SingleLine().Shrink(1)
		}
		return
	}
	crumbs := []string{a.root}
	if path != a.root {
		rel := workspace.RelPath(a.root, path)
		at := a.root
		for _, part := range strings.Split(rel, "/") {
			at = filepath.Join(at, part)
			crumbs = append(crumbs, at)
		}
	}
	for i, p := range crumbs {
		if i > 0 {
			glyph.Draw(c, glyph.Chevron, 9, pal.Dim)
		}
		b := ui.ButtonBase(c.Key(p)).Label(filepath.Base(p)).Tooltip(tildePath(p)).Height(20).Padding(0, 4).Gap(4).Radius(4).AlignItems(ui.Center).MinWidth(0).Shrink(1).KeepFocus()
		if b.Hovered() {
			b.Background(pal.Pill)
		}
		b.Children(func() {
			last := i == len(crumbs)-1
			if p == a.root {
				glyph.DrawFolder(c, 13)
			} else if last && !a.isTreeDir(p) {
				glyph.File(c, filepath.Base(p), 13)
			}
			ui.Text(c, filepath.Base(p)).FontSize(widget.FontSize(c, 12)).TextColor(pal.Muted).SingleLine().Ellipsis("…").MinWidth(0).Shrink(1)
		})
		b.OnClick(func() { a.selectInProject(p) })
	}
}

// statusWidget is one of the status bar's widgets on the right.
func statusWidget(c *ui.Context, key, label, tip string) ui.Element {
	b := ui.ButtonBase(c.Key(key)).Label(tip).Tooltip(tip).Height(20).Padding(0, 5).Radius(4).Center().Shrink(0).KeepFocus()
	if b.Hovered() {
		b.Background(theme.Of(c).Pill)
	}
	b.Children(func() {
		ui.Text(c, label).FontSize(widget.FontSize(c, 12)).TextColor(theme.Of(c).Muted).SingleLine()
	})
	return b
}

// indentLabel is how the editor indents, as IntelliJ's status bar says
// it: "4 spaces", or "Tab".
func indentLabel(st *editor.State) string {
	lang := st.Buffer.Language()
	size := st.TabSize
	if size <= 0 && lang != nil {
		size = lang.TabSize
	}
	if size <= 0 {
		size = 4
	}
	if st.Indent == editor.IndentTabs || st.Indent == editor.IndentAuto && lang != nil && lang.HardTabs {
		return locales.StatusTab()
	}
	return fmt.Sprintf(locales.StatusSpaces(), size)
}

func titleMenuButton(c *ui.Context, label, tip string, open bool) ui.Element {
	b := ui.ButtonBase(c.Key(label)).Label(label).Tooltip(tip).Height(26).Padding(0, 10).Gap(6).Radius(6).MinWidth(0).Shrink(1).KeepFocus()
	if open || b.Hovered() || b.Pressed() {
		b.Background(theme.Of(c).Pill)
	}
	return b
}

type projectPicker struct {
	open     bool
	button   ui.Handle
	query    string
	selected int
	list     ui.ListState
}

func titlePickerPanel(c *ui.Context, anchor ui.Rect, preferredWidth float32, open *bool, body func(float32)) {
	width, height := c.Size()
	w := min(preferredWidth, width-12)
	top := anchor.Y + anchor.H + 4
	ui.Overlay(c, func() {
		back := ui.Box(c).Absolute().Top(0).Left(0).Right(0).Bottom(0).Modal()
		back.OnClick(func() { *open = false })
		panel := ui.Column(c).Absolute().Left(max(6, min(anchor.X, width-w-6))).Top(top).Width(w).Padding(6).Gap(4).Radius(7).
			Background(theme.Of(c).Popover).Border(1, theme.Of(c).PopoverEdge).Shadow(0, 6, 20, 0, theme.Shade(.28))
		panel.OnClick(func() {})
		panel.Children(func() { body(height - top - 6) })
	})
}

func (a *App) projectTitle(c *ui.Context) {
	p := &a.projectPicker
	b := titleMenuButton(c, locales.MsgSwitchProject(), tildePath(a.root), p.open).Bind(&p.button)
	b.OnClick(func() {
		a.git.picker.open = false
		p.open = !p.open
		p.query = ""
		p.selected = max(0, slices.Index(a.cfg.Recent, a.root))
		p.list.ScrollTo(p.selected, ui.Center)
	})
	b.Padding(0, 8, 0, 4)
	b.Children(func() {
		projectBadge(c, a.root, 16)
		ui.Text(c, filepath.Base(a.root)).FontSize(toolbarFontSize).SingleLine().Ellipsis("…").MinWidth(0).Shrink(1)
		glyph.Draw(c, glyph.Restore, 16, theme.Of(c).Muted)
	})
}

func (a *App) projectPickerPanel(c *ui.Context) {
	p := &a.projectPicker
	titlePickerPanel(c, p.button.Bounds(c), 320, &p.open, func(available float32) {
		pal := theme.Of(c)
		var field ui.Element
		ui.Row(c).FillWidth().Height(29).Padding(0, 6).Gap(5).AlignItems(ui.Center).Radius(4).Border(1, pal.PopoverEdge).Children(func() {
			glyph.Draw(c, glyph.Search, 14, pal.Muted)
			field = ui.TextInput(c.Key("project-search"), &p.query).Placeholder(locales.Search()).Label(locales.MsgRecentProjects()).Grow(1).MinWidth(0).AutoFocus().BorderWidth(0).Background(ui.Transparent)
		})
		var paths []string
		query := strings.ToLower(strings.TrimSpace(p.query))
		for _, path := range a.cfg.Recent {
			if strings.Contains(strings.ToLower(tildePath(path)), query) {
				paths = append(paths, path)
			}
		}
		// The final row always opens the folder chooser.
		last := len(paths)
		p.selected = min(p.selected, last)
		choose := func(i int) {
			p.open = false
			if i == last {
				a.openFolderDialog()
			} else {
				a.openRecent(paths[i])
			}
		}
		previous := p.selected
		switch {
		case field.Shortcut(0, ui.KeyEscape):
			p.open = false
		case field.Shortcut(ui.Cmd, ui.KeyO):
			choose(last)
		case field.Submitted():
			choose(p.selected)
		case field.Changed():
			p.selected = 0
		case field.Shortcut(0, ui.KeyDown):
			p.selected = min(last, p.selected+1)
		case field.Shortcut(0, ui.KeyUp):
			p.selected = max(0, p.selected-1)
		}
		if previous != p.selected {
			p.list.ScrollTo(p.selected, ui.Center)
		}
		ui.List(c, &p.list, last+1, func(i int) {
			label, tip := locales.MsgOpenFolder(), locales.MsgOpenFolder()
			if i < last {
				label, tip = projectLabel(a.cfg.Recent, paths[i]), tildePath(paths[i])
			}
			row := ui.ButtonBase(c.Key(i)).KeepFocus().FillWidth().Height(24).Padding(1, 8).Gap(6).Radius(4).AlignItems(ui.Center).Label(label).Tooltip(tip)
			if i == p.selected {
				row.Background(pal.ListSelection)
			} else if row.Hovered() {
				row.Background(pal.Pill)
			}
			if i == last && i > 0 {
				row.BorderWidth(1, 0, 0, 0).BorderColor(pal.Separator)
			}
			row.Children(func() {
				if i < last {
					projectBadge(c, paths[i], 16)
				} else {
					glyph.Draw(c, glyph.WelcomeOpen, 14, pal.Muted)
				}
				ui.Text(c, label).SingleLine().Ellipsis("…").MinWidth(0).Grow(1)
				if i < last && paths[i] == a.root {
					glyph.Draw(c, glyph.Check, 12, pal.Muted)
				}
				if i == last {
					ui.Text(c, widget.KeyLabel("Cmd+", "O")).TextColor(pal.Muted)
				}
			})
			row.OnClick(func() { choose(i) })
		}).FillWidth().Height(min(float32((last+1)*24), 360, max(24, available-50)))
	})
}

// projectBadge is the project's tile, as IntelliJ's beside the project's
// name: its initials on a color that its path picks.
func projectBadge(c *ui.Context, root string, size float32) {
	sum := 0
	for _, r := range root {
		sum = sum*31 + int(r)
	}
	color := theme.ProjectBadges[(sum%len(theme.ProjectBadges)+len(theme.ProjectBadges))%len(theme.ProjectBadges)]
	ui.Box(c).Size(size, size).Radius(5).Background(color).Center().Shrink(0).Children(func() {
		ui.Text(c, projectInitials(filepath.Base(root))).FontSize(size * 0.5).Bold().TextColor(theme.OnAccent).SingleLine()
	})
}

// projectInitials are the first letters of a project name's first two
// words, as IntelliJ's tile: "plumb" is P, "my-app" is MA.
func projectInitials(name string) string {
	words := strings.FieldsFunc(name, func(r rune) bool { return r == '-' || r == '_' || r == '.' || r == ' ' })
	out := ""
	for _, w := range words[:min(2, len(words))] {
		r, _ := utf8.DecodeRuneInString(w)
		out += string(r)
	}
	if out == "" {
		out = "?"
	}
	return strings.ToUpper(out)
}

// projectLabel names a recent folder by its last part, and its parent's
// too where another recent folder has the same name, as Peek does.
func projectLabel(recent []string, p string) string {
	name := filepath.Base(p)
	for _, r := range recent {
		if r != p && filepath.Base(r) == name {
			return name + " — " + tildePath(filepath.Dir(p))
		}
	}
	return name
}

func (a *App) activeLang() (langKind, string) {
	if a.focus != nil {
		p := a.activePath()
		switch {
		case languages.TypeScriptLanguage(p) != "":
			return langTS, "tsrs"
		case languages.ClangLanguage(p) != "":
			return langClang, "clangd"
		case languages.CMakeLanguage(p) != "":
			return langCMake, "cmake-language-server"
		}
	}
	return langGo, "gopls"
}

// activityView is the pill in the middle of the toolbar, as Xcode's: what
// the app is doing, and what went wrong.
func (a *App) activityView(c *ui.Context) {
	kind, name := a.activeLang()
	sl := a.langs[kind]
	state, serverErr := sl.state, sl.err
	retry := func() { a.retryServer(kind) }
	pill := ui.Row(c).Height(16).MaxWidth(420).Shrink(1).Gap(5).AlignItems(ui.Center)
	pill.Children(func() {
		// The dot tells the state at a glance, as Peek's toolbar does: the
		// accent while work goes on, red when it went wrong.
		dot := theme.Of(c).Muted
		switch {
		case a.finding, a.find.running, state == langserver.Starting:
			dot = c.Theme().Accent
		case a.noticeError, state == langserver.Failed, a.watchErr != nil:
			dot = theme.Of(c).Err
		}
		if dot != theme.Of(c).Muted {
			ui.Box(c).Size(6, 6).Radius(3).Background(dot).Shrink(0)
		}
		switch {
		case a.finding:
			ui.Spinner(c).FontSize(11)
			ui.Text(c, a.findingLabel).FontSize(widget.FontSize(c, 12)).TextColor(theme.Of(c).Muted).SingleLine()
		case a.notice != "":
			t := ui.Text(c, a.notice).FontSize(widget.FontSize(c, 12)).SingleLine().Shrink(1).Tooltip(a.notice)
			if a.noticeError {
				t.TextColor(theme.Of(c).Err)
			}
			for _, act := range a.noticeActions {
				if ui.Button(c, act.label).Padding(0, 6).FontSize(widget.FontSize(c, 11)).Clicked() {
					a.note("", false)
					act.fn()
				}
			}
			if state == langserver.Failed && ui.Button(c, locales.Retry()).Label(locales.MsgRetry()+name).Padding(0, 6).FontSize(widget.FontSize(c, 11)).Clicked() {
				retry()
			}
		case a.find.running:
			ui.Spinner(c).FontSize(11)
			ui.Text(c, locales.Searching()).FontSize(widget.FontSize(c, 12)).TextColor(theme.Of(c).Muted)
		case state == langserver.Starting:
			ui.Spinner(c).FontSize(11)
			ui.Text(c, locales.MsgStarting()+name+"…").FontSize(widget.FontSize(c, 12)).TextColor(theme.Of(c).Muted)
		case state == langserver.Failed:
			msg := name + " unavailable"
			if serverErr != nil {
				msg = name + ": " + widget.ShortErr(serverErr)
			}
			ui.Text(c, msg).FontSize(widget.FontSize(c, 12)).TextColor(theme.Of(c).Err).SingleLine().Shrink(1)
			if ui.Button(c, locales.Retry()).Label(locales.MsgRetry()+name).Padding(0, 6).FontSize(widget.FontSize(c, 11)).Clicked() {
				retry()
			}
		case a.watchErr != nil:
			ui.Text(c, locales.MsgNotWatchingForChanges()).FontSize(widget.FontSize(c, 12)).TextColor(theme.Of(c).Muted).Tooltip(a.watchErr.Error())
		}
	})
}
