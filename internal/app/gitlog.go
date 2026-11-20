// INPUT: 提交列表与列宽。
// OUTPUT: 日志表格的列定义与行绘制（按 IntelliJ：无表头、24pt 行、圆角选中、合并提交置灰）；分支 / 标签按 IntelliJ 的标签图标与灰字靠右画在提交信息列内（refLabels、refLabel）。
// POS: 日志表格的公共绘制件。
// PROTOCOL: 变更时更新此头部，然后检查本包 doc.go

package app

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"

	"revtrail/internal/git"
	"revtrail/internal/glyph"
	"revtrail/internal/theme"
	"revtrail/locales"
)

// The Git log as IntelliJ's: one log component, in the Git tool window
// or in an editor tab, which opens with the tool window's filters and
// then keeps its own. Its rows are a table: the graph, the subject with
// the branches and tags at its right end, who made the commit, when, and
// its hash.

// gitLogTab is the log's tab. It is not kept between launches.
const gitLogTab = diffTabPrefix + "log"

// openGitLog shows the log in an editor tab, in the focused group, where
// it is already open if it is.
func (a *App) openGitLog() {
	for _, g := range a.groups() {
		if g.has(gitLogTab) && g.diffs[gitLogTab] != nil {
			a.activateIn(g, gitLogTab)
			return
		}
	}
	g := a.focus
	if g.diffs == nil {
		g.diffs = map[string]*gitDiffTab{}
	}
	w := &a.git.logWindow
	g.diffs[gitLogTab] = &gitDiffTab{name: "Git Log", log: &gitLogWindow{filter: w.filter, scope: w.scope}}
	g.addTab(gitLogTab, len(g.order))
	a.activateIn(g, gitLogTab)
	a.term.maximized = false
}

// logRowHeight is IntelliJ's (Table.rowHeight).
const logRowHeight = 24

type logTableColumns struct{ graph, author, date, hash float32 }

func logColumns(width float32, lanes int) logTableColumns {
	columns := logTableColumns{graph: max(62, graphX(max(lanes, 1)-1)+graphLane), date: 92}
	if width >= 900 {
		columns.author = 140
	}
	if width >= 1100 {
		columns.hash = 72
	}
	return columns
}

func (columns logTableColumns) minimumWidth() float32 {
	count := 3
	if columns.author > 0 {
		count++
	}
	if columns.hash > 0 {
		count++
	}
	return columns.graph + columns.author + columns.date + columns.hash + 160 + 24 + float32(count-1)*12
}

func logText(key string) string { return locales.Catalog.Format(i18n.Locale(), key, nil) }

