// INPUT: gitState 的改动与展开状态。
// OUTPUT: 改动树（按目录 / 分组）的行、选择与快捷键。
// POS: Commit 工具窗口的文件树。
// PROTOCOL: 变更时更新此头部，然后检查本包 doc.go

package app

import (
	"path"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"

	"revtrail/internal/git"
	"revtrail/internal/glyph"
	"revtrail/internal/theme"
	"revtrail/internal/widget"
	"revtrail/locales"
)

type changeTreeExpansion uint8

const (
	changesDefault changeTreeExpansion = iota
	changesExpanded
	changesCollapsed
)

// commitFilesSection draws one section of the changes, as IntelliJ's
// changelists: with more than one repository, a node for each holds its
// changes, as IntelliJ's Group By Repository.
func (a *App) commitFilesSection(c *ui.Context, title string, changes []git.Change) {
	if len(changes) == 0 || !a.changeGroup(c, title, title, "", "", changes, 0) {
		return
	}
	if len(a.git.roots) < 2 {
		a.changeFiles(c, title, changes, 1)
		return
	}
	var roots []string
	for _, ch := range changes {
		if !slices.Contains(roots, ch.Root) {
			roots = append(roots, ch.Root)
		}
	}
	for _, root := range roots {
		rows := slices.DeleteFunc(slices.Clone(changes), func(ch git.Change) bool { return ch.Root != root })
		key := title + "\x00" + root
		if a.changeGroup(c, key, a.repoLabel(root), "", title, rows, 1) {
			a.changeFiles(c, key, rows, 2)
		}
	}
}

func (a *App) changeFiles(c *ui.Context, section string, changes []git.Change, depth int) {
	if !a.git.groupByDirectory {
		for _, ch := range changes {
			a.gitRow(c, ch, depth, section)
		}
		return
	}
	slices.SortFunc(changes, func(l, r git.Change) int { return strings.Compare(l.Path, r.Path) })
	a.changeDirectory(c, section, "", changes, depth)
}

// Git paths use slashes on every platform. Sorting makes each folder's
// descendants a contiguous slice for drawing and group inclusion.
func (a *App) changeDirectory(c *ui.Context, section, prefix string, rows []git.Change, depth int) {
	parent := section
	if prefix != "" {
		parent = section + "/" + strings.TrimSuffix(prefix, "/")
	}
	for i := 0; i < len(rows); {
		part, _, folder := strings.Cut(strings.TrimSuffix(rows[i].Path[len(prefix):], "/"), "/")
		if !folder {
			i++
			continue
		}
		base := prefix + part + "/"
		end := i + 1
		for end < len(rows) && strings.HasPrefix(rows[end].Path, base) {
			end++
		}

		// Fold a single-child directory chain into one label. A direct
		// file or a second child ends the shared directory prefix.
		dir := path.Dir(strings.TrimSuffix(rows[i].Path, "/"))
		for !strings.HasPrefix(rows[end-1].Path, dir+"/") {
			dir = path.Dir(dir)
		}
		children := rows[i:end]
		if a.changeGroup(c, section+"/"+dir, strings.TrimPrefix(dir, prefix), dir, parent, children, depth) {
			a.changeDirectory(c, section, dir+"/", children, depth+1)
		}
		i = end
	}
	for _, ch := range rows {
		if !strings.Contains(strings.TrimSuffix(ch.Path[len(prefix):], "/"), "/") {
			a.gitRow(c, ch, depth, parent)
		}
	}
}

