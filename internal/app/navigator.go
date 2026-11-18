// INPUT: 工作区目录、树选择与 git 状态。
// OUTPUT: 左侧导航栏、Project 标题与悬停显示的操作（含 ⋮ 与隐藏）、项目树（项目文件夹为根节点）、单击 / 双击 / 键盘打开、根节点菜单。
// POS: Project 工具窗口；文件管理逻辑在 tree_files.go。
// PROTOCOL: 变更时更新此头部，然后检查本包 doc.go

package app

import (
	"fmt"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"revtrail/internal/glyph"
	"revtrail/internal/theme"
	"revtrail/internal/widget"
	"revtrail/internal/workspace"
	"revtrail/locales"
)

// The tool window bar selects the navigator shown here.
func (a *App) navigatorPane(c *ui.Context) {
	pane := ui.Column(c).Fill().Background(theme.Of(c).Sidebar)
	pane.Children(func() {
		if a.navigator == navGit {
			a.gitNavigator(c)
		} else if a.navigator == navStructure {
			a.structureNavigator(c)
		} else if a.navigator == navProblems {
			a.problemsNavigator(c)
		} else {
			a.projectNavigator(c)
		}
	})
}

func (a *App) projectNavigator(c *ui.Context) {
	// The tree's folder rows take drops before this root target.
	container := ui.Column(c).Grow(1).FillWidth()
	if paths := container.DroppedFiles(); paths != nil {
		a.copyInto(a.root, paths)
	}
	if v, ok := ui.Drop[*tabDrag](container); ok && v.from == nil && v.change == nil {
		a.copyInto(a.root, []string{v.path})
	}
	// The header's actions show while the pointer is over the tool window
	// or it has the keyboard, as IntelliJ's.
	actions := container.Hovered() || a.tree.List.Handle.FocusWithin(c)
	container.Children(func() { a.projectNavigatorContent(c, actions) })
}

func (a *App) projectNavigatorContent(c *ui.Context, actions bool) {
	a.syncFilter()
	a.treeHeading(c, actions)
	switch {
	case a.filtering():
		switch {
		case a.quick.err != nil:
			ui.Column(c).Grow(1).Padding(8, 12).Gap(6).Children(func() {
				ui.Text(c, locales.MsgCouldNotListTheFiles()+describeErr(a.quick.err)).TextColor(theme.Of(c).Err).FontSize(widget.FontSize(c, 12))
				if ui.Button(c, locales.Retry()).Clicked() {
					a.retryFiles()
				}
			})
		case a.quick.files == nil:
			ui.Row(c).Padding(8, 12).Gap(8).Children(func() {
				ui.Spinner(c)
				ui.Text(c, locales.MsgListingFiles()).TextColor(theme.Of(c).Muted)
			})
		case a.filter.matches == 0:
			ui.Column(c).Grow(1).FillWidth().AlignItems(ui.Center).Justify(ui.Center).Children(func() {
				ui.Text(c, locales.MsgNoResults()).TextColor(theme.Of(c).Muted)
			})
		default:
			a.fileTree(c, a.filter.roots)
		}
	default:
		// The project's folder is the tree's root node, as IntelliJ's:
		// its row shows while the entries load, or why they did not.
		a.fileTree(c, []string{a.root})
	}
	if a.filter.toList {
		a.filter.toList = false
		if a.tree.Rows() > 0 {
			a.tree.List.Handle.Focus(c)
		}
	}
}

// treeHeading is the tool window's header, as IntelliJ's: its name, what
// the filter matches, and the actions on the tree while they show.
func (a *App) treeHeading(c *ui.Context, actions bool) {
	ui.Row(c).FillWidth().Height(editorTabBarHeight).Padding(0, 8, 0, sidebarInset+4).Gap(6).AlignItems(ui.Center).Children(func() {
		searching := a.filter.focus || a.filter.query != "" || a.filter.field.Focused(c)
		switch {
		case searching:
			a.filterBar(c)
		case a.filter.speed.active():
			a.treeSearchField(c)
		default:
			ui.Text(c, locales.NavigatorProject()).Bold().FontSize(widget.FontSize(c, 13)).SingleLine().Grow(1).Shrink(1)
		}
		if a.filtering() {
			detail := fmt.Sprintf("%d of %d", a.filter.matches, len(a.quick.files))
			if a.quick.files == nil {
				detail = "Filtering…"
			}
			ui.Text(c, detail).FontSize(widget.FontSize(c, 11)).TextColor(theme.Of(c).Dim).SingleLine().Shrink(1)
		}
		if !actions && !searching {
			return
		}
		ui.Row(c).Gap(toolWindowActionGap).Shrink(0).AlignItems(ui.Center).Children(func() {
			if toolWindowAction(c, glyph.LocateChange, commandLabel("Select in Project View"), commandLabel("Select in Project View"), 0, a.activePath() == "").Clicked() {
				a.selectActiveInProject()
			}
			if toolWindowAction(c, glyph.CollapseChanges, "Collapse All", "Collapse All", 0, a.filtering()).Clicked() {
				a.tree.Open.Clear()
				a.tree.Open.Add(a.root)
				a.reveal = ""
				a.setTreeRow(0)
			}
			toolWindowAction(c, glyph.MoreVertical, locales.ToolOptions(), locales.ToolOptions(), 0, false).Menu(func(m *ui.Menu) {
				if m.Item(locales.MsgNewFile()).Chosen() {
					a.beginEntry(a.entryTarget(), false)
				}
				if m.Item(locales.ProjectNewFolder()).Chosen() {
					a.beginEntry(a.entryTarget(), true)
				}
				m.Separator()
				if m.Item(locales.ProjectReload()).Chosen() {
					a.refreshAll()
				}
			})
			a.hideToolWindowAction(c)
		})
	})
}

