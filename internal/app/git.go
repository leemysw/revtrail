// INPUT: internal/git 的状态读取与工作区监听。
// OUTPUT: gitState、仓库选择、刷新（只在前台窗口定时刷新、回到前台立即刷新）、基准版本、改动 diff 标签与提交流程。
// POS: Git 集成的核心状态；其他 git_*.go 都基于它。
// PROTOCOL: 变更时更新此头部，然后检查本包 doc.go

package app

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"revtrail/internal/git"
	"revtrail/internal/glyph"
	"revtrail/internal/theme"
	"revtrail/internal/widget"
	"revtrail/internal/workspace"
	"revtrail/locales"
)

type gitState struct {
	// detected are the repositories in the workspace, and roots those
	// its directory mappings keep: all but the excluded.
	detected, roots, excluded             []string
	scanned                               time.Time
	touched                               bool // files changed since scanned
	requestedRoot                         string
	push                                  *pushWindow
	merge                                 *mergeWindow
	messageHistory                        []string
	messageHistoryOpen, generatingMessage bool
	logWindow                             gitLogWindow

	optionsOpen                             bool
	commitAuthor, commitError               string
	signOff, checkWhitespace, checkGoFormat bool
	groupByDirectory, showIgnored, amend    bool
	ignored                                 []git.Change
	foldedChanges                           map[string]bool
	changeTreeExpansion                     changeTreeExpansion
	root, branch, message                   string
	headMessage                             string
	amendFilled                             bool
	op                                      git.RepoOp
	prompt                                  *gitPrompt
	choice                                  *gitChoiceDialog
	remoteDialog                            *remotesDialog
	clone                                   *cloneDialog
	rebase                                  *rebaseDialog
	stashes                                 stashWindow
	stashTab                                bool // the Commit tool window shows its Stash tab
	stash                                   stashPanel
	favorites                               map[string]bool // full ref → marked or unmarked as favorite; see isFavorite
	logTab                                  int             // the Git tool window's tab: logTabLog or logTabWorktrees
	worktreesClosed                         bool            // its Worktrees tab is closed
	worktrees                               worktreePanel
	compare                                 compareWindow
	fileHistory                             fileHistoryWindow
	head                                    string // HEAD's commit, so committed versions are read again only when it moves
	refs                                    string
	changes                                 []git.Change
	blocks                                  map[string]git.BlockSelection
	included                                map[string]bool // explicit choices by repository path
	selected                                map[string]bool
	rows, nextRows                          changeTreeRows
	treeHandle                              ui.Handle
	refocusTree                             bool
	treeCursor, scrollToChange              string
	deletions                               []changeDeletion
	selectionAnchor                         string
	commitSplit                             float32
	repositories                            map[string]worktreeSnapshot
	err                                     string
	loading, pending, busy                  bool
	diffSeq                                 uint64
	peek                                    *gitPeek
	picker                                  branchPicker
	// upstream is the branch HEAD's tracks, and ahead and behind how
	// far apart they are; remotes are the repository's, origin first.
	upstream      string
	ahead, behind int
	remotes       []string
	web           string // the web page of the repository, or ""
	syncing       string // what talks to the remote now: Fetching, Pulling, Pushing
	autoFetch     bool
	autoFetching  bool
	afterFetch    func() // what the user asked for during autoFetching, run once it ends
	lastFetch     time.Time
	// Line totals for each working file relative to HEAD.
	counts map[string]git.FileStat
	// pendingTabs are the Git tabs of the last session, read once the
	// repository is known.
	pendingTabs []pendingTab
}

type worktreeSnapshot struct {
	head      string
	files     map[string]byte
	basePaths map[string]string // original HEAD path for renames/copies, empty for additions
	branch    string            // the branch HEAD is on, or its short hash
	upstream  string            // the branch its HEAD tracks, as Update Project pulls
}

func (a *App) selectRepository(root string) {
	if root == a.git.root {
		return
	}
	g := &a.git
	g.amend, g.amendFilled = false, false
	g.headMessage = ""
	g.messageHistory = nil
	g.lastFetch = time.Time{}
	g.requestedRoot = root
	g.root, g.head, g.branch, g.upstream, g.refs, g.web, g.commitError = "", "", "", "", "", "", ""
	g.op = git.OpNormal
	g.prompt = nil
	g.choice = nil
	g.stashes = stashWindow{}
	g.compare = compareWindow{}
	g.fileHistory = fileHistoryWindow{}
	g.rebase = nil
	g.remotes = nil
	g.diffSeq++
	g.peek = nil
	a.refreshGit()
}

// editorRepository is the repository of the file in the editor, as
// IntelliJ's toolbar Git widget shows in a workspace of several; else the
// current one.
func (a *App) editorRepository() string {
	if len(a.git.roots) > 1 && a.focus != nil && a.docs[a.focus.active] != nil {
		if root, _ := a.treeGitStatus(a.focus.active); root != "" && slices.Contains(a.git.roots, root) {
			return root
		}
	}
	return a.git.root
}

type gitPeek struct {
	index    int
	version  uint64
	detailed bool
	original *editor.State
	doc      *doc
	group    *group
	line     int
	diff     *editor.DiffState
	scroll   ui.ScrollState
}

