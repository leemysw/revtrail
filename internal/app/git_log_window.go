// INPUT: git 日志、多仓库根与过滤条件。
// OUTPUT: 底部 Git 工具窗口的日志：分支栏（收起时为竖排 Branches 窄条）、筛选、提交列表与详情（按目录的文件树 + 提交信息）。
// POS: IntelliJ Git Log 的对应实现。
// PROTOCOL: 变更时更新此头部，然后检查本包 doc.go

package app

import (
	"context"
	"fmt"
	"maps"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"revtrail/internal/git"
	"revtrail/internal/glyph"
	"revtrail/internal/theme"
	"revtrail/internal/widget"
	"revtrail/locales"
)

type logSource struct{ root, roots, head, branch, refs, upstream string }

// logScope is which repositories the log reads, as IntelliJ's log covers
// every root unless a filter picks some: repo is the one the Repositories
// filter keeps, or "" for all, and update the commits an update brought.
type logScope struct {
	repo   string
	update *logUpdate
}

// logUpdate is what View Commits shows after Update Project: the range
// each repository moved by.
type logUpdate struct {
	label  string
	ranges map[string]string // root → old..new
}

type gitLogWindow struct {
	source, marked            logSource // marked: the source incoming and outgoing are of
	filter, loaded            git.LogFilter
	scope, loadedScope        logScope
	skips                     map[string]int // the commits read of each repository
	refQuery                  string
	textDraft, textShown      string // the text filter's field, and the filter it last showed
	refs                      []string
	refTips                   map[string]string     // full ref → its commit
	branchInfo                map[string]git.Branch // local branch's full ref → it, for Update Selected
	refSel                    string                // the ref the branches tree has selected
	refFolded                 map[string]bool       // the branches tree's sections and folders, by key
	refsCollapsed, flatRefs   bool                  // Collapse All; Group by Directory off
	showMine                  bool                  // Show My Branches
	mine                      map[string]bool       // the user's branches, by full ref
	mineFor, me               string                // the refs mine was read for; the user's name
	branchesHidden            bool
	hideDetails               bool // View Options › Show Details off
	hideAuthor, hideDate      bool // columns View Options hides
	hideHash                  bool
	seek                      string // the commit Go to Hash looks for, reading pages until it comes
	seekPages                 int
	foldedFiles               map[string]bool // the details' folders, by path; "" is the root
	filesCollapsed, flatFiles bool            // Collapse All: folders not in foldedFiles are folded; flatFiles: Group by Directory off
	fileSel                   string          // the details' selected file
	left, center, details     float32
	list, files, refList      ui.ListState
	table                     ui.Handle
	selection                 int
	restoreHash               string
	incoming, outgoing        map[string]bool
	commits                   []git.Commit
	lanes                     []string
	lanesW                    int // the most lanes a row has, for the graph's width
	horizontal                ui.ScrollState
	detail                    *commitView
	loading, done             bool
	err                       string
	cancel                    context.CancelFunc
	seq                       uint64
}

func (a *App) showGitLogPanel() {
	a.term.view, a.term.open, a.term.focused, a.term.focus = panelGitLog, true, false, nil
}

// The Git tool window's tabs.
const (
	logTabLog = iota
	logTabWorktrees
)

// gitLogWindow is the Git tool window, as IntelliJ's: its title, its Log
// and Worktrees tabs, + for another log and the list of tabs, then its
// options and Hide; under them the chosen tab.
func (a *App) gitLogWindow(c *ui.Context) {
	pal := theme.Of(c)
	if a.git.worktreesClosed && a.git.logTab == logTabWorktrees {
		a.git.logTab = logTabLog
	}
	ui.Row(c).Height(editorTabBarHeight).FillWidth().Gap(2).Padding(0, 6, 0, 10).AlignItems(ui.Center).Background(pal.TabBar).BorderWidth(0, 0, 1, 0).BorderColor(pal.Hairline).Children(func() {
		ui.Text(c, "Git").Bold().SingleLine().Margin(0, 6, 0, 0)
		a.logWindowTab(c, logTabLog, locales.GitWindowLog(), false)
		if !a.git.worktreesClosed {
			a.logWindowTab(c, logTabWorktrees, locales.GitWindowWorktrees(), true)
		}
		if toolWindowAction(c, glyph.Plus, locales.GitLogNewTab(), locales.GitLogNewTab(), 0, false).Clicked() {
			a.hideTerminal()
			a.openGitLog()
		}
		toolWindowAction(c, glyph.Restore, locales.GitLogTabList(), locales.GitLogTabList(), 0, false).Menu(func(m *ui.Menu) {
			if m.Item(locales.GitWindowLog()).Checked(a.git.logTab == logTabLog).Chosen() {
				a.git.logTab = logTabLog
			}
			if m.Item(locales.GitWindowWorktrees()).Checked(a.git.logTab == logTabWorktrees).Chosen() {
				a.git.worktreesClosed, a.git.logTab = false, logTabWorktrees
			}
		})
		ui.Box(c).Grow(1)
		ui.Row(c).Gap(toolWindowActionGap).Shrink(0).AlignItems(ui.Center).Children(func() {
			toolWindowAction(c, glyph.MoreVertical, locales.ToolOptions(), locales.ToolOptions(), 0, false).Menu(func(m *ui.Menu) {
				if m.Item(locales.MsgOpenInTab()).Chosen() {
					a.hideTerminal()
					a.openGitLog()
				}
				label := locales.TermMaximize()
				if a.term.maximized {
					label = locales.TermRestore()
				}
				if m.Item(label).Chosen() {
					a.term.maximized = !a.term.maximized
				}
			})
			toolWindowAction(c, glyph.Hide, locales.GitWindowHide(), locales.GitWindowHide(), 0, false).OnClick(a.hideTerminal)
		})
	})
	if a.git.logTab == logTabWorktrees {
		a.worktreesView(c)
		return
	}
	a.gitLogBody(c, &a.git.logWindow)
}