func (a *App) changeGroup(c *ui.Context, key, name, dir, parent string, rows []git.Change, depth int) bool {
	// A repository's node sits between a section and its folders.
	repo := depth > 0 && dir == ""
	level := depth
	if len(a.git.roots) > 1 && !repo {
		level--
	}
	folded, exists := a.git.foldedChanges[key]
	if !exists {
		folded = a.git.changeTreeExpansion == changesCollapsed || a.git.changeTreeExpansion == changesDefault && level >= 2
	}
	root := ""
	if depth > 0 {
		root = rows[0].Root
	}
	toggle := func() {
		a.git.toggleFolder(key, !folded)
	}
	count := locales.MsgDFileS(len(rows))
	container := ui.Row(c.Key(key)).FillWidth().Radius(4).MinHeight(23).Padding(0, 8, 0, float32(sidebarInset+depth*16)).Gap(4).AlignItems(ui.Center)
	a.git.addTreeRow(key, changeTreeRow{group: true, dir: dir, root: root, files: rows, folded: folded, parent: parent})
	a.git.revealChange(key, container)
	if a.git.selected[key] {
		container.Background(a.git.selectionColor(c))
	} else if container.Hovered() {
		container.Background(theme.Of(c).Pill)
	}
	container.ContextMenu(func(m *ui.Menu) {
		a.git.selectContextRow(key)
		a.inclusionMenu(m)
		a.changeDeleteMenu(m)
	})
	container.Children(func() {
		arrow := ui.ButtonBase(c).Size(12, 20).Center().KeepFocus().Label(name)
		arrow.Children(func() {
			icon := glyph.Draw(c, glyph.Chevron, 9, theme.Of(c).Muted)
			if !folded {
				icon.Rotate(90)
			}
		})
		arrow.Label(name + " ▸")
		arrow.OnClick(toggle)
		a.inclusionCheckbox(c, key, name+" ("+count+")", rows)
		b := ui.ButtonBase(c).Grow(1).Shrink(1).KeepFocus()
		b.Children(func() {
			ui.Row(c).FillWidth().Gap(5).AlignItems(ui.Center).Children(func() {
				switch {
				case repo:
					ui.Box(c).Size(12, 12).Radius(3).Background(repoColor(slices.Index(a.git.roots, root)))
				case depth > 0:
					glyph.Draw(c, glyph.WelcomeFolder, 14, theme.Of(c).Muted)
				}
				label := ui.Text(c, name).SingleLine().Shrink(1)
				total := ui.Text(c, count).SingleLine().TextColor(theme.Of(c).Muted)
				if depth == 0 {
					label.FontSize(widget.FontSize(c, 12))
					total.FontSize(widget.FontSize(c, 11))
				}
			})
		})
		b.Label(name)
		if dir != "" {
			b.Label(dir)
		}
		if b.Clicked() {
			a.git.selectChange(key, b.ClickModifiers())
			a.git.treeHandle.Focus(c)
		}
		if b.DoubleClicked() {
			toggle()
		}
	})
	return !folded
}

type changeTreeRows struct {
	order []string
	byKey map[string]changeTreeRow
}

type changeTreeRow struct {
	group  bool
	parent string
	root   string // the repository of its files, "" for a section
	dir    string
	files  []git.Change
	folded bool
}

func (g *gitState) addTreeRow(key string, row changeTreeRow) {
	g.nextRows.order = append(g.nextRows.order, key)
	g.nextRows.byKey[key] = row
}

func (g *gitState) selectContextRow(key string) {
	if !g.selected[key] {
		g.selectChange(key, 0)
	}
}

func (g *gitState) selectedFiles() []git.Change {
	var files []git.Change
	seen := make(map[string]bool)
	for _, key := range g.rows.order {
		if !g.selected[key] {
			continue
		}
		row := g.rows.byKey[key]
		for _, ch := range row.files {
			if id := changeID(ch); !seen[id] {
				files = append(files, ch)
				seen[id] = true
			}
		}
	}
	return files
}