// hideToolWindowAction is the — ending a tool window's header.
func (a *App) hideToolWindowAction(c *ui.Context) {
	if toolWindowAction(c, glyph.Hide, locales.ToolHide(), locales.ToolHide(), 0, false).Clicked() {
		a.sidebarOff = true
	}
}

func (a *App) projectRootMenu(m *ui.Menu, services ui.Services) {
	root := a.root
	m.Submenu(locales.ProjectMenuNew(), func(sub *ui.Menu) {
		if sub.Item(locales.MsgNewFile()).Chosen() {
			a.beginEntry(root, false)
		}
		if sub.Item(locales.ProjectNewFolder()).Chosen() {
			a.beginEntry(root, true)
		}
	})
	m.Separator()
	if m.Item(locales.Copy()).Chosen() {
		a.copyFiles([]string{root})
	}
	a.copyReferenceMenu(m, []string{root})
	paths := clipboardFiles(services)
	if m.Item(locales.MsgPaste()).Disabled(len(paths) == 0).Chosen() {
		a.pasteInto(root, paths)
	}
	m.Separator()
	if m.Item(locales.FindFilesTitle()).Chosen() {
		a.find.mode, a.find.editingScope = findProject, false
		a.openFindPopup()
	}
	m.Submenu(locales.ProjectOpenIn(), func(sub *ui.Menu) {
		label := locales.ProjectFileManager()
		if runtime.GOOS == "darwin" {
			label = "Finder"
		}
		if sub.Item(label).Chosen() {
			if err := mygo.Shell.OpenPath(root); err != nil {
				a.note(locales.MsgCouldNotOpen()+describeErr(err), true)
			}
		}
		if sub.Item(locales.Terminal()).Chosen() {
			a.newTerminalTab()
		}
	})
	if a.git.root != "" {
		m.Submenu("Git", func(sub *ui.Menu) {
			if sub.Item(locales.MsgCommit()).Chosen() {
				a.showCommitNavigator()
			}
			if sub.Item(locales.ToolGitLog()).Chosen() {
				a.showGitLogPanel()
			}
		})
	}
	m.Separator()
	if m.Item(locales.ProjectReload()).Chosen() {
		a.refreshAll()
	}
}

const toolWindowActionGap = 1

func toolWindowAction(c *ui.Context, icon *ui.SVG, label, tip string, count int, disabled bool) ui.Element {
	pal := theme.Of(c)
	b := ui.ButtonBase(c.Key(label)).Label(label).Tooltip(tip).Height(24).MinWidth(24).Padding(0, 4).Gap(1).Radius(4).Center().Shrink(0).KeepFocus().Disabled(disabled)
	tint := pal.Muted
	if disabled {
		tint = pal.Dim
	} else {
		if count > 0 {
			tint = c.Theme().Accent
		}
		if b.Hovered() || b.Pressed() {
			b.Background(pal.Pill)
		}
	}
	b.Children(func() {
		ui.Icon(c, icon).Size(16, 16).Shrink(0).TextColor(tint)
		if count > 0 {
			ui.Text(c, fmt.Sprint(count)).SingleLine().FontSize(widget.FontSize(c, 10)).Bold().TextColor(tint)
		}
	})
	return b
}

// Filter Files also searches descendants of unopened folders.
func (a *App) filterBar(c *ui.Context) {
	field := ui.SearchField(c.Key("workspace-filter"), &a.filter.query).Grow(1).Height(24).
		Label("Filter Files").Bind(&a.filter.field)
	if a.filter.focus {
		a.filter.focus = false
		a.filter.field.Focus(c)
	}
	if field.Shortcut(0, ui.KeyDown) {
		a.filter.toList = true
	}
}

