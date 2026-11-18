// INPUT: git.Branches 与多仓库根。
// OUTPUT: 分支选择器（本地 / 远程 / 标签、按仓库分组）与切换、新建入口。
// POS: IntelliJ Branches 弹窗的界面层；具体 git 操作在 git_actions.go。
// PROTOCOL: 变更时更新此头部，然后检查本包 doc.go

package app

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"

	"revtrail/internal/git"
	"revtrail/internal/glyph"
	"revtrail/internal/theme"
	"revtrail/internal/widget"
	"revtrail/locales"
)

type branchPicker struct {
	open, loading, flat bool
	query, page, err    string
	repoPage            bool
	repoBranches        map[string][]git.Branch
	rootQuery           string
	rootSel             int
	sel                 int
	list                ui.ListState
	branches            []git.Branch
	recent, tags        []string
	target              *git.Branch
	folded              map[string]bool
	seq                 uint64
	at, anchor          ui.Rect
}

type branchItem struct {
	name, group, hint string
	page              string // the popup page it opens
	run               func() // what choosing it does, closing the popup
	repo              string
	common            bool
	depth             int
	branch            *git.Branch
	icon              *ui.SVG
	disabled, divider bool
}

func (a *App) openBranchPicker(at ui.Rect) {
	if a.git.root == "" {
		a.note(locales.MsgThisFolderIsNotInAGitRepository(), true)
		return
	}
	a.projectPicker.open = false
	p := &a.git.picker
	*p = branchPicker{open: true, loading: true, at: at, anchor: p.anchor, seq: p.seq + 1, flat: p.flat, folded: make(map[string]bool)}
	roots := slices.Clone(a.git.roots)
	seq, gen, ctx, root := p.seq, a.gen, a.wsCtx, a.git.root
	go func() {
		bs, err := git.Branches(ctx, root)
		repoBranches := map[string][]git.Branch{root: bs}
		for _, r := range roots {
			if r != root {
				repoBranches[r], _ = git.Branches(ctx, r)
			}
		}
		recent, tags := git.BranchExtras(ctx, root)
		a.post.Post(func() {
			if gen != a.gen || seq != p.seq {
				return
			}
			p.repoBranches = repoBranches
			p.loading, p.branches, p.recent, p.tags = false, bs, recent, tags
			if err != nil {
				p.err = err.Error()
			}
			p.sel = firstChoosable(a.branchItems())
		})
	}()
}

