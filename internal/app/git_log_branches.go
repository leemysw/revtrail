// INPUT: 日志窗口的引用列表、收藏与当前分支。
// OUTPUT: 日志的分支面板（按 IntelliJ）：左侧竖排操作条（新建、更新、删除、与当前比较、我的分支、获取、收藏、在日志中定位、分组选项、展开 / 折叠）与按目录分组的引用树；收起时为窄条。
// POS: IntelliJ Git Log 的 Branches 面板。
// PROTOCOL: 变更时更新此头部，然后检查本包 doc.go

package app

import (
	"fmt"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"

	"revtrail/internal/git"
	"revtrail/internal/glyph"
	"revtrail/internal/theme"
	"revtrail/internal/widget"
	"revtrail/locales"
)

// refSection is a top node of the branches tree.
type refSection struct {
	key, prefix string
	label       func() string
}

var refSections = []refSection{
	{"local", "refs/heads/", locales.GitWindowLocal},
	{"remote", "refs/remotes/", locales.GitWindowRemote},
	{"tags", "refs/tags/", locales.GitWindowTags},
}

// refRow is a row of the branches tree: HEAD, a section, a folder or a
// ref.
type refRow struct {
	key, name string // key: the folding key of a section or folder
	ref       string // a ref's full name; "HEAD" for HEAD
	depth     int
	folded    bool
	folder    bool // a folder, not a section
}

// refTree is IntelliJ's branches tree under one node: with grouped,
// names with slashes sit in folders; in each, favorites first, then
// folders, then the other refs, each by name.
func refTree(key, prefix string, names []string, grouped bool, favorite func(ref string) bool, folded func(key string) bool, depth int) []refRow {
	var rows []refRow
	type folder struct {
		name  string
		names []string
	}
	var folders []folder
	var favs, rest []string
	for _, n := range names {
		if dir, sub, ok := strings.Cut(n, "/"); ok && grouped {
			if i := slices.IndexFunc(folders, func(f folder) bool { return f.name == dir }); i >= 0 {
				folders[i].names = append(folders[i].names, sub)
			} else {
				folders = append(folders, folder{dir, []string{sub}})
			}
			continue
		}
		if favorite(prefix + n) {
			favs = append(favs, n)
		} else {
			rest = append(rest, n)
		}
	}
	slices.Sort(favs)
	slices.Sort(rest)
	slices.SortFunc(folders, func(l, r folder) int { return strings.Compare(l.name, r.name) })
	leaf := func(n string) refRow { return refRow{name: n, ref: prefix + n, depth: depth} }
	for _, n := range favs {
		rows = append(rows, leaf(n))
	}
	for _, f := range folders {
		k := key + "/" + f.name
		row := refRow{key: k, name: f.name, depth: depth, folded: folded(k), folder: true}
		rows = append(rows, row)
		if !row.folded {
			rows = append(rows, refTree(k, prefix+f.name+"/", f.names, grouped, favorite, folded, depth+1)...)
		}
	}
	for _, n := range rest {
		rows = append(rows, leaf(n))
	}
	return rows
}

// isFavorite reports whether a branch is a favorite: marked as one, or
// main or master, locally or on a remote, unless unmarked.
func (a *App) isFavorite(ref string) bool {
	if mark, ok := a.git.favorites[ref]; ok {
		return mark
	}
	name := ref[strings.LastIndex(ref, "/")+1:]
	return (name == "main" || name == "master") && (ref == "refs/heads/"+name || strings.HasPrefix(ref, "refs/remotes/") && strings.Count(ref, "/") == 3)
}

// selectedLogBranch is the branch the branches tree has selected, if it
// has a branch selected.
func (a *App) selectedLogBranch(v *gitLogWindow) (git.Branch, bool) {
	if name, ok := strings.CutPrefix(v.refSel, "refs/heads/"); ok {
		b := git.Branch{Name: name, Current: name == a.git.branch && a.git.onBranch()}
		if info, ok := v.branchInfo[v.refSel]; ok {
			b.Upstream = info.Upstream
		}
		return b, true
	}
	if name, ok := strings.CutPrefix(v.refSel, "refs/remotes/"); ok {
		return git.RemoteBranch(a.git.remotes, name), true
	}
	return git.Branch{}, false
}