type gitDiffTab struct {
	digest string
	hunks  []editor.DiffHunk
	root   string
	state  *editor.DiffState // nil for a commit
	patch  editor.FilePatch
	name   string
	staged bool
	rev    string        // the commit's short hash, for a file of a commit
	commit *commitView   // a commit, which shows in place of a diff
	log    *gitLogWindow // a log in an editor tab, its own filters and selection
	// What the tab shows, to read it again in the next session: the
	// commit's full hash and a renamed file's old path.
	hash    string
	oldPath string
	pending bool // restored, and waiting to be read
}

// label tells what a diff compares: the commit, the index or the
// working tree.
func (v *gitDiffTab) label() string {
	switch {
	case v.rev != "":
		return v.rev
	case v.staged:
		return locales.GitStaged()
	}
	return locales.GitWorkingTree()
}

// page reports whether the tab shows a commit or the log, not a file's
// diff.
func (v *gitDiffTab) page() bool { return v.commit != nil || v.log != nil }

// diffStates are the diffs the tab shows: its own, or the log's of the
// commit chosen.
func (v *gitDiffTab) diffStates() []*editor.DiffState {
	switch {
	case v.state != nil:
		return []*editor.DiffState{v.state}
	}
	return nil
}

func (a *App) newGitDiffTab(p editor.FilePatch, name string, staged bool) *gitDiffTab {
	d := &editor.DiffState{Theme: a.edTheme}
	a.applyDiffPrefs(d)
	d.SetPatch(p)
	d.GoToHunk(0)
	return &gitDiffTab{root: a.git.root, hunks: d.Hunks(), state: d, patch: p, name: name, staged: staged}
}

// foreground marks the window as in front or behind; as IntelliJ does on
// frame activation, coming to the front refreshes Git at once, while
// behind it skips the periodic refresh.
func (a *App) foreground(front bool) {
	was := a.background
	a.background = !front
	if front && was && a.root != "" {
		a.refreshGit()
		a.refreshUnwatched()
	}
}

func (a *App) startGit() {
	if watchFiles == nil {
		return
	}
	a.git.autoFetch = true
	a.refreshGit()
	ctx, gen := a.wsCtx, a.gen
	go func() {
		t := time.NewTicker(5 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				a.post.Post(func() {
					// IntelliJ polls nothing: the watch brings HEAD, the
					// index and refs. Only what it cannot see is read again.
					if a.gen == gen && !a.background && a.gitUnwatched() {
						a.refreshGit()
					}
				})
			}
		}
	}()
}

func (a *App) showCommitNavigator() {
	a.term.maximized = false
	a.sidebarOff, a.navigator = false, navGit
	a.git.refocusTree = true
	a.saveNamedFiles()
	a.refreshGit()
}