func (a *App) branchItems() []branchItem {
	p := &a.git.picker
	query := strings.TrimSpace(p.query)
	match := func(s string) bool { return strings.Contains(strings.ToLower(s), strings.ToLower(query)) }
	var items []branchItem
	add := func(it branchItem) {
		if match(it.name) {
			items = append(items, it)
		}
	}
	switch p.page {
	case "revision":
		if query != "" {
			items = append(items, branchItem{name: query, icon: glyph.Git, run: func() { a.checkoutRevision(query, query) }})
		}
		for _, tag := range p.tags {
			if tag != query {
				add(branchItem{name: tag, icon: glyph.Git, run: func() { a.checkoutRevision("refs/tags/"+tag, tag) }})
			}
		}
	case "branch":
		for _, act := range a.branchActions(*p.target) {
			add(branchItem{name: act.label, icon: act.icon, divider: act.divider, run: act.run})
		}
	default:
		if a.anyMergeConflict() {
			add(branchItem{name: locales.MergeTitle() + "…", icon: glyph.Git, run: a.showMergeConflicts})
		}
		if op := a.git.op; op != git.OpNormal {
			add(branchItem{name: locales.GitContinue() + " " + op.Label(), disabled: a.hasMergeConflict(), run: a.continueGitOp})
			if op.CanSkip() {
				add(branchItem{name: locales.GitSkip() + " " + op.Label(), run: a.skipGitOp})
			}
			add(branchItem{name: locales.GitAbort() + " " + op.Label(), run: a.abortGitOp})
		}
		add(branchItem{name: locales.BranchUpdate(), icon: glyph.Pull, hint: widget.KeyLabel("Cmd+", "T"), disabled: len(a.tracked()) == 0, run: a.reviewUpdate})
		add(branchItem{name: locales.MsgCommit() + "…", icon: glyph.CommitToolWindow, hint: widget.KeyLabel("Cmd+", "K"), run: a.showCommitNavigator})
		add(branchItem{name: locales.BranchPush(), icon: glyph.Push, hint: widget.KeyLabel("Shift+Cmd+", "K"), run: a.reviewBranchPush})
		add(branchItem{name: locales.BranchNew(), icon: glyph.Plus, divider: true, hint: widget.KeyLabel("Alt+Cmd+", "N"), run: a.promptNewBranchHere})
		add(branchItem{name: locales.BranchRevision(), page: "revision", disabled: a.git.head == ""})
		add(branchItem{name: locales.GitStashChanges(), divider: true, disabled: len(a.git.changes) == 0, run: a.promptStashChanges})
		add(branchItem{name: locales.GitUnstashChanges(), run: a.openStashesModal})
		if len(a.git.roots) > 1 && !p.repoPage {
			for i, r := range a.git.roots {
				current := ""
				for _, b := range p.repoBranches[r] {
					if b.Current {
						current = b.Name
						break
					}
				}
				add(branchItem{name: a.repoLabel(r), repo: r, hint: current, divider: i == 0})
				if query != "" {
					for j := range p.repoBranches[r] {
						b := &p.repoBranches[r][j]
						if match(b.Name) {
							items = append(items, branchItem{name: b.Name, branch: b, repo: r, depth: 1, hint: a.repoLabel(r), icon: glyph.Git})
						}
					}
				}
			}
			group := locales.CommonBranches()
			for i := range p.branches {
				if !a.syncBranches() {
					break
				}
				b := &p.branches[i]
				if b.Remote || !match(b.Name) {
					continue
				}
				common := true
				for _, r := range a.git.roots {
					if !slices.ContainsFunc(p.repoBranches[r], func(other git.Branch) bool { return !other.Remote && other.Name == b.Name }) {
						common = false
						break
					}
				}
				if !common {
					continue
				}
				if !slices.ContainsFunc(items, func(it branchItem) bool { return it.group == group }) {
					items = append(items, branchItem{name: group, group: group, divider: true})
				}
				if !p.folded[group] || query != "" {
					items = append(items, branchItem{name: b.Name, branch: b, common: true, depth: 1, icon: glyph.Git})
				}
			}
			break
		}
		recent := map[string]bool{}
		var recentRows, locals, remotes []*git.Branch
		byName := map[string]*git.Branch{}
		for i := range p.branches {
			b := &p.branches[i]
			if !b.Remote {
				byName[b.Name] = b
			}
		}
		for _, name := range append([]string{a.git.branch}, p.recent...) {
			if b := byName[name]; b != nil && !recent[name] && len(recentRows) < 8 {
				recent[name] = true
				recentRows = append(recentRows, b)
			}
		}
		for i := range p.branches {
			b := &p.branches[i]
			if b.Remote {
				remotes = append(remotes, b)
			} else if !recent[b.Name] {
				locals = append(locals, b)
			}
		}
		section := func(name string, bs []*git.Branch) {
			seen := map[string]bool{}
			start := len(items)
			for _, b := range bs {
				if !match(b.Name) {
					continue
				}
				if !seen[name] {
					seen[name] = true
					items = append(items, branchItem{name: name, group: name, divider: start > 0})
				}
				if query == "" && p.folded[name] {
					break
				}
				depth, label := 1, b.Name
				if !p.flat && query == "" {
					key := name
					parts := strings.Split(b.Name, "/")
					hidden := false
					for _, part := range parts[:len(parts)-1] {
						key += "/" + part
						if !seen[key] {
							seen[key] = true
							items = append(items, branchItem{name: part, group: key, depth: depth, icon: glyph.WelcomeFolder})
						}
						depth++
						if p.folded[key] {
							hidden = true
							break
						}
					}
					if hidden {
						continue
					}
					label = parts[len(parts)-1]
				}
				items = append(items, branchItem{name: label, branch: b, depth: depth, icon: glyph.Git, hint: b.Upstream})
			}
		}
		groupOrder := func(bs []*git.Branch) {
			slices.SortFunc(bs, func(x, y *git.Branch) int { return strings.Compare(x.Name, y.Name) })
		}
		if !p.flat && query == "" {
			groupOrder(recentRows)
			// Keep the active prefix together when putting the current branch first.
			current := strings.SplitN(a.git.branch, "/", 2)[0]
			slices.SortStableFunc(recentRows, func(x, y *git.Branch) int {
				under := func(n string) bool { return n == current || strings.HasPrefix(n, current+"/") }
				if under(x.Name) == under(y.Name) {
					return 0
				}
				if under(x.Name) {
					return -1
				}
				return 1
			})
		}
		groupOrder(locals)
		groupOrder(remotes)
		section(locales.BranchRecent(), recentRows)
		section(locales.GitWindowLocal(), locals)
		section(locales.GitWindowRemote(), remotes)
	}
	return items
}
func (a *App) repoLabel(root string) string {
	for _, r := range a.git.roots {
		if r != root && filepath.Base(r) == filepath.Base(root) {
			if rel, err := filepath.Rel(a.realRoot, root); err == nil {
				return rel
			}
		}
	}
	return filepath.Base(root)
}