// logWindowTab is a tab of the Git tool window's header: its name over
// the accent line while chosen, and a close button on a closable one.
func (a *App) logWindowTab(c *ui.Context, tab int, label string, closable bool) {
	pal := theme.Of(c)
	on := a.git.logTab == tab
	b := ui.ButtonBase(c.Key(label)).Label(label).Height(editorTabBarHeight).Padding(0, 8).Gap(4).AlignItems(ui.Center).Shrink(0).KeepFocus()
	if on {
		b.Background(pal.Pill)
	} else if b.Hovered() {
		b.Background(pal.Pill)
	}
	b.Children(func() {
		ui.Text(c, label).SingleLine()
		if closable {
			close := ui.ButtonBase(c.Key("close")).Label(locales.Close()+" "+label).Size(16, 16).Center().Radius(3)
			close.Children(func() { glyph.Draw(c, glyph.CloseSmall, 16, pal.Muted) })
			close.OnClick(func() { a.git.worktreesClosed = true })
		}
		if on {
			ui.Box(c).Absolute().Left(0).Right(0).Bottom(0).Height(2).Background(pal.TabIndicator)
		}
	})
	if b.Clicked() {
		a.git.logTab = tab
	}
}

// gitLogBody is IntelliJ's log, the same in the tool window and in an
// editor tab: the branches, the commits of every repository, and the
// chosen commit's files and message.
func (a *App) gitLogBody(c *ui.Context, v *gitLogWindow) {
	if v.left == 0 {
		v.left, v.center, v.details = 28, 69, 66
	}
	if a.git.root == "" {
		msg := locales.MsgLoading()
		if a.git.err != "" {
			msg = a.git.err
		}
		logWindowHint(c, msg)
		return
	}
	src := logSource{a.git.root, strings.Join(a.git.roots, "\x00"), a.git.head, a.git.branch, a.git.refs, a.git.upstream}
	if v.scope.repo != "" && !slices.Contains(a.git.roots, v.scope.repo) {
		v.scope.repo = ""
	}
	if v.source != src {
		v.source = src
		v.incoming, v.outgoing, v.marked = nil, nil, src
		if src.upstream != "" && (a.git.ahead > 0 || a.git.behind > 0) {
			v.marked = logSource{}
			gen, ctx := a.gen, a.wsCtx
			go func() {
				incoming, outgoing := git.Incoming(ctx, src.root), git.Unpushed(ctx, src.root)
				a.post.Post(func() {
					if a.gen == gen && v.source == src {
						v.incoming, v.outgoing, v.marked = incoming, outgoing, src
					}
				})
			}()
		}
		v.refs, v.refTips = nil, map[string]string{}
		v.addRefs(src.refs)
		if len(a.git.roots) > 1 {
			// As IntelliJ's, the branches of every repository, each name
			// once: choosing one shows it in the repositories that have it.
			gen, ctx, roots := a.gen, a.wsCtx, a.git.roots
			go func() {
				var others []string
				for _, r := range roots {
					if r != src.root {
						if refs, err := git.Refs(ctx, r); err == nil {
							others = append(others, refs)
						}
					}
				}
				a.post.Post(func() {
					if a.gen != gen || v.source != src {
						return
					}
					for _, refs := range others {
						v.addRefs(refs)
					}
					v.keepRefSelection()
				})
			}()
		} else {
			v.keepRefSelection()
		}
		if v.showMine {
			a.loadMyBranches(v)
		}
		gen, ctx := a.gen, a.wsCtx
		go func() {
			bs, _ := git.Branches(ctx, src.root)
			me := git.UserName(ctx, src.root)
			a.post.Post(func() {
				if a.gen != gen || v.source != src {
					return
				}
				v.branchInfo, v.me = map[string]git.Branch{}, me
				for _, b := range bs {
					v.branchInfo[branchRef(b)] = b
				}
			})
		}()
		a.loadWindowLog(v, true)
	} else if v.loaded != v.filter || v.loadedScope != v.scope {
		a.loadWindowLog(v, true)
	}
	main := func() {
		if v.hideDetails {
			a.logWindowCommits(c, v)
			return
		}
		widget.SplitPanes(c.Key("log-changes"), ui.SplitPercent, &v.center, func() { a.logWindowCommits(c, v) }, func() { a.logWindowDetails(c, v) }).Fill()
	}
	if v.branchesHidden {
		ui.Row(c).Grow(1).FillWidth().Children(func() {
			logBranchesStrip(c, v)
			ui.Column(c).Grow(1).Basis(0).FillHeight().Children(main)
		})
	} else {
		widget.SplitPanes(c.Key("log-branches"), ui.SplitPercent, &v.left, func() { a.logWindowBranches(c, v) }, main).Grow(1).FillWidth()
	}
}