// logBranchesStrip is IntelliJ's folded Branches pane: a narrow strip
// with the arrow that opens it.
func logBranchesStrip(c *ui.Context, v *gitLogWindow) {
	title := locales.GitWindowShowBranches()
	ui.Column(c).Width(logStripWidth).FillHeight().Shrink(0).AlignItems(ui.Center).BorderWidth(0, 1, 0, 0).BorderColor(theme.Of(c).Hairline).Children(func() {
		ui.Row(c).Height(editorTabBarHeight).AlignItems(ui.Center).Children(func() {
			toolWindowAction(c, glyph.Forward, title, title, 0, false).OnClick(func() { v.branchesHidden = false })
		})
	})
}

// logStripWidth is the width of the branches pane's action strip.
const logStripWidth = 32

// logBranchActions is the branches pane's vertical toolbar, as
// IntelliJ's: hide, then what acts on the selected branch, then the
// tree's options.
func (a *App) logBranchActions(c *ui.Context, v *gitLogWindow) {
	pal := theme.Of(c)
	busy := a.git.busy
	b, isBranch := a.selectedLogBranch(v)
	tag, isTag := strings.CutPrefix(v.refSel, "refs/tags/")
	ui.Column(c).Width(logStripWidth).FillHeight().Shrink(0).AlignItems(ui.Center).Gap(2).Children(func() {
		ui.Row(c).Height(editorTabBarHeight).AlignItems(ui.Center).Children(func() {
			hide := locales.GitWindowHideBranches()
			toolWindowAction(c, glyph.Back, hide, hide, 0, false).OnClick(func() { v.branchesHidden = true })
		})
		ui.Box(c).Size(16, 1).Background(pal.Separator).Margin(0, 0, 2, 0)
		from, start := a.git.branch, "HEAD"
		if isBranch || isTag {
			from, start = strings.TrimPrefix(strings.TrimPrefix(v.refSel, "refs/heads/"), "refs/"), v.refSel
			if isTag {
				from = tag
			}
		}
		if toolWindowAction(c, glyph.Plus, locales.GitLogNewBranch(), locales.GitLogNewBranch(), 0, busy).Clicked() {
			a.promptNewBranch(from, start)
		}
		update := isBranch && !b.Remote && b.Upstream != ""
		if toolWindowAction(c, glyph.Pull, locales.GitLogUpdateSelected(), locales.GitLogUpdateSelected(), 0, busy || !update).Clicked() {
			a.updateBranch(b)
		}
		deletable := isBranch && !b.Current || isTag
		if toolWindowAction(c, glyph.Delete, locales.GitDeleteAction(), locales.GitDeleteAction(), 0, busy || !deletable).Clicked() {
			if isTag {
				a.promptDeleteTag(tag)
			} else {
				a.deleteBranch(b)
			}
		}
		compare := isBranch && !b.Current && a.git.onBranch()
		if toolWindowAction(c, glyph.ShowDiff, locales.GitLogCompareCurrent(), locales.GitLogCompareCurrent(), 0, !compare).Clicked() {
			a.openCompareWith(b.Name)
		}
		mine := logToggle(c, glyph.Search, locales.GitLogMyBranches(), v.showMine, false)
		if mine.Clicked() {
			v.showMine = !v.showMine
			if v.showMine {
				a.loadMyBranches(v)
			}
		}
		if toolWindowAction(c, glyph.Fetch, locales.GitLogFetch(), locales.GitLogFetch(), 0, busy || len(a.git.remotes) == 0).Clicked() {
			a.gitFetch()
		}
		fav := isBranch && a.isFavorite(v.refSel)
		label := locales.GitLogMarkFavorite()
		if fav {
			label = locales.GitLogUnmarkFavorite()
		}
		if logToggle(c, glyph.Favorite, label, fav, !isBranch).Clicked() {
			if a.git.favorites == nil {
				a.git.favorites = map[string]bool{}
			}
			a.git.favorites[v.refSel] = !fav
			a.saveConfigSoon()
		}
		if toolWindowAction(c, glyph.LocateChange, locales.GitLogNavigate(), locales.GitLogNavigate(), 0, v.refTips[v.refSel] == "").Clicked() {
			hash := v.refTips[v.refSel]
			v.filter.Ref = ""
			a.seekCommit(v, hash)
		}
		ui.Box(c).Size(16, 1).Background(pal.Separator).Margin(2, 0)
		toolWindowAction(c, glyph.Gear, locales.GitLogBranchOptions(), locales.GitLogBranchOptions(), 0, false).Menu(func(m *ui.Menu) {
			if m.Item(locales.CommitGroupDirectory()).Checked(!v.flatRefs).Chosen() {
				v.flatRefs = !v.flatRefs
			}
		})
		if toolWindowAction(c, glyph.ExpandChanges, locales.CommitExpandAll(), locales.CommitExpandAll(), 0, false).Clicked() {
			v.refFolded, v.refsCollapsed = nil, false
		}
		if toolWindowAction(c, glyph.CollapseChanges, locales.CommitCollapseAll(), locales.CommitCollapseAll(), 0, false).Clicked() {
			v.refFolded, v.refsCollapsed = nil, true
		}
	})
}