func (a *App) backBranchPage() {
	p := &a.git.picker
	if p.page == "" && p.repoPage {
		p.repoPage = false
		p.query = ""
		p.sel = 0
		return
	}
	p.target = nil
	p.page, p.query, p.sel = "", p.rootQuery, p.rootSel
	p.list.ScrollTo(p.sel, ui.Center)
}

func firstChoosable(items []branchItem) int {
	for i, it := range items {
		if it.group == "" && !it.disabled {
			return i
		}
	}
	return 0
}

func (a *App) branchPage(page string) {
	p := &a.git.picker
	if p.page == "" {
		p.rootQuery, p.rootSel = p.query, p.sel
	}
	p.page, p.query = page, ""
	p.sel = firstChoosable(a.branchItems())
	p.list.ScrollTo(p.sel, ui.Start)
}
func (a *App) chooseBranch(it branchItem) {
	p := &a.git.picker
	if it.disabled || a.git.busy {
		return
	}
	switch {
	case it.group != "":
		p.folded[it.group] = !p.folded[it.group]
	case it.repo != "":
		if a.git.generatingMessage {
			return
		}
		a.selectRepository(it.repo)
		p.repoPage, p.page, p.query, p.sel = true, "", "", 0
		p.recent, p.tags = nil, nil
		p.branches = p.repoBranches[it.repo]
		p.list.ScrollTo(0, ui.Start)
		a.loadRepoBranchExtras(it.repo)
		if it.branch != nil {
			p.target = it.branch
			a.branchPage("branch")
		}
	case it.common:
		p.open = false
		a.checkoutCommonBranch(it.branch.Name, slices.Clone(a.git.roots))
	case it.branch != nil:
		p.target = it.branch
		a.branchPage("branch")
	case it.page != "":
		a.branchPage(it.page)
	case it.run != nil:
		p.open = false
		it.run()
	}
}

// promptNewBranchHere is New Branch… from HEAD.
// promptNewBranchHere is New Branch from the branches popup: from HEAD,
// in every root under synchronous branch control.
func (a *App) promptNewBranchHere() {
	if !a.syncBranches() {
		a.promptNewBranch(a.git.branch, "")
		return
	}
	ctx, roots, labels := a.wsCtx, slices.Clone(a.git.roots), make(map[string]string)
	for _, r := range roots {
		labels[r] = a.repoLabel(r)
	}
	a.ask(&gitPrompt{
		title: locales.GitCreateNewBranch(), detail: fmt.Sprintf(locales.GitInRoots(), len(roots)), placeholder: "branch-name", submitLabel: locales.MsgCreate(),
		toggleLabel: locales.GitCheckoutBranch(), toggle: true,
		submit: func(name string, checkout bool) {
			a.runBranchJob(fmt.Sprintf(locales.GitCreatedBranch(), name), func() error {
				return eachRoot(roots, labels, func(r string) error { return git.CreateBranch(ctx, r, name, "", checkout) })
			})
		},
	})
}

// syncBranches is IntelliJ's synchronous branch control: in a workspace
// of several repositories, branch operations cover them all.
func (a *App) syncBranches() bool { return !a.cfg.GitPerRootBranches && len(a.git.roots) > 1 }

// commonBranch is the branch every root is on under synchronous branch
// control, which the toolbar names alone, or "".
func (a *App) commonBranch() string {
	if !a.syncBranches() {
		return ""
	}
	for _, r := range a.git.roots {
		if a.git.repositories[r].branch != a.git.branch {
			return ""
		}
	}
	return a.git.branch
}

// eachRoot runs op in each root, stopping at the first that fails, which
// it names.
func eachRoot(roots []string, labels map[string]string, op func(string) error) error {
	for _, r := range roots {
		if err := op(r); err != nil {
			return fmt.Errorf("%s: %w", labels[r], err)
		}
	}
	return nil
}