func (a *App) refreshGit() {
	if a.root == "" || a.wsCtx == nil {
		return
	}
	if a.git.loading || a.git.busy {
		a.git.pending = true
		return
	}
	a.git.loading = true
	ctx, root, gen := a.wsCtx, a.root, a.gen
	wanted, detected, excluded := a.git.requestedRoot, a.git.detected, slices.Clone(a.git.excluded)
	// Walking the tree for repositories is costly in a large project: a
	// watched one walks again, at most once a minute, only after files
	// changed (applyEvents), so an idle project does not walk at all.
	scan := a.git.scanned.IsZero() || time.Since(a.git.scanned) > time.Minute && (!a.watching || a.git.touched)
	if scan {
		a.git.scanned, a.git.touched = time.Now(), false
	}
	previousHead := a.git.head
	includeIgnored := a.git.showIgnored
	counting := a.navigator == navGit && !a.sidebarOff
	go func() {
		if scan {
			detected = git.Roots(ctx, root)
		}
		var roots []string
		for _, d := range detected {
			if !slices.Contains(excluded, d) {
				roots = append(roots, d)
			}
		}
		dir := wanted
		if !slices.Contains(roots, dir) {
			dir = ""
		}
		if dir == "" && len(roots) > 0 {
			dir = roots[0]
		}
		var (
			r, b    string
			current []git.Change
			err     error
		)
		switch {
		case dir == "" && len(detected) > 0:
			err = errors.New(locales.GitNoMappedRoots())
		case dir == "":
			r, b, current, err = git.Read(ctx, root)
		default:
			if r, b, current, err = git.Read(ctx, dir); r != dir {
				r, b, current, err = "", "", nil, fmt.Errorf("repository unavailable: %s", dir)
			}
		}
		var (
			head, headMessage, upstream, web, refs string
			refsErr                                error
			ahead, behind                          int
			op                                     git.RepoOp
			remotes                                []string
			messages                               []string
			changes, ignored                       []git.Change
		)
		repos := roots
		if r != "" && !slices.Contains(repos, r) {
			repos = append([]string{r}, repos...)
		}
		repositories := make(map[string]worktreeSnapshot, len(repos))
		counts := map[string]git.FileStat{}
		for _, repo := range repos {
			status := current
			if repo != r {
				status, _ = git.WorktreeStatus(ctx, repo)
			}
			snapshot := worktreeSnapshot{head: git.Head(ctx, repo), files: make(map[string]byte, len(status)), basePaths: make(map[string]string)}
			var repoAhead, repoBehind int
			if repo == r {
				snapshot.branch = b
			} else {
				snapshot.branch, _ = git.CurrentBranch(ctx, repo)
			}
			snapshot.upstream, repoAhead, repoBehind = git.Tracking(ctx, repo)
			if repo == r {
				upstream, ahead, behind = snapshot.upstream, repoAhead, repoBehind
			}
			files := snapshot.files
			var rows []git.Change
			for _, ch := range status {
				if ch.OldPath != "" {
					snapshot.basePaths[ch.Path] = ch.OldPath
				} else if ch.Index == 'A' {
					snapshot.basePaths[ch.Path] = ""
				}
				switch {
				case ch.Index == '!':
					files[strings.TrimSuffix(ch.Path, "/")] = 'I'
					if includeIgnored {
						ignored = append(ignored, ch)
					}
					continue
				case files[ch.Path] == 'D' && ch.Index == '?':
					files[ch.Path] = 'M'
				default:
					files[ch.Path] = ch.Status()
				}
				// An embedded repository appears as one untracked directory
				// in its parent; its files are the embedded one's changes.
				if ch.Index != '?' || !slices.Contains(repos, filepath.Join(repo, filepath.FromSlash(ch.Path))) {
					rows = append(rows, ch)
				}
			}
			repositories[repo] = snapshot
			changes = append(changes, commitListChanges(rows)...)
			if counting && len(rows) > 0 {
				for name, stat := range git.LineCounts(ctx, repo, snapshot.head) {
					counts[filepath.Join(repo, filepath.FromSlash(name))] = stat
				}
			}
		}

		if err == nil {
			head = repositories[r].head
			if head != previousHead && head != "" {
				if first, recent, logErr := git.RecentCommitMessages(ctx, r, head, 20); logErr == nil {
					headMessage = first
					for _, msg := range recent {
						if !slices.Contains(messages, msg) {
							messages = append(messages, msg)
						}
					}
				}
			}
			refs, refsErr = git.Refs(ctx, r)
			op = git.OperationState(ctx, r)
			remotes = git.Remotes(ctx, r)
			remote := "origin"
			if up, _, ok := strings.Cut(upstream, "/"); ok {
				remote = up
			} else if len(remotes) > 0 {
				remote = remotes[0]
			}
			web = git.RemoteWebURL(ctx, r, remote)
		}
		a.post.Post(func() {
			if a.gen != gen {
				return
			}
			a.git.loading = false
			a.git.detected, a.git.roots = detected, roots
			if a.git.requestedRoot != wanted {
				a.git.pending = false
				a.refreshGit()
				return
			}
			if wanted != "" && !slices.Contains(roots, wanted) {
				a.git.requestedRoot = ""
				a.git.included = nil
				a.git.blocks = nil
				a.git.message = ""
			}
			if r != "" {
				a.git.requestedRoot = r
			}
			if refsErr == nil {
				a.git.refs = refs
			}
			if head != previousHead {
				if a.git.amendFilled && a.git.message == a.git.headMessage {
					a.git.message = ""
				}
				a.git.amend, a.git.amendFilled = false, false
				a.git.headMessage = headMessage
				a.git.messageHistory = messages
			}
			a.git.syncInclusion(changes)
			if op != a.git.op {
				defer a.host.RefreshMenu() // the Git menu shows the operation's items
			}
			a.git.root, a.git.branch, a.git.changes, a.git.head, a.git.op = r, b, changes, head, op
			a.git.ignored = ignored
			a.git.upstream, a.git.ahead, a.git.behind = upstream, ahead, behind
			if counting || len(changes) == 0 {
				a.git.counts = counts
			}
			if err == nil {
				a.git.web, a.git.remotes = web, remotes
			}
			a.git.repositories = repositories
			for _, d := range a.docs {
				if d.buf != nil {
					a.loadGitBase(d)
				}
			}
			a.git.err = ""
			if err != nil {
				a.git.err = err.Error()
			}
			if len(a.git.pendingTabs) > 0 {
				a.loadPendingTabs(err == nil)
			}
			if a.git.pending {
				a.git.pending = false
				a.refreshGit()
			}
			a.maybeFetchGit(time.Now())
		})
	}()
}

// Compare with the file's own worktree, including unsaved buffer edits.
func (a *App) loadGitBase(d *doc) {
	root, status := a.treeGitStatus(d.path)
	var key gitBaseRevision
	if root != "" && !d.untitled && status != 'U' && status != 'I' {
		repo := a.git.repositories[root]
		name := workspace.RelPath(root, workspace.DocPath(a.realRoot, a.root, d.path))
		if original, ok := repo.basePaths[name]; ok {
			name = original
		}
		key = gitBaseRevision{root, repo.head, name}
	}
	if d.gitBaseKey == key {
		return
	}
	d.gitBaseKey = key
	d.gitBaseSeq++
	if key.root == "" || key.path == "" || key.head == "" {
		var base *editor.Buffer
		if key.root != "" && key.path == "" {
			base = editor.NewBuffer("")
		}
		a.setGitBase(d, base)
		return
	}
	ctx, gen, seq := a.wsCtx, a.gen, d.gitBaseSeq
	go func() {
		data, err := git.Blob(ctx, key.root, key.head, key.path)
		var base *editor.Buffer
		if err == nil && len(data) <= workspace.PlainLimit && workspace.Classify(data) == workspace.Text {
			base = editor.NewBuffer(strings.TrimPrefix(string(data), "\ufeff"))
		}
		a.post.Post(func() {
			if a.gen != gen || a.docs[d.path] != d || d.gitBaseSeq != seq {
				return
			}
			if err != nil {
				d.gitBaseKey = gitBaseRevision{}
			}
			a.setGitBase(d, base)
		})
	}()
}

type gitBaseRevision struct{ root, head, path string }