// logRow is the log's row of its i-th commit.
func (a *App) logRow(c *ui.Context, v *gitLogWindow, i int, columns logTableColumns, graphVisible bool) {
	cm := v.commits[i]
	// IntelliJ's table row: no hover, a selection with small rounded corners,
	// and a merge commit's text grayed.
	row := ui.Row(c.Key(cm.Root+cm.Hash)).FillWidth().Height(logRowHeight).Padding(0, 8).Gap(12).AlignItems(ui.Center).Radius(4).
		Label(cm.Subject).Tooltip(cm.Short + " " + cm.Subject)
	if i == v.selection {
		row.Background(theme.Of(c).ListSelectionInactive)
		if v.table.FocusWithin(c) {
			row.Background(theme.Of(c).ListSelection)
		}
	}
	text := theme.Of(c).Text
	if len(cm.Parents) > 1 {
		text = theme.Of(c).Muted
	}
	row.ContextMenu(func(m *ui.Menu) {
		a.commitContextMenu(c, m, cm)
	})
	row.Children(func() {
		if len(a.git.roots) > 1 {
			// IntelliJ's root color: a stripe at the row's left edge.
			ui.Box(c).Size(3, 16).Radius(1.5).Absolute().Left(2).Top((logRowHeight - 16) / 2).Background(repoColor(slices.Index(a.git.roots, cm.Root))).Tooltip(a.repoLabel(cm.Root))
		}
		graph := ui.Box(c).Size(columns.graph, logRowHeight).Shrink(0)
		if graphVisible {
			graph.Label(logText("gitLogGraph") + " " + cm.Short).Draw(func(p *ui.Painter, r ui.Rect) { drawGraphRowLanes(p, r, cm, max(v.lanesW, 1)) })
			// Decorated tips carry author identity; ordinary commits keep compact dots.
			if len(cm.Refs) > 0 {
				graph.Children(func() {
					ui.Box(c).Size(18, 18).Absolute().Left(graphX(cm.Lane)-9).Top((logRowHeight-18)/2).
						Radius(9).Border(1.5, laneColor(cm.Lane)).Background(theme.Of(c).Sidebar).Center().Children(func() { a.personAvatar(c, cm.Author, cm.Email, 14) })
				})
			}
		}
		ui.Row(c).Grow(1).Shrink(1).Basis(0).Gap(5).AlignItems(ui.Center).Children(func() {
			ui.Text(c, cm.Subject).TextColor(text).SingleLine().Ellipsis("…").Shrink(1)
			if v.outgoing[cm.Hash] {
				ui.Text(c, "↑").Bold().TextColor(c.Theme().Accent).Shrink(0).Tooltip(locales.MsgNotPushedYet())
			}
			if v.incoming[cm.Hash] {
				ui.Text(c, "↓").Bold().TextColor(c.Theme().Accent).Shrink(0).Tooltip(logText("gitIncomingCommit"))
			}
			labels := refLabels(cm.Refs)
			if len(labels) == 0 {
				return
			}
			ui.Box(c).Grow(1).MinWidth(8)
			current := cm.Root == "" || cm.Root == a.git.root
			ui.Row(c).Shrink(0).MaxWidth(320).Gap(8).AlignItems(ui.Center).Tooltip(strings.Join(labels, "\n")).Children(func() {
				for _, l := range labels[:min(2, len(labels))] {
					a.refLabel(c.Key(l), cm, l, current)
				}
				if len(labels) > 2 {
					ui.Text(c, fmt.Sprintf("+%d", len(labels)-2)).TextColor(theme.Of(c).Muted).Shrink(0)
				}
			})
		})
		if columns.author > 0 {
			ui.Text(c, authors(cm)).Width(columns.author).Shrink(0).TextColor(text).SingleLine().Tooltip(authors(cm))
		}
		if columns.date > 0 {
			ui.Text(c, shortDate(cm.Date, time.Now())).Width(columns.date).Shrink(0).TextColor(text).SingleLine().Tooltip(cm.Date.Format(time.RFC1123))
		}
		if columns.hash > 0 {
			ui.Text(c, cm.Short).Width(columns.hash).Shrink(0).TextColor(theme.Of(c).Dim).SingleLine()
		}
	})
}

// refLabels are a commit's branch and tag names as IntelliJ's log labels
// them: HEAD and remote HEADs left out, and a local branch with the
// remote branch of the same name at the same commit shown once, as
// "origin & main".
func refLabels(refs []string) []string {
	var labels []string
	merged := map[string]bool{}
	for _, ref := range refs {
		if ref == "HEAD" || strings.HasSuffix(ref, "/HEAD") || merged[ref] {
			continue
		}
		label := ref
		for _, other := range refs {
			if remote, ok := strings.CutSuffix(other, "/"+ref); ok && remote != "" && !strings.Contains(remote, "/") {
				label = remote + " & " + ref
				merged[other] = true
				break
			}
		}
		labels = append(labels, label)
	}
	return labels
}

// refLabel draws a label of refLabels as IntelliJ's log does: an icon for
// each name it stands for, colored by kind, then the label in gray.
func (a *App) refLabel(c *ui.Context, cm git.Commit, label string, current bool) {
	pal := theme.Of(c)
	ui.Row(c).Shrink(1).MinWidth(0).Gap(3).AlignItems(ui.Center).Children(func() {
		ui.Row(c).Shrink(0).Children(func() {
			for i, name := range refNames(label) {
				color := pal.RefLocal
				switch {
				case slices.Contains(cm.Tags, name):
					color = pal.RefTag
				case current && name == a.git.branch:
					color = pal.RefHead
				case a.isRemoteRef(name):
					color = pal.RefRemote
				}
				icon := glyph.Draw(c.Key(i), glyph.RefLabel, 14, color)
				if i > 0 {
					icon.Margin(0, 0, 0, -8)
				}
			}
		})
		ui.Text(c, label).TextColor(pal.Muted).SingleLine().Ellipsis("…").MaxWidth(200).Shrink(1)
	})
}

// isRemoteRef reports whether a ref is a remote branch, as origin/main.
func (a *App) isRemoteRef(name string) bool {
	remote, _, ok := strings.Cut(name, "/")
	return ok && (slices.Contains(a.git.remotes, remote) || len(a.git.remotes) == 0 && remote == "origin")
}

// refNames are the names a label of refLabels stands for.
func refNames(label string) []string {
	if remote, local, ok := strings.Cut(label, " & "); ok {
		return []string{local, remote + "/" + local}
	}
	return []string{label}
}