func (a *App) loadWindowLog(v *gitLogWindow, reset bool) {
	delay := false
	if reset {
		if v.cancel != nil {
			v.cancel()
		}
		delay = v.loaded.Author != v.filter.Author
		v.restoreHash = ""
		if v.loaded == v.filter && v.loadedScope == v.scope && v.detail != nil {
			v.restoreHash = v.detail.commit.Hash
		}
		v.commits, v.lanes, v.lanesW, v.detail = nil, nil, 0, nil
		v.selection = -1
		v.list = ui.ListState{Selected: &v.selection}
		v.loaded, v.loadedScope, v.done, v.err = v.filter, v.scope, false, ""
		v.skips = map[string]int{}
	} else if v.loading || v.done {
		return
	}
	v.seq++
	v.loading = true
	gen, seq, src, filter, skip := a.gen, v.seq, v.source, v.filter, len(v.commits)
	roots, ranges := a.logRoots(v), map[string]string(nil)
	if v.scope.update != nil {
		ranges = v.scope.update.ranges
	}
	skips := maps.Clone(v.skips)
	ctx, cancel := context.WithCancel(a.wsCtx)
	v.cancel = cancel
	lanes := v.lanes
	go func() {
		defer cancel()
		if delay {
			select {
			case <-ctx.Done():
				return
			case <-time.After(160 * time.Millisecond):
			}
		}
		var commits []git.Commit
		var err error
		done := false
		switch {
		case len(roots) != 1 || ranges != nil || roots[0] != src.root:
			commits, done, err = mergedLog(ctx, roots, ranges, filter, skips)
		case src.head != "" || src.refs != "" && filter.Ref != "HEAD" && filter.Ref != "refs/heads/"+src.branch:
			commits, err = git.FilteredLog(ctx, src.root, filter, skip, historyPage)
			done = len(commits) < historyPage
		default:
			done = true
		}
		if filter.Text == "" && filter.Author == "" && filter.Since == "" {
			lanes = git.Graph(commits, lanes)
		}
		a.post.Post(func() {
			if a.gen != gen || seq != v.seq {
				return
			}
			v.loading, v.cancel = false, nil
			if err != nil {
				v.err = gitErrLine(err)
				v.done = true
				return
			}
			if v.restoreHash != "" {
				for i, cm := range commits {
					if cm.Hash == v.restoreHash {
						v.selection = skip + i
						v.restoreHash = ""
						break
					}
				}
			}
			v.commits = append(v.commits, commits...)
			v.lanes, v.skips = lanes, skips
			for _, cm := range commits {
				v.lanesW = max(v.lanesW, len(cm.Before), len(cm.After), cm.Lane+1)
			}
			v.done = done
			if v.seek != "" {
				if i := slices.IndexFunc(commits, func(cm git.Commit) bool { return cm.Hash == v.seek }); i >= 0 {
					v.selection, v.seek = skip+i, ""
					v.list.ScrollIntoView(skip + i)
				} else if done || v.seekPages >= 50 {
					a.note(fmt.Sprintf(locales.GitLogNotFound(), shortHash(v.seek)), true)
					v.seek = ""
				} else {
					v.seekPages++
					a.loadWindowLog(v, false)
				}
			}
		})
	}()
}

// logRoots are the repositories a log reads.
func (a *App) logRoots(v *gitLogWindow) []string {
	switch {
	case v.scope.update != nil:
		var roots []string
		for _, r := range a.git.roots {
			if v.scope.update.ranges[r] != "" {
				roots = append(roots, r)
			}
		}
		return roots
	case v.scope.repo != "":
		return []string{v.scope.repo}
	case len(a.git.roots) > 1:
		return a.git.roots
	}
	return []string{a.git.root}
}

// mergedLog reads the next page of several repositories' logs, newest
// first, as IntelliJ's log of every root: each keeps its own order, and
// skips counts how many of each the pages so far took. Under an update,
// each repository shows the range it moved by.
func mergedLog(ctx context.Context, roots []string, ranges map[string]string, filter git.LogFilter, skips map[string]int) ([]git.Commit, bool, error) {
	pages := make([][]git.Commit, len(roots))
	more := make([]bool, len(roots))
	for i, r := range roots {
		f := filter
		switch {
		case ranges != nil:
			f.Ref = ranges[r]
		case f.Ref == "" && !git.HasRef(ctx, r, "HEAD"):
			continue
		case f.Ref != "" && !git.HasRef(ctx, r, f.Ref):
			continue // a branch another repository has
		}
		page, err := git.FilteredLog(ctx, r, f, skips[r], historyPage)
		if err != nil {
			return nil, true, fmt.Errorf("%s: %w", filepath.Base(r), err)
		}
		pages[i], more[i] = page, len(page) == historyPage
	}
	var commits []git.Commit
	for len(commits) < historyPage {
		next := -1
		for i, page := range pages {
			if len(page) > 0 && (next < 0 || page[0].Date.After(pages[next][0].Date)) {
				next = i
			}
		}
		if next < 0 {
			break
		}
		commits = append(commits, pages[next][0])
		pages[next] = pages[next][1:]
		skips[roots[next]]++
	}
	done := true
	for i := range roots {
		if more[i] || len(pages[i]) > 0 {
			done = false
		}
	}
	return commits, done, nil
}