func (a *App) setGitBase(d *doc, base *editor.Buffer) {
	d.gitBase = base
	d.gitChanges = nil
	if base != nil {
		d.gitChanges = &editor.DiffState{Old: base, New: d.buf}
	}
	for _, g := range a.groups() {
		if st := g.eds[d.path]; st != nil {
			st.DiffBase = base
		}
	}
	if a.git.peek != nil && a.git.peek.doc == d {
		a.git.peek = nil
	}
}

func (a *App) showGitPeek(g *group, d *doc, st *editor.State, line int) {
	if st.DiffBase != nil {
		diff := &editor.DiffState{Old: st.DiffBase, New: d.buf, Layout: editor.DiffUnified, Theme: a.edTheme, FitContent: true}
		a.applyDiffPrefs(diff)
		for i, h := range diff.Hunks() {
			if line >= h.NewStart && line < max(h.NewEnd, h.NewStart+1) || h.NewStart >= d.buf.LineCount() && line == d.buf.LineCount()-1 {
				a.choices = nil
				diff.GoToHunk(i)
				var original *editor.State
				if h.OldEnd > h.OldStart {
					first := diff.Old.Offset(editor.Pos{Line: h.OldStart})
					last := diff.Old.Offset(editor.Pos{Line: h.OldEnd - 1, Col: len(diff.Old.Line(h.OldEnd - 1))})
					buf := editor.NewBuffer(diff.Old.Slice(first, last))
					buf.SetLanguage(d.buf.Language())
					original = &editor.State{Buffer: buf, ReadOnly: true, Theme: a.edTheme, Font: st.Font, LineHeight: st.LineHeight, TabSize: st.TabSize, LineNumbers: editor.LineNumbersOff, NoFolding: true}
				}
				a.git.peek = &gitPeek{doc: d, group: g, line: line, diff: diff, index: i, version: d.buf.Version(), original: original}
				a.git.peek.scroll.Y = diffHunkOffset(st, diff, i)
				return
			}
		}
	}
	a.git.peek = nil
	start := d.buf.Offset(editor.Pos{Line: line})
	end := d.buf.Len()
	if line+1 < d.buf.LineCount() {
		end = d.buf.Offset(editor.Pos{Line: line + 1})
	}
	st.Select(start, end)
}

// commitBatch is one repository's part of a commit: IntelliJ commits the
// changes of each repository included, with the same message.
type commitBatch struct {
	root, label string
	changes     []git.Change
}

// commitBatches splits changes by repository, in their order; an amend
// with no changes amends the current repository's commit.
func (a *App) commitBatches(changes []git.Change) []commitBatch {
	var batches []commitBatch
	for _, ch := range changes {
		i := slices.IndexFunc(batches, func(b commitBatch) bool { return b.root == ch.Root })
		if i < 0 {
			i = len(batches)
			batches = append(batches, commitBatch{root: ch.Root, label: a.repoLabel(ch.Root)})
		}
		batches[i].changes = append(batches[i].changes, ch)
	}
	if len(batches) == 0 && a.git.root != "" {
		batches = append(batches, commitBatch{root: a.git.root, label: a.repoLabel(a.git.root)})
	}
	return batches
}

func (a *App) commitSelected(pushAfter bool, args []string) {
	if a.git.busy || a.git.root == "" || !a.saveNamedFiles() {
		return
	}
	a.git.busy, a.git.commitError = true, ""
	ctx, gen := a.wsCtx, a.gen
	batches := a.commitBatches(a.git.includedChanges())
	whitespace, goFormat := a.git.checkWhitespace, a.git.checkGoFormat
	go func() {
		var err error
		var committed []git.Change
		for _, b := range batches {
			if err = git.CommitSelected(ctx, b.root, b.changes, whitespace, goFormat, args); err != nil {
				if len(batches) > 1 {
					err = fmt.Errorf("%s: %w", b.label, err)
				}
				break
			}
			committed = append(committed, b.changes...)
		}
		a.post.Post(func() {
			if a.gen != gen {
				return
			}
			a.git.busy = false
			a.git.diffSeq++
			for _, ch := range committed {
				if ch.Blocks.Mask != "" {
					delete(a.git.blocks, changeID(ch))
					a.git.included[changeID(ch)] = false
				}
			}
			if err != nil {
				a.git.commitError = err.Error()
				a.note(locales.MsgGit()+err.Error(), true)
			} else {
				a.git.message, a.git.commitAuthor = "", ""
				a.git.amend, a.git.amendFilled = false, false
				a.note(locales.CommittedSelection(), false)
				if pushAfter {
					if len(batches) == 1 {
						a.openPush(batches[0].root)
					} else {
						a.reviewPush()
					}
				}
			}
			a.refreshGit()
		})
	}()
}

// commitDiff is the diff a commit of changes makes, each repository's in
// turn, as their commits will.
func commitDiff(ctx context.Context, batches []commitBatch, amend bool) (string, error) {
	var b strings.Builder
	for _, batch := range batches {
		patch, err := git.CommitDiff(ctx, batch.root, batch.changes, amend)
		if err != nil {
			return "", err
		}
		b.WriteString(patch)
	}
	return b.String(), nil
}

func (a *App) openGitFile(ch git.Change) {
	path := filepath.Join(ch.Root, filepath.FromSlash(ch.Path))
	a.term.maximized = false
	a.git.diffSeq++
	a.userOpen(path)
}