// logToggle is a tool window action that stays pressed while on.
func logToggle(c *ui.Context, icon *ui.SVG, label string, on, disabled bool) ui.Element {
	b := toolWindowAction(c, icon, label, label, 0, disabled)
	if on && !disabled {
		b.Background(theme.Of(c).SelectedRow)
	}
	return b
}

// loadMyBranches reads which branches are the user's, for Show My
// Branches.
func (a *App) loadMyBranches(v *gitLogWindow) {
	if v.mineFor == v.source.refs {
		return
	}
	v.mineFor = v.source.refs
	gen, ctx, root, refs := a.gen, a.wsCtx, a.git.root, v.source.refs
	go func() {
		mine, err := git.MyBranches(ctx, root)
		a.post.Post(func() {
			if a.gen != gen || v.mineFor != refs {
				return
			}
			if err != nil {
				a.note(locales.MsgGit()+gitErrLine(err), true)
			}
			v.mine = mine
		})
	}()
}

func (a *App) logWindowBranches(c *ui.Context, v *gitLogWindow) {
	pal := theme.Of(c)
	ui.Row(c).Fill().Clip().Children(func() {
		a.logBranchActions(c, v)
		ui.Column(c).Grow(1).Basis(0).FillHeight().Clip().Children(func() {
			ui.Row(c).FillWidth().Height(editorTabBarHeight).Padding(0, 6, 0, 0).AlignItems(ui.Center).Children(func() {
				logSearchField(c, &v.refQuery, locales.GitWindowRefs(), locales.GitWindowRefs(), nil, nil).Grow(1)
			})
			query := strings.ToLower(strings.TrimSpace(v.refQuery))
			folded := func(key string) bool {
				if query != "" {
					return false
				}
				if f, ok := v.refFolded[key]; ok {
					return f
				}
				return v.refsCollapsed
			}
			var rows []refRow
			if query == "" || strings.Contains(strings.ToLower(locales.GitWindowHEAD()), query) {
				rows = append(rows, refRow{name: locales.GitWindowHEAD(), ref: "HEAD"})
			}
			for _, s := range refSections {
				var names []string
				for _, ref := range v.refs {
					name, ok := strings.CutPrefix(ref, s.prefix)
					if !ok || query != "" && !strings.Contains(strings.ToLower(name), query) || v.showMine && s.key != "tags" && !v.mine[ref] {
						continue
					}
					names = append(names, name)
				}
				if s.key == "local" && a.git.head == "" && a.git.branch != "" && !slices.Contains(names, a.git.branch) {
					names = append(names, a.git.branch) // a branch without commits yet
				}
				if len(names) == 0 {
					continue
				}
				row := refRow{key: s.key, name: s.label(), folded: folded(s.key)}
				rows = append(rows, row)
				if row.folded {
					continue
				}
				if s.key == "remote" {
					// Remote branches sit under their remote, as in IntelliJ.
					byRemote := map[string][]string{}
					var remotes []string
					for _, n := range names {
						remote, rest, _ := strings.Cut(n, "/")
						if _, ok := byRemote[remote]; !ok {
							remotes = append(remotes, remote)
						}
						byRemote[remote] = append(byRemote[remote], rest)
					}
					slices.Sort(remotes)
					for _, r := range remotes {
						k := s.key + "/" + r
						rr := refRow{key: k, name: r, depth: 1, folded: folded(k), folder: true}
						rows = append(rows, rr)
						if !rr.folded {
							rows = append(rows, refTree(k, s.prefix+r+"/", byRemote[r], !v.flatRefs, a.isFavorite, folded, 2)...)
						}
					}
					continue
				}
				rows = append(rows, refTree(s.key, s.prefix, names, !v.flatRefs, a.isFavorite, folded, 1)...)
			}
			ui.List(c, &v.refList, len(rows), func(i int) {
				r := rows[i]
				id := r.key + "\x00" + r.ref
				b := ui.ButtonBase(c.Key(id)).FillWidth().Height(logRowHeight).Padding(0, 8, 0, float32(4+16*r.depth)).Gap(4).AlignItems(ui.Center).Justify(ui.Start).Label(r.name).Radius(4)
				if r.ref != "" && r.ref == v.refSel {
					b.Background(pal.ListSelection)
				} else if b.Hovered() {
					b.Background(pal.Pill)
				}
				b.Children(func() {
					switch {
					case r.key != "":
						arrow := glyph.Restore
						if r.folded {
							arrow = glyph.Forward
						}
						glyph.Draw(c, arrow, 16, pal.Muted)
						if r.folder {
							glyph.Draw(c, glyph.Folder, 16, pal.Muted)
						}
					case r.ref == "HEAD":
						ui.Box(c).Width(16).Shrink(0)
					default:
						ui.Box(c).Width(16).Shrink(0)
						glyph.Draw(c, a.refIcon(r.ref), 16, a.refIconColor(c, r.ref))
					}
					ui.Text(c, r.name).SingleLine().Ellipsis("…").Shrink(1)
				})
				b.OnClick(func() {
					if r.key != "" {
						if v.refFolded == nil {
							v.refFolded = map[string]bool{}
						}
						v.refFolded[r.key] = !r.folded
						return
					}
					v.refSel, v.filter.Ref = r.ref, r.ref
				})
				if r.key == "" && r.ref != "HEAD" {
					name := strings.TrimPrefix(strings.TrimPrefix(strings.TrimPrefix(r.ref, "refs/heads/"), "refs/remotes/"), "refs/tags/")
					b.ContextMenu(func(m *ui.Menu) {
						a.refContextMenu(m, r.ref, name)
					})
				}
			}).Padding(0, 4).FillWidth().Grow(1)
		})
	})
}

