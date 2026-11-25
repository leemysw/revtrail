// INPUT: 文档与布局树。
// OUTPUT: 编辑器组、标签增删、激活、固定、预览标签与最近文件记录。
// POS: 标签与分屏的数据模型。
// PROTOCOL: 变更时更新此头部，然后检查本包 doc.go

package app

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"revtrail/internal/lsp"
	"revtrail/internal/workspace"
)

// group is an editor area with tabs of its own, as Xcode's editors
// side by side: each shows its documents in editors of its own, which
// keep their own place, over the buffers all groups share.
type group struct {
	lastTabStripWidth    float32
	lastShownTab         string
	id                   int
	order                []string // the tabs, left to right
	mru                  []string // most recently active last
	active               string
	preview              string
	pinned               map[string]bool
	eds                  map[string]*editor.State
	pending              map[string]goTo // where to go once a document loads
	markdownModes        map[string]markdownMode
	markdownSplitPercent float32
	hexPreview           map[string]bool
	hexLists             map[string]*ui.ListState
	hexControls          map[string]*hexControl
	diffs                map[string]*gitDiffTab
	tabSel               int
	focusHandle          ui.Handle
	relatedHandle        ui.Handle
	relatedBounds        ui.Rect
	pointerAnchor        ui.Point
	pointerInEditor      bool
	// changeBars are the Git change bars the editor shows, in the window,
	// and changeBarPress is whether a press on one is being taken.
	changeBars     []changeBar
	changeBarPress bool
	caretAnchor    ui.Point
}

// goTo is a place to show in a document: a position of the buffer, or
// a server's, converted once the text is there, and how many bytes to
// select from it.
type goTo struct {
	pos    *editor.Pos
	lsp    *lsp.Position
	enc    string
	selLen int
}

func (g *group) has(path string) bool { return slices.Contains(g.order, path) }

const diffTabPrefix = "\x00diff:"

func isDiffTab(tab string) bool { return strings.HasPrefix(tab, diffTabPrefix) }
func diffTab(path string, staged bool) string {
	return fmt.Sprintf("%s%t:%s", diffTabPrefix, staged, path)
}

// Keep the original tab IDs for an enclosing repository and distinguish nested worktrees.
func (a *App) repoTabKey(root, key string) string {
	if root != "" && root != a.realRoot && workspace.Within(a.realRoot, root) {
		return key + "\x00" + root
	}
	return key
}

// tabFile is the file of the workspace a tab of g shows, which the tree
// chooses: a diff's too, of the working tree, the index or a commit; ""
// for a commit's own tab.
func (a *App) tabFile(g *group, tab string) string {
	if tab == "" {
		return ""
	}
	if !isDiffTab(tab) {
		return tab
	}
	v := g.diffs[tab]
	if v == nil || v.page() || v.root == "" {
		return ""
	}
	return workspace.DocPath(a.root, a.realRoot, filepath.Join(v.root, filepath.FromSlash(v.name)))
}

// side is where a new group goes beside another.
type side int

const (
	sideNone side = iota // in the group itself
	sideLeft
	sideRight
	sideTop
	sideBottom
)

// layout is the tree the editor area is split into: a leaf shows a
// group; a split shows a and b side by side, or a above b, a taking pct
// of the room.
type layout struct {
	g        *group
	vertical bool
	pct      float32
	a, b     *layout
	parent   *layout
}

func (n *layout) leaves(out []*group) []*group {
	if n == nil {
		return out
	}
	if n.g != nil {
		return append(out, n.g)
	}
	return n.b.leaves(n.a.leaves(out))
}

func (n *layout) find(g *group) *layout {
	if n == nil {
		return nil
	}
	if n.g == g {
		return n
	}
	if f := n.a.find(g); f != nil {
		return f
	}
	return n.b.find(g)
}

// groups lists the groups, left to right and top to bottom.
func (a *App) groups() []*group { return a.layout.leaves(nil) }

func (a *App) newGroup() *group {
	a.nextGroup++
	return &group{id: a.nextGroup, markdownModes: map[string]markdownMode{}, markdownSplitPercent: 50, eds: map[string]*editor.State{}, pending: map[string]goTo{}, tabSel: -1}
}

// resetLayout leaves one empty group.
func (a *App) resetLayout() {
	g := a.newGroup()
	a.layout = &layout{g: g}
	a.focus = g
}

// splitBeside puts a new, empty group at s of g.
func (a *App) splitBeside(g *group, s side) *group {
	leaf := a.layout.find(g)
	if leaf == nil || s == sideNone {
		return g
	}
	ng := a.newGroup()
	old := &layout{g: g, parent: leaf}
	nw := &layout{g: ng, parent: leaf}
	leaf.g = nil
	leaf.vertical = s == sideTop || s == sideBottom
	leaf.pct = 50
	if s == sideLeft || s == sideTop {
		leaf.a, leaf.b = nw, old
	} else {
		leaf.a, leaf.b = old, nw
	}
	return ng
}