func (a *App) discardGitChanges(changes []git.Change) {
	if a.git.busy || len(changes) == 0 || a.actionConfirm != nil {
		return
	}
	gen, ctx := a.gen, a.wsCtx
	ready := func() bool {
		for _, ch := range changes {
			if ch.Status() == '!' {
				return false
			}
			for _, name := range []string{ch.Path, ch.OldPath} {
				if name == "" {
					continue
				}
				path := workspace.DocPath(a.root, a.realRoot, filepath.Join(ch.Root, filepath.FromSlash(name)))
				if d := a.docs[path]; d != nil && d.buf != nil && d.buf.Modified() {
					a.note(locales.MsgSaveOrCloseTheFileSUnsavedEdits(), true)
					return false
				}
			}
		}
		return true
	}
	if !ready() {
		return
	}
	message, detail := locales.DiscardFilesQuestion(len(changes)), locales.DiscardDetail()
	if len(changes) == 1 {
		ch := changes[0]
		message = locales.DiscardFileQuestion(ch.Path)
		if ch.Index == '?' {
			message, detail = locales.TrashUntrackedQuestion(ch.Path), locales.TrashRecoveryDetail()
		} else if ch.Index == 'A' || ch.Index == 'C' {
			detail = locales.RollbackAddedDetail()
		}
	}
	q := &confirmation{message: message, detail: detail}
	q.actions = []action{
		{locales.DiscardChanges(), func() {
			if a.actionConfirm != q || a.git.busy {
				return
			}
			a.actionConfirm = nil
			if !ready() {
				return
			}
			a.git.busy = true
			trash := a.host.TrashItem
			go func() {
				var err error
				var applied []git.Change
				for _, ch := range changes {
					if ch.Index == '?' {
						err = trash(filepath.Join(ch.Root, filepath.FromSlash(ch.Path)))
					} else {
						err = git.DiscardTrackedChange(ctx, ch.Root, ch)
					}
					if err != nil {
						break
					}
					applied = append(applied, ch)
				}
				a.post.Post(func() {
					if a.gen != gen {
						return
					}
					a.git.busy = false
					for _, ch := range applied {
						for _, name := range []string{ch.Path, ch.OldPath} {
							if name == "" {
								continue
							}
							path := workspace.DocPath(a.root, a.realRoot, filepath.Join(ch.Root, filepath.FromSlash(name)))
							if ch.Index == '?' {
								a.removed(path)
							} else if d := a.docs[path]; d != nil {
								a.reloadDoc(d)
							}
							a.refreshDir(filepath.Dir(path))
						}
					}
					if len(applied) > 0 {
						a.git.diffSeq++
						a.quick.files = nil
						a.filter.forQuery = ""
					}
					if err != nil {
						a.note(locales.MsgCouldNotDiscardGitChanges()+err.Error(), true)
					}
					a.refreshGit()
				})
			}()
		}},
		{locales.Cancel(), func() { a.actionConfirm = nil }},
	}
	a.actionConfirm = q
}

func gitChangeKey(path string) string {
	return "file\x00" + path
}

func (g *gitState) selectChange(key string, mods ui.Modifiers) {
	g.treeCursor = key
	if mods&ui.Shift != 0 && g.selectionAnchor != "" {
		keys := g.rows.order
		start, end := -1, -1
		for i, item := range keys {
			if item == g.selectionAnchor {
				start = i
			}
			if item == key {
				end = i
			}
		}
		if start >= 0 && end >= 0 {
			if mods&(ui.Cmd|ui.Ctrl) == 0 {
				g.selected = make(map[string]bool)
			}
			if g.selected == nil {
				g.selected = make(map[string]bool)
			}
			if start > end {
				start, end = end, start
			}
			for _, item := range keys[start : end+1] {
				g.selected[item] = true
			}
			return
		}
	}
	if mods&(ui.Cmd|ui.Ctrl) == 0 {
		g.selected = make(map[string]bool)
	}
	if g.selected == nil {
		g.selected = make(map[string]bool)
	}
	if mods&(ui.Cmd|ui.Ctrl) != 0 && g.selected[key] {
		delete(g.selected, key)
	} else {
		g.selected[key] = true
	}
	g.selectionAnchor = key
}

func (a *App) showGitDiff(ch git.Change, staged bool) {
	a.showGitDiffIn(ch, staged, a.focus, sideNone, len(a.focus.order), false)
}

func (a *App) showGitDiffIn(ch git.Change, staged bool, target *group, placement side, at int, preview bool) {
	if ch.Status() == '!' {
		a.openMergeAt(ch.Root, ch.Path)
		return
	}
	if ch.Root == "" || (!staged && !a.saveNamedFiles()) {
		return
	}
	a.term.maximized = false
	a.git.peek = nil
	a.git.diffSeq++
	seq, gen, root, ctx := a.git.diffSeq, a.gen, ch.Root, a.wsCtx
	go func() {
		oldPath := ch.OldPath
		if ch.Index == 'C' || ch.Work == 'C' {
			oldPath = ""
		}
		out, err := git.ChangeDiff(ctx, root, ch.Path, oldPath, staged)
		patches, parseErr := editor.ParsePatch(string(out))
		a.post.Post(func() {
			if a.gen != gen || a.git.diffSeq != seq || a.layout.find(target) == nil {
				return
			}
			if err != nil {
				a.note(locales.MsgGitDiff()+err.Error(), true)
				return
			}
			if parseErr != nil || len(patches) == 0 || patches[0].Binary {
				a.note(locales.MsgNoTextDiffAvailableFor()+ch.Path, true)
				return
			}
			v := a.newGitDiffTab(patches[0], ch.Path, staged)
			v.root, v.digest, v.oldPath = root, git.PatchDigest(out), oldPath
			key := a.repoTabKey(root, diffTab(ch.Path, staged))
			if placement != sideNone {
				target = a.splitBeside(target, placement)
				at = 0
			}
			if target.diffs == nil {
				target.diffs = map[string]*gitDiffTab{}
			}
			target.diffs[key] = v
			if !target.has(key) {
				old := target.preview
				oldDiff := target.diffs[old]
				replace := preview && oldDiff != nil && !oldDiff.page() && oldDiff.rev == ""
				if replace {
					at = slices.Index(target.order, old)
				}
				target.addTab(key, at)
				if replace {
					a.closeTab(target, old)
				}
				if preview {
					target.preview = key
				}
			} else if !preview && target.preview == key {
				target.preview = ""
			}
			a.activateIn(target, key)
		})
	}()
}