// addRefs adds the refs git.Refs read that the log has not yet, with
// their tips: the first repository's where several have one.
func (v *gitLogWindow) addRefs(refs string) {
	for line := range strings.SplitSeq(refs, "\n") {
		fields := strings.Split(line, "\x00")
		if len(fields) != 3 || fields[2] != "" {
			continue
		}
		if _, ok := v.refTips[fields[0]]; !ok {
			v.refs = append(v.refs, fields[0])
			v.refTips[fields[0]] = fields[1]
		}
	}
}

// keepRefSelection drops the branch filter and selection once no
// repository has their branch.
func (v *gitLogWindow) keepRefSelection() {
	if v.filter.Ref != "" && v.filter.Ref != "HEAD" && v.refTips[v.filter.Ref] == "" {
		v.filter.Ref = ""
	}
	if v.refSel == "" || v.refSel != "HEAD" && v.refTips[v.refSel] == "" {
		v.refSel = v.filter.Ref
	}
}

func logWindowHint(c *ui.Context, msg string) {
	ui.Column(c).Fill().Center().Children(func() { ui.Text(c, msg).TextColor(theme.Of(c).Muted).SingleLine().Shrink(1).Tooltip(msg) })
}

func (v *gitLogWindow) branchMenu(m *ui.Menu) {
	if m.Item(locales.GitWindowAll()).Checked(v.filter.Ref == "").Chosen() {
		v.filter.Ref = ""
	}
	if m.Item(locales.GitWindowHEAD()).Checked(v.filter.Ref == "HEAD").Chosen() {
		v.filter.Ref = "HEAD"
	}
	for _, ref := range v.refs {
		if m.Item(strings.TrimPrefix(ref, "refs/")).Checked(v.filter.Ref == ref).Chosen() {
			v.filter.Ref = ref
		}
	}
}

func (a *App) repositoryMenu(v *gitLogWindow, m *ui.Menu) {
	if m.Item(locales.GitWindowAll()).Checked(v.scope.repo == "").Chosen() {
		v.scope.repo = ""
	}
	for _, r := range a.git.roots {
		if m.Item(a.repoLabel(r)).Checked(v.scope.repo == r).Chosen() {
			v.scope.repo = r
		}
	}
}

func (v *gitLogWindow) dateMenu(m *ui.Menu) {
	for _, d := range []struct{ title, since string }{{locales.GitWindowAnyDate(), ""}, {locales.GitWindowWeek(), "7 days ago"}, {locales.GitWindowMonth(), "30 days ago"}} {
		if m.Item(d.title).Checked(v.filter.Since == d.since).Chosen() {
			v.filter.Since = d.since
		}
	}
}

func (a *App) logWindowCommits(c *ui.Context, v *gitLogWindow) {
	width := v.table.Bounds(c).W
	ui.Column(c).Bind(&v.table).Fill().Clip().Children(func() {
		a.logToolbar(c, v, width)
		if u := v.scope.update; u != nil {
			ui.Row(c).FillWidth().Height(26).Padding(0, 8).Gap(6).AlignItems(ui.Center).Background(theme.Of(c).Pill).Children(func() {
				glyph.Draw(c, glyph.Pull, 12, c.Theme().Accent)
				ui.Text(c, locales.GitUpdateInfo()+" · "+u.label).SingleLine().Shrink(1).Grow(1)
				widget.GlyphButton(c, glyph.Close, locales.GitShowAllCommits(), locales.GitShowAllCommits(), false, false).OnClick(func() { v.scope.update = nil })
			})
		}
		if v.filter.Text != "" || v.filter.Author != "" || v.filter.Since != "" || v.filter.Paths != "" {
			ui.Text(c, logText("gitLogFilteredGraph")).TextColor(theme.Of(c).Muted).Padding(4, 12).FillWidth()
		}
		switch {
		case v.err != "":
			logWindowHint(c, v.err)
		case len(v.commits) == 0 && v.loading:
			logWindowHint(c, locales.MsgReadingHistory())
		case len(v.commits) == 0:
			ui.Column(c).Grow(1).FillWidth().Center().Gap(4).Children(func() {
				msg := locales.MsgNoMatches()
				if a.git.head == "" && a.git.refs == "" {
					msg = locales.MsgNoCommits()
				}
				ui.Text(c, msg).TextColor(theme.Of(c).Muted)
				if msg == locales.MsgNoCommits() {
					ui.ButtonBase(c).Label(locales.GitWindowCommit()).OnClick(func() { a.showCommitNavigator() }).Children(func() { ui.Text(c, locales.GitWindowCommit()).TextColor(c.Theme().Accent) })
				}
			})
		default:
			columns := logColumns(width, v.lanesW)
			if v.hideAuthor {
				columns.author = 0
			}
			if v.hideDate {
				columns.date = 0
			}
			if v.hideHash {
				columns.hash = 0
			}
			graphVisible := v.filter.Text == "" && v.filter.Author == "" && v.filter.Since == "" && v.filter.Paths == ""
			ui.ScrollHorizontal(c).Grow(1).FillWidth().TrackScroll(&v.horizontal).Label(logText("gitLogTable")).Children(func() {
				ui.Column(c).Width(max(width, columns.minimumWidth())).Shrink(0).FillHeight().Padding(0, 4).Children(func() {
					ui.List(c, &v.list, len(v.commits), func(i int) {
						if i == len(v.commits)-1 && !v.done && !v.loading {
							a.loadWindowLog(v, false)
						}
						a.logRow(c, v, i, columns, graphVisible)
					}).Grow(1).FillWidth().Label(locales.MsgCommits())
				})
			})
			if v.selection >= 0 && v.selection < len(v.commits) {
				cm := v.commits[v.selection]
				if v.detail == nil || v.detail.commit.Hash != cm.Hash {
					v.restoreHash = ""
					v.detail = a.readCommitViewAt(a.commitRoot(cm), cm)
					v.files = ui.ListState{}
					v.foldedFiles, v.filesCollapsed = nil, false
				}
			}
		}
	})
}

