// INPUT: gitState 的改动列表与勾选状态。
// OUTPUT: 提交纳入勾选框、计数与右键菜单。
// POS: Commit 工具窗口的「包含到提交」逻辑。
// PROTOCOL: 变更时更新此头部，然后检查本包 doc.go

package app

import (
	"path/filepath"
	"slices"

	"github.com/egoist/mygo/ui"

	"revtrail/internal/git"
	"revtrail/internal/theme"
	"revtrail/locales"
)

// changeID names a change across repositories: its absolute path.
func changeID(ch git.Change) string {
	return filepath.Join(ch.Root, filepath.FromSlash(ch.Path))
}

func (g *gitState) isIncluded(ch git.Change) bool {
	if ch.Status() == '!' {
		return false
	}
	if value, ok := g.included[changeID(ch)]; ok {
		return value
	}
	return ch.Index != '?'
}

func (g *gitState) includedChanges() []git.Change {
	var included []git.Change
	for _, ch := range g.changes {
		if g.isIncluded(ch) {
			ch.Blocks = g.blocks[changeID(ch)]
			included = append(included, ch)
		}
	}
	return included
}

func (g *gitState) syncInclusion(changes []git.Change) {
	if len(g.included) == 0 {
		return
	}
	current := make(map[string]bool)
	for name := range g.blocks {
		if !slices.ContainsFunc(changes, func(ch git.Change) bool { return changeID(ch) == name && ch.Status() != '!' }) {
			delete(g.blocks, name)
		}
	}
	for _, ch := range changes {
		if v, ok := g.included[changeID(ch)]; ok {
			current[changeID(ch)] = v
		}
	}
	g.included = current
}

func (g *gitState) inclusionCounts(rows []git.Change) (included, eligible int) {
	for _, ch := range rows {
		if ch.Status() == '!' {
			continue
		}
		eligible++

		if g.isIncluded(ch) && g.blocks[changeID(ch)].Mask == "" {
			included++
		}
	}
	return
}

func (a *App) toggleIncluded(rows []git.Change) {
	g := &a.git
	n, total := g.inclusionCounts(rows)
	if g.included == nil {
		g.included = make(map[string]bool)
	}
	for _, ch := range rows {
		if ch.Status() != '!' {
			delete(g.blocks, changeID(ch))
			g.included[changeID(ch)] = n < total
		}
	}
	a.saveConfigSoon()
}

func (a *App) inclusionCheckbox(c *ui.Context, key, label string, rows []git.Change) {
	n, total := a.git.inclusionCounts(rows)
	all := total > 0 && n == total
	b := ui.Checkbox(c, &all, "").Label(locales.CommitInclude() + " " + label).Disabled(a.git.busy || total == 0)
	if n > 0 && n < total || slices.ContainsFunc(rows, func(ch git.Change) bool { return a.git.blocks[changeID(ch)].Mask != "" }) {
		color := theme.Of(c).Text
		b.Mixed().DrawOver(func(p *ui.Painter, r ui.Rect) {
			p.Fill(ui.Rect{X: r.X + (r.W-7)/2, Y: r.Y + (r.H-2)/2, W: 7, H: 2}, color, 1)
		})
	}
	b.OnClick(func() { a.git.selectChange(key, 0); a.git.treeHandle.Focus(c); a.toggleIncluded(rows) })
}

func (a *App) inclusionMenu(m *ui.Menu) {
	rows := a.git.selectedFiles()
	n, total := a.git.inclusionCounts(rows)
	label := locales.CommitInclude()
	if n == total {
		label = locales.CommitExclude()
	}
	if m.Item(label).Disabled(a.git.busy || total == 0).Chosen() {
		a.toggleIncluded(rows)
	}
}

// A path removed from the index can also be reported as untracked at the same name.
func commitListChanges(changes []git.Change) []git.Change {
	paths := make(map[string]int, len(changes))
	rows := make([]git.Change, 0, len(changes))
	for _, ch := range changes {
		if i, exists := paths[ch.Path]; exists {
			rows[i] = git.Change{Root: ch.Root, Path: ch.Path, Index: 'D', Work: 'M'}
			continue
		}
		paths[ch.Path] = len(rows)
		rows = append(rows, ch)
	}
	return rows
}