func (a *App) gitNavigator(c *ui.Context) {
	a.commitHeading(c)
	if a.git.root == "" {
		msg := "Looking for a Git repository…"
		if a.git.err != "" {
			msg = "No Git repository found, or Git is unavailable."
		}
		ui.Text(c, msg).TextColor(theme.Of(c).Muted).Padding(12, 10).Tooltip(a.git.err)
		return
	}
	if a.git.stashTab {
		a.stashView(c)
		return
	}
	var conflicts, tracked, untracked []git.Change
	for _, ch := range a.git.changes {
		if ch.Status() == '!' {
			conflicts = append(conflicts, ch)
			continue
		}
		if ch.Index == '?' {
			untracked = append(untracked, ch)
			continue
		}
		tracked = append(tracked, ch)
	}
	a.commitToolbar(c)
	a.repoOpBanner(c)
	changes := func() {
		tree := ui.Scroll(c.Key("change-tree")).Grow(1).FillWidth().Focusable().Bind(&a.git.treeHandle)
		a.git.nextRows.order = a.git.nextRows.order[:0]
		if a.git.nextRows.byKey == nil {
			a.git.nextRows.byKey = make(map[string]changeTreeRow)
		}
		clear(a.git.nextRows.byKey)
		tree.Children(func() {
			ui.Column(c).FillWidth().Padding(0, 0, 6, 0).Children(func() {
				if len(a.git.changes) == 0 {
					ui.Text(c, locales.MsgNoChanges()).TextColor(theme.Of(c).Muted).SingleLine().Ellipsis("…").FillWidth().Padding(4, 16, 8, 16)
				}
				a.commitFilesSection(c, locales.MergeSection(), conflicts)
				a.commitFilesSection(c, locales.CommitModified(), tracked)
				a.commitFilesSection(c, locales.CommitUnversioned(), untracked)
				if a.git.showIgnored && len(a.git.ignored) > 0 {
					a.gitSection(c, locales.CommitIgnoredFiles(), len(a.git.ignored))
					for _, ch := range a.git.ignored {
						name := ch.Path
						if len(a.git.roots) > 1 {
							name = a.repoLabel(ch.Root) + "/" + name
						}
						ui.Text(c, name).SingleLine().TextColor(theme.Of(c).GitIgnored).Tooltip(name).Padding(2, 16)
					}
				}
			})
		})
		a.git.rows, a.git.nextRows = a.git.nextRows, a.git.rows
		for key := range a.git.selected {
			if _, ok := a.git.rows.byKey[key]; !ok {
				delete(a.git.selected, key)
			}
		}
		if a.git.refocusTree {
			a.git.treeHandle.Focus(c)
			a.git.refocusTree = false
		}
		a.changeTreeShortcuts(tree)
	}
	if a.git.commitSplit == 0 {
		a.git.commitSplit = 70
	}
	panel := widget.SplitPanes(c.Key("changes-message"), ui.SplitVerticalPercent, &a.git.commitSplit,
		func() { ui.Column(c).Fill().Children(changes) },
		func() { a.commitBar(c, len(a.git.includedChanges())) }).Grow(1).FillWidth()
	if a.canCommit(len(a.git.includedChanges())) {
		panel.OnShortcut(ui.Cmd, ui.KeyEnter, func() { a.submitCommit(false) })
	}
	if panel.Changed() {
		a.saveConfigSoon()
	}
}

func (a *App) gitSection(c *ui.Context, name string, count int) {
	ui.Row(c).FillWidth().Padding(9, 16, 4, 16).Gap(6).AlignItems(ui.Center).Children(func() {
		ui.Text(c, name).Bold().TextColor(theme.Of(c).Muted)
		ui.Text(c, fmt.Sprint(count)).TextColor(theme.Of(c).Dim)
	})
}