// logToolbar is the log's toolbar, as IntelliJ's: the text filter with
// its .* and Cc, the Branch, User, Date and Paths filters, IntelliSort,
// then Refresh, Cherry-Pick, View Options and Go to Hash/Branch/Tag.
// Where it is narrow the filters fold into one menu.
func (a *App) logToolbar(c *ui.Context, v *gitLogWindow, width float32) {
	ui.Row(c).Height(editorTabBarHeight).FillWidth().Padding(0, 6).Gap(2).AlignItems(ui.Center).Clip().Children(func() {
		// The field holds what is typed; Enter applies it, and emptying it
		// clears the filter at once.
		if v.filter.Text != v.textShown {
			v.textDraft, v.textShown = v.filter.Text, v.filter.Text
		}
		if v.textDraft == "" && v.filter.Text != "" {
			v.filter.Text, v.textShown = "", ""
		}
		logSearchField(c, &v.textDraft, locales.MsgFilterCommits(), locales.GitWindowQuery(), func() {
			v.filter.Text, v.textShown = v.textDraft, v.textDraft
		}, func() {
			if filterOption(c, ".*", locales.MsgRegularExpression(), v.filter.Regexp).Clicked() {
				v.filter.Regexp = !v.filter.Regexp
			}
			if filterOption(c, "Cc", locales.MsgMatchCase(), v.filter.CaseSensitive).Clicked() {
				v.filter.CaseSensitive = !v.filter.CaseSensitive
			}
		}).Grow(1).Basis(0).MinWidth(150).MaxWidth(300).Margin(0, 4, 0, 0)
		// The filters fold into one menu before they squeeze the field.
		fold := float32(600)
		if len(a.git.roots) > 1 {
			fold += 110 // Repositories
		}
		if width > 0 && width < fold {
			active := v.filter.Ref != "" || v.filter.Since != "" || v.filter.Author != "" || v.filter.Paths != "" || v.scope.repo != ""
			logFilterButton(c, locales.MsgFilters(), active).Menu(func(m *ui.Menu) {
				m.Submenu(locales.GitWindowBranchFilter(), v.branchMenu)
				m.Submenu(locales.GitWindowUser(), func(m *ui.Menu) { a.userMenu(v, m) })
				m.Submenu(locales.GitWindowDate(), v.dateMenu)
				m.Submenu(locales.GitLogPaths(), func(m *ui.Menu) { a.pathsMenu(v, m) })
				if len(a.git.roots) > 1 {
					m.Submenu(locales.GitWindowRepositories(), func(m *ui.Menu) { a.repositoryMenu(v, m) })
				}
			})
		} else {
			branch := locales.GitWindowBranchFilter()
			if v.filter.Ref != "" {
				branch += ": " + strings.TrimPrefix(strings.TrimPrefix(strings.TrimPrefix(v.filter.Ref, "refs/heads/"), "refs/remotes/"), "refs/tags/")
			}
			logFilterButton(c, branch, v.filter.Ref != "").Menu(v.branchMenu)
			user := locales.GitWindowUser()
			if v.filter.Author != "" {
				user += ": " + v.filter.Author
			}
			logFilterButton(c, user, v.filter.Author != "").Menu(func(m *ui.Menu) { a.userMenu(v, m) })
			logFilterButton(c, locales.GitWindowDate(), v.filter.Since != "").Menu(v.dateMenu)
			paths := locales.GitLogPaths()
			if v.filter.Paths != "" {
				paths += ": " + strings.ReplaceAll(strings.TrimSpace(v.filter.Paths), "\n", ", ")
			}
			logFilterButton(c, paths, v.filter.Paths != "").Menu(func(m *ui.Menu) { a.pathsMenu(v, m) })
			if len(a.git.roots) > 1 {
				logFilterButton(c, locales.GitWindowRepositories(), v.scope.repo != "").Menu(func(m *ui.Menu) { a.repositoryMenu(v, m) })
			}
		}
		if logToggle(c, glyph.IntelliSort, locales.GitLogIntelliSort(), !v.filter.DateOrder, false).Clicked() {
			v.filter.DateOrder = !v.filter.DateOrder
		}
		ui.Box(c).Grow(1).MinWidth(4)
		if toolWindowAction(c, glyph.Refresh, locales.CommitRefresh(), locales.CommitRefresh(), 0, v.loading).Clicked() {
			a.refreshGit()
			a.loadWindowLog(v, true)
		}
		var selected *git.Commit
		if v.selection >= 0 && v.selection < len(v.commits) {
			selected = &v.commits[v.selection]
		}
		pick := selected != nil && !a.git.busy && a.git.onBranch() && a.commitRoot(*selected) == a.git.root && len(selected.Parents) < 2 && !v.outgoing[selected.Hash]
		if toolWindowAction(c, glyph.CherryPick, locales.GitCherryPick(), locales.GitCherryPick(), 0, !pick).Clicked() {
			a.cherryPick(*selected)
		}
		toolWindowAction(c, glyph.CommitView, locales.CommitViewOptions(), locales.CommitViewOptions(), 0, false).Menu(func(m *ui.Menu) {
			if m.Item(locales.GitLogShowDetails()).Checked(!v.hideDetails).Chosen() {
				v.hideDetails = !v.hideDetails
			}
			m.Separator()
			if m.Item(locales.GitLogAuthorColumn()).Checked(!v.hideAuthor).Chosen() {
				v.hideAuthor = !v.hideAuthor
			}
			if m.Item(locales.GitWindowDate()).Checked(!v.hideDate).Chosen() {
				v.hideDate = !v.hideDate
			}
			if m.Item(locales.GitLogHashColumn()).Checked(!v.hideHash).Chosen() {
				v.hideHash = !v.hideHash
			}
		})
		if toolWindowAction(c, glyph.Search, locales.GitLogGoTo(), locales.GitLogGoTo(), 0, false).Clicked() {
			a.promptGoToCommit(v)
		}
	})
}