// branchButton opens the branch picker beside the project name.
func (a *App) branchButton(c *ui.Context) {
	if a.git.branch == "" {
		return
	}
	a.git.picker.anchor = a.branchHandle.Bounds(c)
	b := titleMenuButton(c, locales.MsgSwitchBranch(), locales.MsgSwitchBranch2()+a.git.branch, a.git.picker.open).Bind(&a.branchHandle)
	b.OnClick(func() {
		if a.git.picker.open {
			a.git.picker.open = false
			return
		}
		a.openBranchPicker(a.git.picker.anchor)
	})
	b.Children(func() {
		glyph.Draw(c, glyph.Git, 16, theme.Of(c).Muted)
		label := a.git.branch
		if len(a.git.roots) > 1 && a.commonBranch() == "" {
			label = a.repoLabel(a.git.root) + " · " + label
		}
		ui.Text(c, label).TextColor(theme.Of(c).Muted).SingleLine().Ellipsis("…").MinWidth(0).Shrink(1)
		// IntelliJ's incoming and outgoing marks, without counts.
		if a.git.behind > 0 {
			s := fmt.Sprintf(locales.BranchIncoming(), a.git.behind)
			glyph.Draw(c, glyph.Pull, 14, c.Theme().Accent).Label(s).Tooltip(s)
		}
		if a.git.ahead > 0 {
			s := fmt.Sprintf(locales.BranchOutgoing(), a.git.ahead)
			glyph.Draw(c, glyph.Push, 14, theme.Of(c).GitFileColor('A')).Label(s).Tooltip(s)
		}
		glyph.Draw(c, glyph.Restore, 16, theme.Of(c).Muted)
	})
}