// retryFiles lists the workspace's files again after listing failed.
func (a *App) retryFiles() {
	a.quick.files, a.quick.err, a.quick.listing = nil, nil, false
	a.filter.forQuery = ""
	a.ensureFiles()
}

// sidebarInset is how far a source list's rows, and their selection,
// stand in from the sidebar's edges, as AppKit's on macOS 26 and later.
const sidebarInset = 10

func (a *App) fileTree(c *ui.Context, roots []string) {
	a.tree.Indent = 12
	// Outline paints the entire selected row, including its disclosure
	// arrow, with Accent: blue while the tree has the keyboard, gray when
	// it has not, as IntelliJ's.
	base := c.Theme()
	treeTheme := *base
	treeTheme.Accent, treeTheme.AccentText = theme.Of(c).ListSelectionInactive, theme.Of(c).Text
	if a.tree.List.Handle.FocusWithin(c) {
		treeTheme.Accent = theme.Of(c).ListSelection
	}
	c.SetTheme(&treeTheme)
	tree := ui.Outline(c, &a.tree, roots, a.children, func(p string) { a.treeItem(c, p) }).Grow(1).FillWidth().Padding(0, sidebarInset).Label(locales.Files())
	c.SetTheme(base)
	if a.renameEntry == nil && a.createEntry == nil && !a.filtering() {
		tree.TextCaret(ui.Rect{X: sidebarInset + 20, Y: -2, H: 1}).HandleInput(a.filter.speed.input)
	}
	// Choose the row of the active document once it shows.
	if a.reveal != "" {
		for i := range a.tree.Rows() {
			if a.tree.Item(i) == a.reveal {
				a.setTreeRow(i)
				a.tree.List.ScrollTo(i, ui.Center)
				a.reveal = ""
				break
			}
		}
	}
	// As IntelliJ's project view: Enter, F4 and Cmd+Down open the item
	// chosen, Shift+F6 renames it, Delete moves it to the Trash.
	chosen := func(act func(string)) func() {
		return func() {
			if a.treeRow >= 0 && a.treeRow < a.tree.Rows() {
				if p := a.tree.Item(a.treeRow); !strings.HasSuffix(p, entrySuffix) && !strings.HasSuffix(p, loadingSuffix) && !strings.HasSuffix(p, errorSuffix) {
					act(p)
				}
			}
		}
	}
	open := chosen(func(p string) {
		if a.isTreeDir(p) {
			a.toggleDir(p)
		} else {
			a.openFromTree(p)
		}
	})
	if tree.Submitted() {
		open()
	}
	if a.renameEntry == nil && a.createEntry == nil {
		h := a.tree.List.Handle
		h.OnShortcut(c, 0, ui.KeyF4, open)
		h.OnShortcut(c, ui.Cmd, ui.KeyDown, open)
		h.OnShortcut(c, ui.Shift, ui.KeyF6, chosen(func(p string) {
			if p != a.root {
				a.beginRename(p)
			}
		}))
		a.treeShortcuts(c)
	}
}

// clickTreeFile is a single click on a file of the tree: it chooses the
// row, and opens the file in the preview tab or for good when the
// settings say so, keeping the keyboard in the tree, as IntelliJ's.
func (a *App) clickTreeFile(p string) {
	switch {
	case a.cfg.PreviewTab:
		a.previewOpen(p)
	case a.cfg.OpenSingleClick:
		a.openFromTree(p)
	default:
		return
	}
	a.focusEditor = nil
}

// The chosen row is already visible, so opening it need not reposition the tree.
func (a *App) openFromTree(path string) {
	a.userOpen(path)
	if a.focus.preview == a.focus.active {
		a.focus.preview = ""
	}
	for i := range a.tree.Rows() {
		if a.tree.Item(i) == path {
			a.setTreeRow(i)
			break
		}
	}
	a.reveal = ""
}

// isTreeDir reports whether a row is a folder, in the tree or in a
// filter's tree.
func (a *App) isTreeDir(p string) bool {
	if a.filtering() {
		return a.filter.tree[p] != nil
	}
	if p == a.root {
		return true
	}
	e, ok := a.entry(p)
	return ok && e.Dir
}

func (a *App) toggleDir(p string) {
	if a.tree.Open.Has(p) {
		a.tree.Open.Remove(p)
	} else {
		a.tree.Open.Add(p)
	}
}