// removeGroup takes an empty group out of the layout: its sibling takes
// its room. The last group stays.
func (a *App) removeGroup(g *group) {
	leaf := a.layout.find(g)
	if leaf == nil || leaf.parent == nil {
		return
	}
	p := leaf.parent
	sib := p.a
	if sib == leaf {
		sib = p.b
	}
	// The parent becomes the sibling.
	p.g, p.vertical, p.pct, p.a, p.b = sib.g, sib.vertical, sib.pct, sib.a, sib.b
	if p.a != nil {
		p.a.parent, p.b.parent = p, p
	}
	if a.focus == g {
		a.focus = a.groups()[0]
		if gs := p.leaves(nil); len(gs) > 0 {
			a.focus = gs[0]
		}
	}
}

// addTab puts path in g at index at (its end when out of range), or
// moves it there if g has it.
func (g *group) addTab(path string, at int) {
	if i := slices.Index(g.order, path); i >= 0 {
		g.order = slices.Delete(g.order, i, i+1)
		if i < at {
			at--
		}
	}
	at = min(max(at, 0), len(g.order))
	g.order = slices.Insert(g.order, at, path)
}

// activateIn makes path the active tab of g, and g the focused group.
func (a *App) activateIn(g *group, path string) {
	if !g.has(path) {
		return
	}
	a.focus = g
	g.active = path
	g.mru = slices.DeleteFunc(g.mru, func(p string) bool { return p == path })
	g.mru = append(g.mru, path)
	if p := a.tabFile(g, path); p != "" {
		a.revealInTree(p)
		a.noteRecent(p)
	}
	a.saveConfigSoon()
}

// maxRecentFiles bounds the files Recent Files lists, as IntelliJ's
// Recent Files Limit.
const maxRecentFiles = 50

// noteRecent puts a file shown at the top of Recent Files.
func (a *App) noteRecent(p string) {
	if isDiffTab(p) {
		return
	}
	a.recent = slices.DeleteFunc(a.recent, func(r string) bool { return r == p })
	a.recent = append(a.recent, p)
	if len(a.recent) > maxRecentFiles {
		a.recent = slices.Delete(a.recent, 0, len(a.recent)-maxRecentFiles)
	}
}

// closeTab closes a tab of g. A document no group shows any more is let
// go; a group left without tabs goes, unless it is the last.
func (a *App) closeTab(g *group, path string) {
	if !g.has(path) {
		return
	}
	if isDiffTab(path) {
		delete(g.diffs, path)
	}
	if d := a.docs[path]; d != nil && d.buf != nil && d.buf.Modified() {
		last := true
		for _, other := range a.groups() {
			if other != g && other.has(path) {
				last = false
				break
			}
		}
		if last && a.guardClose([]*doc{d}, func() { a.closeTab(g, d.path) }) {
			return
		}
	}
	delete(g.pinned, path)
	if g.preview == path {
		g.preview = ""
	}
	g.order = slices.DeleteFunc(g.order, func(p string) bool { return p == path })
	g.mru = slices.DeleteFunc(g.mru, func(p string) bool { return p == path })
	delete(g.eds, path)
	delete(g.pending, path)
	delete(g.markdownModes, path)
	delete(g.hexPreview, path)
	delete(g.hexLists, path)
	if v := g.hexControls[path]; v != nil && v.cancel != nil {
		v.cancel()
	}
	delete(g.hexControls, path)
	if g.active == path {
		g.active = ""
		if n := len(g.mru); n > 0 {
			g.active = g.mru[n-1]
		}
	}
	a.releaseIfUnused(path)
	if a.root == "" && len(a.docs) == 0 {
		a.host.SetTitle("Revtrail")
		a.host.SetWelcomeSize()
	}
	if len(g.order) == 0 && len(a.groups()) > 1 {
		a.removeGroup(g)
	}
	if p := a.tabFile(g, g.active); a.focus == g && p != "" {
		a.revealInTree(p)
	}
	a.saveConfigSoon()
}

// releaseIfUnused lets a document go once no group shows it.
func (a *App) releaseIfUnused(path string) {
	for _, g := range a.groups() {
		if g.has(path) {
			return
		}
	}
	d := a.docs[path]
	if d == nil {
		return
	}
	delete(a.docs, path)
	if d.untitled {
		removeDraft(d)
	} else {
		a.removeRecovery(d)
	}
	d.seq++ // drop a load on its way
	d.closeHTMLPreview()
	if m := a.serverFor(d.path); m != nil {
		m.Close(d.uri)
	}
}