// logFilterButton is a filter of the log's toolbar: its name, with what
// it keeps once set, over a down arrow.
func logFilterButton(c *ui.Context, label string, active bool) ui.Element {
	pal := theme.Of(c)
	b := ui.ButtonBase(c.Key(label)).Label(label).Tooltip(label).Height(24).Padding(0, 2, 0, 6).Gap(1).Radius(4).AlignItems(ui.Center).Shrink(0).KeepFocus()
	if active {
		b.Background(pal.SelectedRow)
	} else if b.Hovered() {
		b.Background(pal.Pill)
	}
	b.Children(func() {
		ui.Text(c, label).SingleLine().Ellipsis("…").MaxWidth(160)
		glyph.Draw(c, glyph.Restore, 16, pal.Muted)
	})
	return b
}

// userMenu is IntelliJ's User filter: anyone, me, the authors of the
// commits read so far, or a name to type.
func (a *App) userMenu(v *gitLogWindow, m *ui.Menu) {
	if m.Item(locales.GitLogAll()).Checked(v.filter.Author == "").Chosen() {
		v.filter.Author = ""
	}
	if v.me != "" && m.Item(locales.GitLogMe()).Checked(v.filter.Author == v.me).Chosen() {
		v.filter.Author = v.me
	}
	var authors []string
	for _, cm := range v.commits {
		if cm.Author != v.me && !slices.Contains(authors, cm.Author) {
			authors = append(authors, cm.Author)
		}
		if len(authors) == 10 {
			break
		}
	}
	slices.Sort(authors)
	if len(authors) > 0 {
		m.Separator()
	}
	for _, name := range authors {
		if m.Item(name).Checked(v.filter.Author == name).Chosen() {
			v.filter.Author = name
		}
	}
	m.Separator()
	if m.Item(locales.GitLogSelectUser()).Chosen() {
		a.ask(&gitPrompt{title: locales.GitWindowUser(), placeholder: logText("gitLogAuthor"), submitLabel: locales.GitLogApply(), value: v.filter.Author, optional: true,
			submit: func(name string, _ bool) { v.filter.Author = strings.TrimSpace(name) }})
	}
}

// pathsMenu is IntelliJ's Paths filter: every path, or the folders and
// files typed, one per line.
func (a *App) pathsMenu(v *gitLogWindow, m *ui.Menu) {
	if m.Item(locales.GitLogAll()).Checked(v.filter.Paths == "").Chosen() {
		v.filter.Paths = ""
	}
	if v.filter.Paths != "" {
		m.Item(strings.ReplaceAll(strings.TrimSpace(v.filter.Paths), "\n", ", ")).Checked(true)
	}
	if m.Item(locales.GitLogSelectPaths()).Chosen() {
		a.ask(&gitPrompt{title: locales.GitLogPaths(), detail: locales.GitLogPathsDetail(), placeholder: "internal/app", submitLabel: locales.GitLogApply(), value: v.filter.Paths, optional: true, multiline: true,
			submit: func(paths string, _ bool) { v.filter.Paths = strings.TrimSpace(paths) }})
	}
}