func (a *App) gitRow(c *ui.Context, ch git.Change, depth int, parent string) {
	key := gitChangeKey(changeID(ch))
	a.git.addTreeRow(key, changeTreeRow{files: []git.Change{ch}, parent: parent, root: ch.Root})
	status := ch.Status()
	pal := theme.Of(c)
	nameColor := pal.GitFileColor(status)
	indent := float32(sidebarInset + 16*(depth+1))
	container := ui.Row(c.Key(key)).FillWidth().Padding(1, 8, 1, indent).Gap(2).Radius(4).AlignItems(ui.Center)
	a.git.revealChange(key, container)
	hovered := container.Hovered()
	if a.git.selected[key] || len(a.git.selected) == 0 && a.focus.active == a.repoTabKey(ch.Root, diffTab(ch.Path, false)) {
		container.Background(a.git.selectionColor(c))
		nameColor = pal.Text
	} else if hovered {
		container.Background(pal.Pill)
	}
	container.Children(func() {
		a.inclusionCheckbox(c, key, ch.Path, []git.Change{ch})
		openLabel := locales.CommitOpenDiff()
		if status == '!' {
			openLabel = locales.MergeTitle()
		}
		row := ui.ButtonBase(c).Label(openLabel+" "+ch.Path).Tooltip(ch.Path).Grow(1).Padding(4, 4).Radius(4)
		row.Drag(&tabDrag{change: &ch})
		row.Children(func() {
			ui.Row(c).FillWidth().Gap(6).AlignItems(ui.Center).Children(func() {
				glyph.File(c, filepath.Base(ch.Path), 15)
				ui.Text(c, filepath.Base(ch.Path)).TextColor(nameColor).SingleLine().Ellipsis("…").MinWidth(0).Shrink(1)
				dir := filepath.Dir(ch.Path)
				if dir != "." && !a.git.groupByDirectory {
					// The folder gives way before the name, as IntelliJ's.
					ui.Text(c, filepath.ToSlash(dir)).TextColor(theme.Of(c).Dim).SingleLine().Ellipsis("…").MinWidth(0).Basis(0).Grow(1)
				}
			})
		})
		mods := row.ClickModifiers()
		if row.Clicked() {
			a.git.selectChange(key, mods)
			a.git.treeHandle.Focus(c)
			if mods&(ui.Cmd|ui.Ctrl|ui.Shift) == 0 {
				a.showGitDiffIn(ch, false, a.focus, sideNone, len(a.focus.order), true)
			}
		}
		discard := func() {
			a.git.selectContextRow(key)
			a.discardGitChanges(a.git.selectedFiles())
		}
		canOpen := ch.Work != 'D' && !(ch.Index == 'D' && ch.Work == ' ')
		container.ContextMenu(func(m *ui.Menu) {
			a.git.selectContextRow(key)
			if m.Item(openLabel).Chosen() {
				a.showGitDiff(ch, false)
			}
			if m.Item(locales.MsgOpenFile()).Disabled(!canOpen).Chosen() {
				a.openGitFile(ch)
			}
			m.Separator()
			a.inclusionMenu(m)
			a.changeDeleteMenu(m)
			if status != '!' && m.Item(locales.DiscardChanges()).Disabled(a.git.busy).Chosen() {
				discard()
			}
		})
		if hovered {
			if canOpen && widget.GlyphButton(c, glyph.GitOpen, "Open file "+ch.Path, "Open File", false, false).Clicked() {
				a.openGitFile(ch)
			}
			if status != '!' && widget.GlyphButton(c, glyph.GitDiscard, "Discard changes "+ch.Path, "Discard Changes", false, a.git.busy).Clicked() {
				discard()
			}
		}
		if !hovered {
			a.lineCounts(c, ch)
		}
		ui.Text(c, string(status)).FontSize(widget.FontSize(c, 11)).TextColor(nameColor).Padding(0, 4)
	})
}

// lineCounts shows the lines a change adds and deletes, as Peek's list.
func (a *App) lineCounts(c *ui.Context, ch git.Change) {
	f, ok := a.git.counts[changeID(ch)]
	if !ok || f.Added < 0 {
		return
	}
	ui.Row(c).Gap(4).Shrink(0).Children(func() {
		if f.Added > 0 {
			ui.Text(c, fmt.Sprintf("+%d", f.Added)).TextColor(a.edTheme.Added).SingleLine()
		}
		if f.Deleted > 0 {
			ui.Text(c, fmt.Sprintf("−%d", f.Deleted)).TextColor(a.edTheme.Deleted).SingleLine()
		}
	})
}

// hasMergeConflict reports whether the current repository has unresolved
// conflicts, and anyMergeConflict whether any repository has.
func (a *App) hasMergeConflict() bool {
	return slices.ContainsFunc(a.git.changes, func(c git.Change) bool { return c.Status() == '!' && c.Root == a.git.root })
}

func (a *App) anyMergeConflict() bool {
	return slices.ContainsFunc(a.git.changes, func(c git.Change) bool { return c.Status() == '!' })
}

func (a *App) showMergeConflicts() {
	a.showCommitNavigator()
	a.git.toggleFolder(locales.MergeSection(), false)
}

func (a *App) continueGitOp() {
	op := a.git.op
	if op == git.OpNormal || a.hasMergeConflict() {
		return
	}
	root, ctx := a.git.root, a.wsCtx
	a.runBranchJob("Continued "+op.Label()+".", func() error { return git.ContinueOperation(ctx, root) })
}

func (a *App) skipGitOp() {
	op := a.git.op
	if !op.CanSkip() {
		return
	}
	root, ctx := a.git.root, a.wsCtx
	a.runBranchJob("Skipped step in "+op.Label()+".", func() error { return git.SkipOperation(ctx, root) })
}