func (a *App) treeItem(c *ui.Context, p string) {
	switch {
	case strings.HasSuffix(p, entrySuffix):
		if a.createEntry != nil && p == a.createEntry.dir+entrySuffix {
			a.entryPanel(c)
		}
		return
	case strings.HasSuffix(p, loadingSuffix):
		ui.Text(c, locales.MsgLoading()).TextColor(theme.Of(c).Muted).FontSize(widget.FontSize(c, 12))
		return
	case strings.HasSuffix(p, errorSuffix):
		dir := strings.TrimSuffix(p, errorSuffix)
		msg := "Could not read this folder"
		if d := a.dirs[dir]; d != nil && d.err != nil {
			msg += ": " + describeErr(d.err)
		}
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			ui.Text(c, msg).TextColor(theme.Of(c).Err).FontSize(widget.FontSize(c, 12)).SingleLine().Shrink(1)
			if ui.Button(c, locales.Retry()).Padding(0, 6).Clicked() {
				a.retryDir(dir)
			}
		})
		return
	}
	if p == a.root {
		a.treeRootItem(c)
		return
	}
	e, _ := a.entry(p)
	if a.filtering() {
		e = workspace.Entry{Name: filepath.Base(p), Path: p, Dir: a.filter.tree[p] != nil}
	}
	name := e.Name
	if name == "" {
		name = filepath.Base(p)
	}
	_, status := a.treeGitStatus(p)
	// Extend the hit area into the outline's row padding and the
	// sidebar's inset, to the sidebar's edge as AppKit's rows, keeping
	// the disclosure arrow as a separate target.
	row := ui.Row(c).Grow(1).AlignItems(ui.Center).Gap(6).Margin(-3, 0).MarginEnd(-(6 + sidebarInset)).Padding(3, 0)
	row.Children(func() {
		if e.Dir {
			glyph.DrawFolder(c, 16)
		} else {
			glyph.File(c, name, 16)
		}
		if a.renameEntry != nil && a.renameEntry.path == p {
			a.renameField(c, a.renameEntry)
			return
		}
		t := a.filter.speed.filename(c, name).SingleLine().Shrink(1)
		if status == 'I' {
			t.TextColor(theme.Of(c).Dim)
		} else if !e.Dir && a.cfg.GitStatus && status != 0 {
			t.TextColor(theme.Of(c).GitFileColor(status))
		}
		if e.Link {
			t.Italic()
			glyph.Draw(c, glyph.Link, 11, theme.Of(c).Dim)
		}
		if d := a.docs[p]; d != nil && d.gone {
			t.Strikethrough()
		}
	})
	services := c.Services()
	row.ContextMenu(func(m *ui.Menu) { a.treeMenu(m, p, e.Dir, services) })
	if e.Dir {
		if paths := row.DroppedFiles(); paths != nil {
			a.copyInto(p, paths)
		}
		if v, ok := ui.Drop[*tabDrag](row); ok && v.from == nil && v.change == nil {
			// A drag inside the project view moves, as IntelliJ's.
			a.moveInto(p, a.treeTargets(v.path))
		}
		_, treeOver := ui.DragOver[*tabDrag](row)
		if row.FileDragOver() || treeOver {
			row.Background(theme.Of(c).DropZone)
		}
		// A click chooses a folder and a double click opens it; its arrow
		// opens it at once.
		if row.DoubleClicked() {
			a.toggleDir(p)
		}
	} else if row.DoubleClicked() {
		a.openFromTree(p)
	}
	if row.Clicked() {
		mods := row.ClickModifiers()
		a.clickTreeRow(p, mods)
		if !e.Dir && mods&(ui.Cmd|ui.Shift) == 0 {
			a.clickTreeFile(p)
		}
		a.tree.List.Handle.Focus(c)
	}
	row.Drag(&tabDrag{path: p})
}

// treeRootItem is the row of the project's folder: its name, and where it
// is in a quieter color, as IntelliJ's root node. Its menu is the
// project's, without Rename or Delete.
func (a *App) treeRootItem(c *ui.Context) {
	p := a.root
	row := ui.Row(c).Grow(1).AlignItems(ui.Center).Gap(6).Margin(-3, 0).MarginEnd(-(6 + sidebarInset)).Padding(3, 0)
	row.Children(func() {
		glyph.DrawFolder(c, 16)
		ui.Text(c, filepath.Base(p)).Bold().SingleLine().Shrink(0)
		ui.Text(c, tildePath(p)).TextColor(theme.Of(c).Muted).SingleLine().Ellipsis("…").MinWidth(0).Shrink(1)
	})
	row.Tooltip(p)
	services := c.Services()
	row.ContextMenu(func(m *ui.Menu) { a.projectRootMenu(m, services) })
	if paths := row.DroppedFiles(); paths != nil {
		a.copyInto(p, paths)
	}
	if v, ok := ui.Drop[*tabDrag](row); ok && v.from == nil && v.change == nil {
		a.moveInto(p, a.treeTargets(v.path))
	}
	if _, over := ui.DragOver[*tabDrag](row); over || row.FileDragOver() {
		row.Background(theme.Of(c).DropZone)
	}
	if row.DoubleClicked() {
		a.toggleDir(p)
	}
	if row.Clicked() {
		a.clickTreeRow(p, row.ClickModifiers())
		a.tree.List.Handle.Focus(c)
	}
}