// promptGoToCommit is IntelliJ's Go to Hash/Branch/Tag.
func (a *App) promptGoToCommit(v *gitLogWindow) {
	a.ask(&gitPrompt{title: locales.GitLogGoTo(), placeholder: locales.GitLogGoToHint(), submitLabel: locales.GitLogGo(),
		submit: func(rev string, _ bool) {
			gen, ctx, root := a.gen, a.wsCtx, a.git.root
			rev = strings.TrimSpace(rev)
			go func() {
				hash, err := git.ResolveCommit(ctx, root, rev)
				a.post.Post(func() {
					if a.gen != gen {
						return
					}
					if err != nil {
						a.note(locales.MsgGit()+gitErrLine(err), true)
						return
					}
					if !slices.ContainsFunc(v.commits, func(cm git.Commit) bool { return cm.Hash == hash }) {
						// As IntelliJ, a commit the filters hide is shown with them reset.
						v.filter = git.LogFilter{DateOrder: v.filter.DateOrder}
						v.scope.update = nil
					}
					a.seekCommit(v, hash)
				})
			}()
		}})
}

// logDetailsToolbar is the toolbar over a commit's files, as IntelliJ's:
// Show Diff, Revert Selected Changes, Show History and View Options, then
// Expand and Collapse All.
func (a *App) logDetailsToolbar(c *ui.Context, v *gitLogWindow, d *commitView) {
	var sel *git.FileStat
	for i := range d.details.Files {
		if d.details.Files[i].Path == v.fileSel {
			sel = &d.details.Files[i]
		}
	}
	ui.Row(c).Height(editorTabBarHeight).FillWidth().Padding(0, 6).Gap(toolWindowActionGap).AlignItems(ui.Center).Children(func() {
		if toolWindowAction(c, glyph.ShowDiff, locales.CommitShowDiff(), locales.CommitShowDiff(), 0, sel == nil).Clicked() {
			a.term.maximized = false
			a.openCommitFile(d.root, a.focus, d.commit, *sel)
		}
		if toolWindowAction(c, glyph.Revert, locales.GitLogRevertSelected(), locales.GitLogRevertSelected(), 0, sel == nil || a.git.busy).Clicked() {
			ctx, root, hash, p := a.wsCtx, d.root, d.commit.Hash, sel.Path
			a.startGitJob(gitJob{done: fmt.Sprintf(locales.GitLogReverted(), path.Base(p)), run: func() error { return git.RevertFiles(ctx, root, hash, []string{p}) }})
		}
		if toolWindowAction(c, glyph.CommitHistory, locales.GitLogShowHistory(), locales.GitLogShowHistory(), 0, sel == nil || sel.Status == 'D').Clicked() {
			a.openFileHistoryOf(filepath.Join(d.root, filepath.FromSlash(sel.Path)))
		}
		toolWindowAction(c, glyph.CommitView, locales.CommitViewOptions(), locales.CommitViewOptions(), 0, false).Menu(func(m *ui.Menu) {
			if m.Item(locales.CommitGroupDirectory()).Checked(!v.flatFiles).Chosen() {
				v.flatFiles = !v.flatFiles
			}
		})
		ui.Box(c).Grow(1)
		if toolWindowAction(c, glyph.ExpandChanges, locales.CommitExpandAll(), locales.CommitExpandAll(), 0, v.flatFiles).Clicked() {
			v.foldedFiles, v.filesCollapsed = nil, false
		}
		if toolWindowAction(c, glyph.CollapseChanges, locales.CommitCollapseAll(), locales.CommitCollapseAll(), 0, v.flatFiles).Clicked() {
			v.foldedFiles, v.filesCollapsed = nil, true
		}
	})
}

func (a *App) logWindowDetails(c *ui.Context, v *gitLogWindow) {
	d := v.detail
	widget.SplitPanes(c.Key("log-detail"), ui.SplitVerticalPercent, &v.details, func() {
		if d == nil {
			logWindowHint(c, locales.GitWindowSelect())
			return
		}
		if d.loading {
			logWindowHint(c, locales.MsgReadingTheCommit())
			return
		}
		if d.err != "" {
			logWindowHint(c, d.err)
			return
		}
		ui.Column(c).Fill().Children(func() {
			a.logDetailsToolbar(c, v, d)
			folded := func(dir string) bool {
				if f, ok := v.foldedFiles[dir]; ok {
					return f
				}
				return v.filesCollapsed && dir != ""
			}
			rows := commitFileTree(d.details.Files, folded, v.flatFiles)
			ui.List(c, &v.files, len(rows), func(i int) {
				a.logFileRow(c, v, d, rows[i])
			}).FillWidth().Grow(1).Label(locales.MsgDFileSChanged(len(d.details.Files)))
		})
	}, func() {
		if d == nil {
			logWindowHint(c, locales.GitWindowDetails())
			return
		}
		ui.Scroll(c.Key(d.commit.Hash)).Fill().Children(func() {
			ui.Column(c).FillWidth().Padding(6, 10).Gap(4).Children(func() {
				ui.Text(c, d.commit.Subject).Bold().Selectable()
				ui.Text(c, d.commit.Short+" · "+d.commit.Author+" · "+d.commit.Date.Format("2006-01-02 15:04")).TextColor(theme.Of(c).Muted).Selectable()
				if body := messageBody(d.details.Body, d.commit.Subject); body != "" {
					ui.Text(c, body).Selectable().FillWidth()
				}
			})
		})
	}).Fill()
}

