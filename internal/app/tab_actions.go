// INPUT: 编辑器组与固定状态。
// OUTPUT: 批量关闭（其他 / 左侧 / 右侧 / 未修改 / 全部 / 未固定）、固定与标签右键菜单。
// POS: IntelliJ 标签菜单的实现。
// PROTOCOL: 变更时更新此头部，然后检查本包 doc.go

package app

import (
	"slices"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"revtrail/internal/workspace"
	"revtrail/locales"
)

type tabCloseMode uint8

const (
	closeOtherTabs tabCloseMode = iota
	closeRightTabs
	closeLeftTabs
	closeSavedTabs
	closeAllTabs
	closeUnpinnedTabs
)

// togglePin pins a tab or unpins it, as IntelliJ's Pin Tab: the pinned
// tabs stay at the left, a bulk close but Close All Tabs keeps them, and
// a pinned tab is no preview.
func (a *App) togglePin(g *group, tab string) {
	if !g.has(tab) {
		return
	}
	if g.pinned[tab] {
		delete(g.pinned, tab)
	} else {
		if g.pinned == nil {
			g.pinned = map[string]bool{}
		}
		g.pinned[tab] = true
		if g.preview == tab {
			g.preview = ""
		}
	}
	at := 0
	for _, t := range g.order {
		if t != tab && g.pinned[t] {
			at++
		}
	}
	g.addTab(tab, at)
	a.saveConfigSoon()
}

func (a *App) closeTabs(g *group, anchor string, mode tabCloseMode) {
	at := slices.Index(g.order, anchor)
	if at < 0 {
		return
	}
	var candidates []string
	for i, tab := range g.order {
		if g.pinned[tab] && mode != closeAllTabs {
			continue
		}
		switch mode {
		case closeOtherTabs:
			if tab == anchor {
				continue
			}
		case closeRightTabs:
			if i <= at {
				continue
			}
		case closeLeftTabs:
			if i >= at {
				continue
			}
		case closeSavedTabs:
			if d := a.docs[tab]; d != nil && d.buf != nil && d.buf.Modified() {
				continue
			}
		}
		candidates = append(candidates, tab)
	}
	a.closeTabSet(g, candidates)
}

// tabCloseTarget follows a document through Save As without capturing later tabs.
type tabCloseTarget struct {
	tab      string
	document *doc
	diff     *gitDiffTab
}

// closeTabSet keeps the selected documents fixed while confirmation is pending.
func (a *App) closeTabSet(g *group, candidates []string) {
	targets := make([]tabCloseTarget, 0, len(candidates))
	for _, tab := range candidates {
		if !g.has(tab) {
			continue
		}
		targets = append(targets, tabCloseTarget{tab: tab, document: a.docs[tab], diff: g.diffs[tab]})
	}
	a.closeTabTargets(g, targets, a.gen, map[*doc]uint64{})
}

func (a *App) closeTabTargets(g *group, targets []tabCloseTarget, generation int, approved map[*doc]uint64) {
	if a.gen != generation || !slices.Contains(a.groups(), g) {
		return
	}
	var candidates []string
	for _, target := range targets {
		tab := target.tab
		if target.document != nil {
			d := target.document
			if a.docs[d.path] != d {
				continue
			}
			tab = d.path
		} else if target.diff == nil || g.diffs[tab] != target.diff {
			continue
		}
		if g.has(tab) {
			candidates = append(candidates, tab)
		}
	}
	var dirty []*doc
	versions := map[*doc]uint64{}
	for _, tab := range candidates {
		d := a.docs[tab]
		if d == nil || d.buf == nil || !d.buf.Modified() {
			continue
		}
		if _, seen := versions[d]; seen {
			continue
		}
		if version, ok := approved[d]; ok && version == d.buf.Version() {
			continue
		}
		retained := false
		for _, other := range a.groups() {
			for _, open := range other.order {
				if !isDiffTab(open) && open == d.path && (other != g || !slices.Contains(candidates, open)) {
					retained = true
				}
			}
		}
		if !retained {
			dirty = append(dirty, d)
			versions[d] = d.buf.Version()
		}
	}
	if a.guardClose(dirty, func() {
		// Discard approves only the versions shown when this question was asked.
		if a.allowDirty {
			for d, version := range versions {
				approved[d] = version
			}
		}
		inherited := a.allowDirty
		a.allowDirty = false
		defer func() { a.allowDirty = inherited }()
		a.closeTabTargets(g, targets, generation, approved)
	}) {
		return
	}
	for _, tab := range candidates {
		inherited := a.allowDirty
		d := a.docs[tab]
		version, ok := approved[d]
		a.allowDirty = ok && d != nil && d.buf != nil && version == d.buf.Version()
		a.closeTab(g, tab)
		a.allowDirty = inherited
	}
}

// tabContextMenu is a tab's menu, in the order of IntelliJ's.
func (a *App) tabContextMenu(m *ui.Menu, g *group, i int, tab string) {
	anyPinned := len(g.pinned) > 0
	if m.Item(commandLabel("Close Tab")).Chosen() {
		a.closeTab(g, tab)
	}
	if m.Item(commandLabel("Close Other Tabs")).Disabled(len(g.order) < 2).Chosen() {
		a.closeTabs(g, tab, closeOtherTabs)
	}
	if m.Item(commandLabel("Close All Tabs")).Chosen() {
		a.closeTabs(g, tab, closeAllTabs)
	}
	if m.Item(commandLabel("Close Unmodified Tabs")).Chosen() {
		a.closeTabs(g, tab, closeSavedTabs)
	}
	if anyPinned && m.Item(commandLabel("Close All but Pinned")).Chosen() {
		a.closeTabs(g, tab, closeUnpinnedTabs)
	}
	if m.Item(commandLabel("Close Tabs to the Left")).Disabled(i == 0).Chosen() {
		a.closeTabs(g, tab, closeLeftTabs)
	}
	if m.Item(commandLabel("Close Tabs to the Right")).Disabled(i == len(g.order)-1).Chosen() {
		a.closeTabs(g, tab, closeRightTabs)
	}
	m.Separator()
	pin := locales.TabPin()
	if g.pinned[tab] {
		pin = locales.TabUnpin()
	}
	if m.Item(pin).Chosen() {
		a.togglePin(g, tab)
	}
	if g.preview == tab && m.Item(commandLabel("Keep Preview Tab")).Chosen() {
		g.preview = ""
	}
	m.Separator()
	split := func(s side) {
		a.activateIn(g, tab)
		a.splitActive(s)
	}
	if m.Item(locales.TabSplitRight()).Chosen() {
		split(sideRight)
	}
	if m.Item(locales.TabSplitDown()).Chosen() {
		split(sideBottom)
	}
	if len(a.groups()) > 1 && m.Item(commandLabel("Move Tab to Next Editor")).Chosen() {
		a.activateIn(g, tab)
		a.moveTabToNextEditor()
	}
	f := a.tabFile(g, tab)
	if f == "" {
		return
	}
	m.Separator()
	a.copyReferenceMenu(m, []string{f})
	if workspace.Within(a.root, f) && m.Item(locales.TabSelectInProject()).Chosen() {
		a.sidebarOff, a.navigator = false, navFiles
		a.revealInTree(f)
	}
	if m.Item(locales.TabRevealInFinder()).Chosen() {
		mygo.Shell.ShowItemInFolder(f)
	}
}