// refIcon is a branch's icon in the tree, as IntelliJ's: the current
// branch's label, a star for a favorite, else a branch.
func (a *App) refIcon(ref string) *ui.SVG {
	current := ref == "refs/heads/"+a.git.branch && a.git.onBranch()
	switch {
	case strings.HasPrefix(ref, "refs/tags/"):
		return glyph.RefLabel
	case current && a.isFavorite(ref):
		return glyph.CurrentFavorite
	case current:
		return glyph.CurrentBranch
	case a.isFavorite(ref):
		return glyph.Favorite
	}
	return glyph.Git
}

func (a *App) refIconColor(c *ui.Context, ref string) ui.Color {
	if icon := a.refIcon(ref); icon == glyph.Git || icon == glyph.RefLabel {
		return theme.Of(c).Muted
	}
	return theme.Of(c).RefHead
}

// logSearchField is IntelliJ's search field: a magnifier, the text, and
// what trailing draws at its end, as the log's .* and Cc.
// logSearchField is a log's search field; with submit, as IntelliJ's text
// filter (vcs.log.filter.text.on.the.fly off), Enter applies what it has.
func logSearchField(c *ui.Context, value *string, label, placeholder string, submit, trailing func()) ui.Element {
	pal := theme.Of(c)
	field := ui.Row(c).Height(26).MinWidth(0).Padding(0, 3, 0, 6).Gap(4).AlignItems(ui.Center).Radius(4).Border(1, pal.Separator).Background(pal.Editor)
	field.Children(func() {
		glyph.Draw(c, glyph.Search, 16, pal.Muted)
		in := ui.TextInput(c, value).Grow(1).Basis(0).MinWidth(0).Height(24).Padding(0, 2).Background(ui.Transparent).BorderWidth(0).
			FontSize(widget.FontSize(c, 13)).Label(label).Placeholder(placeholder)
		if submit != nil {
			in.OnSubmit(submit)
		}
		if trailing != nil {
			trailing()
		}
	})
	return field
}

// seekCommit selects a commit in the log, reading further pages until
// it comes, as IntelliJ's Go to Hash.
func (a *App) seekCommit(v *gitLogWindow, hash string) {
	if hash == "" {
		return
	}
	if i := slices.IndexFunc(v.commits, func(cm git.Commit) bool { return cm.Hash == hash }); i >= 0 && v.loaded == v.filter {
		v.selection = i
		v.list.ScrollIntoView(i)
		return
	}
	v.seek, v.seekPages = hash, 0
	if v.loaded == v.filter && v.loadedScope == v.scope && !v.loading {
		if v.done {
			a.note(fmt.Sprintf(locales.GitLogNotFound(), shortHash(hash)), true)
			v.seek = ""
			return
		}
		a.loadWindowLog(v, false)
	}
}