func (a *App) branchPickerPanel(c *ui.Context) {
	p := &a.git.picker
	items := a.branchItems()
	p.sel = min(max(p.sel, 0), max(0, len(items)-1))
	titlePickerPanel(c, p.at, 390, &p.open, func(available float32) {
		ui.Row(c).FillWidth().Gap(4).AlignItems(ui.Center).Children(func() {
			placeholder := locales.BranchSearch()
			if len(a.git.roots) > 1 {
				placeholder = locales.BranchRepoSearch()
				if p.repoPage {
					placeholder = a.repoLabel(a.git.requestedRoot) + " · " + locales.BranchSearch()
				}
			}
			if p.page != "" || p.repoPage {
				if widget.IconButton(c, glyph.Back, commandLabel("Go Back"), commandLabel("Go Back"), false).Clicked() {
					a.backBranchPage()
				}
			}
			switch p.page {
			case "revision":
				placeholder = locales.BranchRef()
			case "branch":
				placeholder = p.target.Name
			}
			var in ui.Element
			ui.Row(c).Height(29).Grow(1).Padding(0, 6).Gap(5).AlignItems(ui.Center).Radius(4).Border(1, theme.Of(c).PopoverEdge).Children(func() {
				glyph.Draw(c, glyph.Search, 14, theme.Of(c).Muted)
				in = ui.TextInput(c.Key(p.page), &p.query).Placeholder(placeholder).Label(locales.MsgFilterBranches()).Grow(1).MinWidth(0).AutoFocus().BorderWidth(0).Background(ui.Transparent)
			})
			switch {
			case in.Shortcut(ui.Cmd, ui.KeyK):
				p.open = false
				a.showCommitNavigator()
			case in.Shortcut(ui.Shift|ui.Cmd, ui.KeyK):
				p.open = false
				a.reviewBranchPush()
			case in.Shortcut(ui.Cmd, ui.KeyT) && a.git.upstream != "":
				p.open = false
				a.reviewUpdate()
			case in.Shortcut(ui.Alt|ui.Cmd, ui.KeyN):
				p.open = false
				a.promptNewBranchHere()
			case in.Changed():
				p.sel = firstChoosable(a.branchItems())
				p.list.ScrollTo(p.sel, ui.Start)
			case in.Submitted():
				if p.sel < len(items) {
					a.chooseBranch(items[p.sel])
				}
			case in.Shortcut(0, ui.KeyEscape):
				if p.page != "" || p.repoPage {
					a.backBranchPage()
				} else {
					p.open = false
				}
			case p.query == "" && in.Shortcut(0, ui.KeyRight) && p.sel < len(items):
				it := items[p.sel]
				if it.repo != "" || it.branch != nil || it.group != "" && p.folded[it.group] {
					a.chooseBranch(it)
				}
			case p.query == "" && in.Shortcut(0, ui.KeyLeft):
				if p.page != "" || p.repoPage {
					a.backBranchPage()
				} else if p.sel < len(items) && items[p.sel].group != "" {
					p.folded[items[p.sel].group] = true
				}
			case in.Shortcut(0, ui.KeyDown):
				p.sel = min(p.sel+1, len(items)-1)
				p.list.ScrollTo(max(0, p.sel), ui.Center)
			case in.Shortcut(0, ui.KeyUp):
				p.sel = max(0, p.sel-1)
				p.list.ScrollTo(p.sel, ui.Center)
			}
			if p.page == "" {
				if widget.IconButton(c, glyph.CollapseChanges, locales.CommitCollapseAll(), locales.CommitCollapseAll(), false).Clicked() {
					for _, it := range items {
						if it.group != "" {
							p.folded[it.group] = true
						}
					}
					p.sel = firstChoosable(a.branchItems())
				}
				widget.IconButton(c, glyph.Gear, locales.BranchGroup(), locales.CommitViewOptions(), false).Menu(func(m *ui.Menu) {
					if m.Item(locales.BranchGroup()).Checked(!p.flat).Chosen() {
						p.flat = !p.flat
						p.sel = firstChoosable(a.branchItems())
					}
				})
			}
		})
		switch {
		case p.loading:
			ui.Text(c, locales.MsgReadingBranches()).Padding(6)
		case p.err != "":
			ui.Text(c, p.err).TextColor(theme.Of(c).Err).Padding(6)
		case len(items) == 0:
			ui.Text(c, locales.BranchNoResults()).TextColor(theme.Of(c).Muted).Padding(6)
		default:
			ui.List(c, &p.list, len(items), func(i int) {
				it := items[i]
				pal := theme.Of(c)
				b := ui.ButtonBase(c.Key(i)).KeepFocus().FillWidth().Height(24).Padding(1, 8, 1, float32(7+it.depth*14)).Gap(6).Radius(4).AlignItems(ui.Center).Label(it.name).Disabled(it.disabled)
				if it.divider {
					b.BorderWidth(1, 0, 0, 0).BorderColor(pal.Separator)
				}
				if i == p.sel {
					b.Background(pal.ListSelection)
				} else if b.Hovered() {
					b.Background(pal.Pill)
				}
				b.OnClick(func() { p.sel = i; a.chooseBranch(it) })
				b.Children(func() {
					if it.group != "" {
						ic := glyph.Draw(c, glyph.Chevron, 10, pal.Muted)
						if !p.folded[it.group] || p.query != "" {
							ic.Rotate(90)
						}
					}
					if it.repo != "" {
						i := slices.Index(a.git.roots, it.repo)
						ui.Box(c).Size(12, 12).Radius(3).Background(repoColor(i))
					} else if it.icon != nil {
						icon := it.icon
						tint := pal.Muted
						if it.branch != nil && it.branch.Current {
							icon = glyph.CurrentBranch
							tint = theme.CurrentBranch
						}
						glyph.Draw(c, icon, 14, tint)
					} else if it.group == "" {
						ui.Box(c).Width(14)
					}
					ui.Text(c, it.name).SingleLine().Ellipsis("…").Shrink(1)
					if it.branch != nil && it.branch.Current {
						if s := branchTracking(a.git.ahead, a.git.behind); s != "" {
							ui.Text(c, s).TextColor(c.Theme().Accent)
						}
					}
					ui.Box(c).Grow(1)
					if it.hint != "" {
						ui.Text(c, it.hint).TextColor(pal.Muted).SingleLine().Ellipsis("…").Shrink(1)
					}
					if it.branch != nil || it.repo != "" {
						glyph.Draw(c, glyph.Chevron, 10, pal.Muted)
					}
				})
			}).FillWidth().Height(min(float32(len(items)*24), 360, max(24, available-50))).Label(locales.MsgBranches())
		}
	})
}

func branchTracking(ahead, behind int) string {
	var parts []string
	if behind > 0 {
		parts = append(parts, fmt.Sprintf("↙%d", behind))
	}
	if ahead > 0 {
		parts = append(parts, fmt.Sprintf("↗%d", ahead))
	}
	return strings.Join(parts, " ")
}

func (a *App) loadRepoBranchExtras(root string) {
	p := &a.git.picker
	ctx, gen, seq := a.wsCtx, a.gen, p.seq
	go func() {
		recent, tags := git.BranchExtras(ctx, root)
		a.post.Post(func() {
			if a.gen == gen && p.seq == seq && p.repoPage && a.git.requestedRoot == root {
				p.recent, p.tags = recent, tags
			}
		})
	}()
}
func repoColor(i int) ui.Color {
	return theme.RepoColors[max(0, i)%len(theme.RepoColors)]
}

func (a *App) reviewBranchPush() {
	if a.git.picker.repoPage {
		a.openPush(a.git.requestedRoot)
	} else {
		a.reviewPush()
	}
}