// commitFileRow is a row of a commit's file tree: a folder, with how
// many files it holds, or a file.
type commitFileRow struct {
	dir, name string // dir: the folder's path, "" for the root and files
	depth     int
	count     int
	folded    bool
	file      *git.FileStat
}

// commitFileTree is IntelliJ's tree of a commit's files: a root with
// their count, then folders, a chain of single folders as one, each
// before its files; flat, the files under the root by path. folded says
// which folders are closed.
func commitFileTree(files []git.FileStat, folded func(dir string) bool, flat bool) []commitFileRow {
	sorted := slices.Clone(files)
	slices.SortFunc(sorted, func(l, r git.FileStat) int { return strings.Compare(l.Path, r.Path) })
	rows := []commitFileRow{{count: len(sorted), folded: folded("")}}
	if rows[0].folded {
		return rows
	}
	if flat {
		for i := range sorted {
			rows = append(rows, commitFileRow{name: sorted[i].Path, depth: 1, file: &sorted[i]})
		}
		return rows
	}
	var walk func(prefix string, files []git.FileStat, depth int)
	walk = func(prefix string, files []git.FileStat, depth int) {
		for i := 0; i < len(files); {
			part, _, nested := strings.Cut(files[i].Path[len(prefix):], "/")
			if !nested {
				i++
				continue
			}
			base := prefix + part + "/"
			end := i + 1
			for end < len(files) && strings.HasPrefix(files[end].Path, base) {
				end++
			}
			dir := path.Dir(files[i].Path)
			for !strings.HasPrefix(files[end-1].Path, dir+"/") {
				dir = path.Dir(dir)
			}
			row := commitFileRow{dir: dir, name: strings.TrimPrefix(dir, prefix), depth: depth, count: end - i, folded: folded(dir)}
			rows = append(rows, row)
			if !row.folded {
				walk(dir+"/", files[i:end], depth+1)
			}
			i = end
		}
		for i := range files {
			if !strings.Contains(files[i].Path[len(prefix):], "/") {
				rows = append(rows, commitFileRow{name: path.Base(files[i].Path), depth: depth, file: &files[i]})
			}
		}
	}
	walk("", sorted, 1)
	return rows
}

func (a *App) logFileRow(c *ui.Context, v *gitLogWindow, d *commitView, r commitFileRow) {
	pal := theme.Of(c)
	key := r.dir
	if r.file != nil {
		key = "file:" + r.file.Path
	}
	row := ui.ButtonBase(c.Key(key)).Height(logRowHeight).FillWidth().Padding(0, 8, 0, float32(4+16*r.depth)).Gap(4).AlignItems(ui.Center).Justify(ui.Start).Radius(4)
	if r.file != nil && r.file.Path == v.fileSel {
		row.Background(pal.ListSelection)
	} else if row.Hovered() {
		row.Background(pal.Pill)
	}
	if f := r.file; f != nil {
		label := f.Path
		if f.OldPath != "" {
			label = f.OldPath + " → " + f.Path
		}
		row.Label(locales.MsgOpen2() + f.Path + locales.MsgAsADiff()).Tooltip(label)
		row.Children(func() {
			ui.Box(c).Width(16).Shrink(0)
			glyph.File(c, r.name, 16)
			ui.Text(c, r.name).TextColor(pal.GitFileColor(f.Status)).SingleLine().Ellipsis("…").Shrink(1)
			if f.OldPath != "" {
				ui.Text(c, "← "+f.OldPath).TextColor(pal.Muted).SingleLine().Ellipsis("…").Shrink(1)
			}
		})
		row.OnClick(func() {
			v.fileSel = f.Path
			a.term.maximized = false
			a.openCommitFile(d.root, a.focus, d.commit, *f)
		})
		return
	}
	name := r.name
	if r.dir == "" {
		name = locales.MsgDFileS(r.count)
	}
	row.Label(name).Tooltip(r.dir)
	row.Children(func() {
		arrow := glyph.Restore
		if r.folded {
			arrow = glyph.Forward
		}
		glyph.Draw(c, arrow, 16, pal.Muted)
		glyph.Draw(c, glyph.Folder, 16, pal.Muted)
		if r.dir == "" {
			ui.Text(c, name).TextColor(pal.Muted).SingleLine()
			return
		}
		ui.Text(c, name).SingleLine().Ellipsis("…").Shrink(1)
		ui.Text(c, locales.MsgDFileS(r.count)).TextColor(pal.Muted).SingleLine().Shrink(0)
	})
	row.OnClick(func() {
		if v.foldedFiles == nil {
			v.foldedFiles = map[string]bool{}
		}
		v.foldedFiles[r.dir] = !r.folded
	})
}