func (a *App) abortGitOp() {
	op := a.git.op
	if op == git.OpNormal || a.git.busy || a.actionConfirm != nil {
		return
	}
	root, ctx := a.git.root, a.wsCtx
	label := op.Label()
	a.actionConfirm = &confirmation{
		title:   label + " in Progress",
		message: "Abort the in-progress " + strings.ToLower(label) + "?",
		actions: []action{
			{"Abort " + label, func() {
				a.actionConfirm = nil
				a.runBranchJob("Aborted "+label+".", func() error { return git.AbortOperation(ctx, root) })
			}},
			{locales.Cancel(), func() { a.actionConfirm = nil }},
		},
	}
}

func (a *App) repoOpBanner(c *ui.Context) {
	if a.git.op == git.OpNormal {
		return
	}
	pal := theme.Of(c)
	hasConflicts := a.hasMergeConflict()
	ui.Column(c).FillWidth().Padding(8, sidebarInset).Gap(6).Background(pal.Pill).BorderWidth(0, 0, 1, 0).BorderColor(pal.Hairline).Children(func() {
		ui.Row(c).FillWidth().Gap(6).AlignItems(ui.Center).Children(func() {
			glyph.Draw(c, glyph.Git, 13, c.Theme().Accent)
			ui.Text(c, fmt.Sprintf(locales.GitInProgress(), a.git.op.Label())).Bold().FontSize(widget.FontSize(c, 12))
			ui.Box(c).Grow(1)
		})
		ui.Row(c).FillWidth().Gap(4).Wrap().Children(func() {
			if hasConflicts {
				ui.Button(c, locales.GitResolve()).OnClick(a.showMergeConflicts)
			}
			ui.Button(c, locales.GitContinue()).Disabled(a.git.busy || hasConflicts).OnClick(a.continueGitOp)
			if a.git.op.CanSkip() {
				ui.Button(c, locales.GitSkip()).Disabled(a.git.busy).OnClick(a.skipGitOp)
			}
			ui.Button(c, locales.GitAbort()).Disabled(a.git.busy).OnClick(a.abortGitOp)
		})
	})
}

func (a *App) canCommit(included int) bool {
	if a.anyMergeConflict() {
		return false
	}
	message := strings.TrimSpace(a.git.message)
	return !a.git.busy && !a.git.loading && (included > 0 && message != "" || a.git.amend && a.git.head != "" && (included > 0 || message != "" && message != strings.TrimSpace(a.git.headMessage)))
}

func (a *App) submitCommit(push bool) {
	message := strings.TrimSpace(a.git.message)
	args := []string{"commit"}
	if author := strings.TrimSpace(a.git.commitAuthor); author != "" {
		args = append(args, "--author", author)
	}
	if a.git.signOff {
		args = append(args, "--signoff")
	}
	if a.git.amend {
		args = append(args, "--amend")
	}
	if message == "" {
		args = append(args, "--no-edit")
	} else {
		args = append(args, "-m", a.git.message)
	}
	a.commitSelected(push, args)
}

func (a *App) commitBar(c *ui.Context, included int) {
	can := a.canCommit(included)
	ui.Column(c).Fill().Padding(6, sidebarInset).Gap(5).Children(func() {
		ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(3).Children(func() {
			if ui.Checkbox(c, &a.git.amend, locales.CommitAmend()).Disabled(a.git.head == "" || a.git.busy).Changed() {
				if a.git.amend && a.git.message == "" {
					a.git.message = a.git.headMessage
					a.git.amendFilled = true
				} else if !a.git.amend {
					if a.git.amendFilled && a.git.message == a.git.headMessage {
						a.git.message = ""
					}
					a.git.amendFilled = false
				}
			}

			a.commitMessageActions(c, included)
		})
		if a.git.commitError != "" {
			ui.Text(c, a.git.commitError).MaxLines(2).Tooltip(a.git.commitError).TextColor(theme.Of(c).Err)
		}
		// mygo wraps a text area's placeholder; IntelliJ's stays on one line
		// and ends in an ellipsis, so the hint is drawn over the empty area
		// and lets the pointer through.
		ui.Column(c).Grow(1).MinHeight(20).FillWidth().Children(func() {
			const padX, padY = 10, 6
			ui.TextArea(c, &a.git.message).Fill().MinHeight(20).Padding(padY, padX).Label(locales.MsgCommitMessage()).Disabled(a.git.busy)
			if a.git.message == "" {
				ui.Text(c, locales.MsgCommitMessage()).TextColor(c.Theme().TextMuted).SingleLine().Ellipsis("…").
					Absolute().Left(padX + 1).Right(padX + 1).Top(padY + 1).HitTest(func(x, y float32, r ui.Rect) bool { return false })
			}
		})
		// As IntelliJ's, the buttons keep one line and a narrow window cuts them off.
		ui.Row(c).FillWidth().Gap(4).Clip().Children(func() {
			ui.PrimaryButton(c, locales.MsgCommit()).Label(locales.CommitIncludedAction()).Tooltip(locales.MsgCommit() + " (" + widget.ShortcutLabel("Enter") + ")").Disabled(!can).Shrink(0).OnClick(func() { a.submitCommit(false) })
			ui.Button(c, locales.CommitAndPush()).Disabled(!can).Shrink(0).OnClick(func() { a.submitCommit(true) })
			ui.Box(c).Grow(1)
			a.commitOptionsButton(c)
		})
	})
}

// selectionColor is the change tree's selection: blue while it has the
// keyboard, gray when it has not, as IntelliJ's lists.
func (g *gitState) selectionColor(c *ui.Context) ui.Color {
	if g.treeHandle.FocusWithin(c) {
		return theme.Of(c).ListSelection
	}
	return theme.Of(c).ListSelectionInactive
}