// moveTab moves a tab from one group to another, at index at, or into a
// new group at s of to. Its editor goes with it, keeping its place.
func (a *App) moveTab(from *group, path string, to *group, s side, at int) {
	if from != nil && from.preview == path {
		from.preview = ""
	}
	if !from.has(path) {
		return
	}
	if s != sideNone {
		if from == to && len(from.order) == 1 {
			return // splitting a group's only tab off itself changes nothing
		}
		to = a.splitBeside(to, s)
		at = 0
	}
	if from == to {
		from.addTab(path, at)
		a.activateIn(to, path)
		return
	}
	if isDiffTab(path) {
		if to.diffs == nil {
			to.diffs = map[string]*gitDiffTab{}
		}
		if !to.has(path) {
			to.diffs[path] = from.diffs[path]
		}
		to.addTab(path, at)
		delete(from.diffs, path)
		from.order = slices.DeleteFunc(from.order, func(p string) bool { return p == path })
		from.mru = slices.DeleteFunc(from.mru, func(p string) bool { return p == path })
		if from.active == path {
			from.active = ""
			if n := len(from.mru); n > 0 {
				from.active = from.mru[n-1]
			}
		}
		if len(from.order) == 0 && len(a.groups()) > 1 {
			a.removeGroup(from)
		}
		a.activateIn(to, path)
		return
	}
	ed := from.eds[path]
	previewMarkdown := from.markdownModes[path]
	previewHex := from.hexPreview[path]
	hexList := from.hexLists[path]
	hexCtl := from.hexControls[path]
	alreadyOpen := to.has(path)
	pend, hasPend := from.pending[path]
	if to.has(path) {
		ed = to.eds[path] // it shows there already: keep that one
	}
	to.addTab(path, at)
	if !alreadyOpen {
		to.markdownModes[path] = previewMarkdown
	}
	if !alreadyOpen && previewHex {
		if to.hexPreview == nil {
			to.hexPreview = map[string]bool{}
		}
		to.hexPreview[path] = true
	}
	if !alreadyOpen && hexList != nil {
		if to.hexLists == nil {
			to.hexLists = map[string]*ui.ListState{}
		}
		to.hexLists[path] = hexList
	}
	if !alreadyOpen && hexCtl != nil {
		if to.hexControls == nil {
			to.hexControls = map[string]*hexControl{}
		}
		to.hexControls[path] = hexCtl
	}
	if ed != nil {
		to.eds[path] = ed
	}
	if hasPend {
		to.pending[path] = pend
	}
	// Close it in from without letting the document go.
	from.order = slices.DeleteFunc(from.order, func(p string) bool { return p == path })
	from.mru = slices.DeleteFunc(from.mru, func(p string) bool { return p == path })
	delete(from.eds, path)
	delete(from.pending, path)
	delete(from.markdownModes, path)
	delete(from.hexPreview, path)
	delete(from.hexLists, path)
	delete(from.hexControls, path)
	if from.active == path {
		from.active = ""
		if n := len(from.mru); n > 0 {
			from.active = from.mru[n-1]
		}
	}
	if len(from.order) == 0 && len(a.groups()) > 1 {
		a.removeGroup(from)
	}
	a.activateIn(to, path)
}

// splitActive shows the focused group's document in a new group at s
// too, at the same place, as Xcode's and VS Code's Split Editor.
func (a *App) splitActive(s side) {
	g := a.focus
	path := g.active
	ng := a.splitBeside(g, s)
	if path == "" {
		a.focus = ng
		a.saveConfigSoon()
		return
	}
	ng.addTab(path, 0)
	if isDiffTab(path) {
		if v := g.diffs[path]; v != nil && v.pending {
			ng.diffs = map[string]*gitDiffTab{path: v} // filled where it is when read
		} else if v != nil && v.log != nil {
			ng.diffs = map[string]*gitDiffTab{path: {name: v.name, log: &gitLogWindow{filter: v.log.filter, scope: v.log.scope}}}
		} else if v != nil && v.commit != nil {
			cv := *v.commit
			cv.scroll = ui.ScrollState{}
			ng.diffs = map[string]*gitDiffTab{path: {root: v.root, name: v.name, rev: v.rev, hash: v.hash, commit: &cv}}
		} else if v != nil {
			nv := a.newGitDiffTab(v.patch, v.name, v.staged)
			nv.root, nv.digest = v.root, v.digest
			nv.rev, nv.hash, nv.oldPath = v.rev, v.hash, v.oldPath
			ng.diffs = map[string]*gitDiffTab{path: nv}
		}
		a.activateIn(ng, path)
		return
	}
	ng.markdownModes[path] = g.markdownModes[path]
	if ed := g.eds[path]; ed != nil {
		pos := ed.CursorPos()
		ng.pending[path] = goTo{pos: &pos}
	}
	a.activateIn(ng, path)
	if d := a.docs[path]; d != nil {
		a.showLoaded(ng, d)
	}
}

// focusGroup focuses the n-th group, from 0.
func (a *App) focusGroup(n int) {
	gs := a.groups()
	if n >= 0 && n < len(gs) {
		a.requestGroupFocus(gs[n])
		a.saveConfigSoon()
	}
}