func (a *App) changeTreeShortcuts(tree ui.Element) {
	g := &a.git
	if !g.busy {
		tree.OnShortcut(0, ui.KeyDelete, a.deleteSelectedChanges)
		tree.OnShortcut(0, ui.KeyBackspace, a.deleteSelectedChanges)
		tree.OnShortcut(ui.Cmd, ui.KeyBackspace, a.deleteSelectedChanges)
		if len(g.deletions) > 0 {
			tree.OnShortcut(ui.Ctrl, ui.KeyZ, a.undoChangeDeletion)
			tree.OnShortcut(ui.Cmd, ui.KeyZ, a.undoChangeDeletion)
		}
	}
	selectAll := func() {
		g.selected = make(map[string]bool, len(g.rows.order))
		for _, key := range g.rows.order {
			g.selected[key] = true
		}
		if len(g.rows.order) > 0 {
			g.selectionAnchor = g.rows.order[0]
			g.treeCursor = g.rows.order[len(g.rows.order)-1]
		}
	}
	for _, mod := range []ui.Modifiers{ui.Cmd, ui.Ctrl} {
		tree.OnShortcut(mod, ui.KeyA, selectAll)
	}
	// macOS also delivers Edit menu shortcuts as responder commands.
	tree.HandleInput(func(event ui.InputEvent) bool {
		if event.Kind != ui.InputCommand {
			return false
		}
		switch event.Text {
		case "undo":
			a.undoChangeDeletion()
		case "delete":
			a.deleteSelectedChanges()
		case "selectAll":
			selectAll()
		default:
			return false
		}
		return true
	})
	for _, mods := range []ui.Modifiers{0, ui.Shift} {
		for _, key := range []ui.Key{ui.KeyUp, ui.KeyDown} {
			tree.OnShortcut(mods, key, func() {
				keys := g.rows.order
				if len(keys) == 0 {
					return
				}
				i := slices.Index(keys, g.treeCursor)
				if key == ui.KeyDown {
					i++
				} else {
					i--
				}
				next := keys[min(max(i, 0), len(keys)-1)]
				g.selectChange(next, mods)
				g.scrollToChange = next
			})
		}
	}
	tree.OnShortcut(0, ui.KeyEnter, func() {
		row, ok := g.rows.byKey[g.treeCursor]
		if !ok {
			return
		}
		if row.group {
			g.toggleFolder(g.treeCursor, !row.folded)
		} else {
			a.showGitDiff(row.files[0], false)
		}
	})
	for _, key := range []ui.Key{ui.KeyLeft, ui.KeyRight} {
		tree.OnShortcut(0, key, func() {
			row, ok := g.rows.byKey[g.treeCursor]
			if !ok {
				return
			}
			if row.group && row.folded == (key == ui.KeyRight) {
				g.toggleFolder(g.treeCursor, key == ui.KeyLeft)
				return
			}
			i := slices.Index(g.rows.order, g.treeCursor)
			if key == ui.KeyRight {
				if row.group && i+1 < len(g.rows.order) {
					g.selectChange(g.rows.order[i+1], 0)
					g.scrollToChange = g.treeCursor
				}
				return
			}
			if row.parent != "" {
				g.selectChange(row.parent, 0)
				g.scrollToChange = row.parent
			}
		})
	}
	tree.OnShortcut(0, ui.KeySpace, func() {
		if g.busy {
			return
		}
		if len(g.selected) == 0 {
			a.toggleIncluded(g.changes)
		} else {
			a.toggleIncluded(g.selectedFiles())
		}
	})
}

func (g *gitState) toggleFolder(key string, folded bool) {
	if g.foldedChanges == nil {
		g.foldedChanges = make(map[string]bool)
	}
	g.foldedChanges[key] = folded
}

func (g *gitState) revealChange(key string, row ui.Element) {
	if g.scrollToChange == key {
		row.ScrollIntoView()
		g.scrollToChange = ""
	}
}

func (a *App) changeDeleteMenu(m *ui.Menu) {
	if m.Item(locales.MoveToTrash()).Disabled(a.git.busy).Chosen() {
		a.deleteSelectedChanges()
	}
}
