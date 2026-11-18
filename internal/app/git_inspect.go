// INPUT: git 的 blame、文件历史、分支比较与补丁。
// OUTPUT: Annotate、文件历史窗口、Compare with Branch、补丁复制 / 应用。
// POS: 查看类 git 功能的界面层。
// PROTOCOL: 变更时更新此头部，然后检查本包 doc.go

package app

import (
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
	"revtrail/locales"
)

type fileHistoryWindow struct {
	open    bool
	loading bool
	path    string
	root    string
	commits []git.Commit
	list    ui.ListState
	err     string
	seq     uint64
}

type compareWindow struct {
	open    bool
	loading bool
	root    string
	base    string
	target  string
	tab     int // 0: Files, 1: Incoming (base..target), 2: Outgoing (target..base)
	list    ui.ListState
	result  git.CompareResult
	err     string
	seq     uint64
}

func (a *App) repoRelativePath(absPath string) (repoRoot, relPath string, ok bool) {
	clean := filepath.Clean(absPath)
	if eval, err := filepath.EvalSymlinks(clean); err == nil {
		clean = eval
	}
	canonical := func(r string) string {
		rc := filepath.Clean(r)
		if eval, err := filepath.EvalSymlinks(rc); err == nil {
			return eval
		}
		return rc
	}
	bestRoot := ""
	bestRaw := ""
	for _, r := range a.git.roots {
		rc := canonical(r)
		if clean == rc || strings.HasPrefix(clean, rc+string(filepath.Separator)) {
			if len(rc) > len(bestRoot) {
				bestRoot = rc
				bestRaw = r
			}
		}
	}
	if bestRoot == "" && a.git.root != "" {
		rc := canonical(a.git.root)
		if clean == rc || strings.HasPrefix(clean, rc+string(filepath.Separator)) {
			bestRoot = rc
			bestRaw = a.git.root
		}
	}
	if bestRoot == "" {
		return "", "", false
	}
	rel, err := filepath.Rel(bestRoot, clean)
	if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
		return "", "", false
	}
	return bestRaw, filepath.ToSlash(rel), true
}

func (a *App) toggleGitBlame() {
	if a.focus == nil || a.focus.active == "" {
		return
	}
	d := a.docs[a.focus.active]
	if d == nil || d.buf == nil || d.untitled {
		return
	}
	d.blameOpen = !d.blameOpen
	if !d.blameOpen {
		return
	}
	root, rel, ok := a.repoRelativePath(d.path)
	if !ok {
		d.blameErr = "file is outside repository"
		return
	}
	d.blameSeq++
	d.blameLoading, d.blameErr = true, ""
	seq, gen, ctx := d.blameSeq, a.gen, a.wsCtx
	go func() {
		lines, err := git.Blame(ctx, root, rel)
		byLine := make(map[int]git.BlameLine, len(lines))
		for _, l := range lines {
			byLine[l.Line] = l
		}
		a.post.Post(func() {
			if a.gen != gen || d.blameSeq != seq {
				return
			}
			d.blameLoading = false
			if err != nil {
				d.blameErr = gitErrLine(err)
				d.blameLines = nil
				return
			}
			d.blameLines = byLine
		})
	}()
}

func (a *App) blameStrip(c *ui.Context, d *doc, st *editor.State) {
	if !d.blameOpen {
		return
	}
	pal := theme.Of(c)
	ui.Column(c).Width(175).FillHeight().Background(pal.TabBar).BorderWidth(0, 1, 0, 0).BorderColor(pal.Hairline).Clip().Children(func() {
		switch {
		case d.blameLoading && len(d.blameLines) == 0:
			ui.Box(c).Fill().Center().Children(func() {
				ui.Text(c, locales.MsgReadingHistory()).FontSize(widget.FontSize(c, 11)).TextColor(pal.Muted)
			})
		case d.blameErr != "":
			ui.Box(c).Fill().Padding(8, 8).Children(func() {
				ui.Text(c, d.blameErr).FontSize(widget.FontSize(c, 11)).TextColor(pal.Muted)
			})
		default:
			first, last := st.VisibleLines()
			prevHash := ""
			scrollY := st.ScrollY()
			lineCount := d.buf.LineCount()
			for line := first; line <= last && line < lineCount; line++ {
				bl, ok := d.blameLines[line+1]
				top := float32(st.LineTop(line) - scrollY)
				bottom := float32(st.LineTop(line+1) - scrollY)
				h := bottom - top
				if h <= 0 {
					continue
				}
				same := ok && bl.Hash == prevHash
				if ok {
					prevHash = bl.Hash
				} else {
					prevHash = ""
				}
				row := ui.Row(c.Key(line)).Absolute().Left(0).Top(top).Width(175).Height(h).Padding(0, 6).Gap(4).AlignItems(ui.Center)
				if !same {
					row.BorderWidth(1, 0, 0, 0).BorderColor(pal.Hairline.Alpha(0.5))
				}
				if ok && bl.Hash != strings.Repeat("0", 40) {
					row.Cursor(ui.CursorPointer).Tooltip(fmt.Sprintf("%s · %s · %s\n%s", bl.Short, bl.Author, shortDate(bl.Time, time.Now()), bl.Summary))
					hash := bl.Hash
					row.OnClick(func() { a.openCommitByHash(hash) })
				}
				row.Children(func() {
					if !ok || same {
						return
					}
					date := ""
					if !bl.Time.IsZero() {
						date = shortDate(bl.Time, time.Now())
					}
					label := bl.Short
					if bl.Author != "" {
						label += " " + bl.Author
					}
					ui.Text(c, label).FontSize(widget.FontSize(c, 10)).SingleLine().Grow(1).Shrink(1).TextColor(pal.Text)
					if date != "" {
						ui.Text(c, date).FontSize(widget.FontSize(c, 10)).SingleLine().TextColor(pal.Muted)
					}
				})
			}
		}
	})
}

func (a *App) openCommitByHash(hash string) {
	if a.git.root == "" || hash == "" {
		return
	}
	root, ctx, gen := a.git.root, a.wsCtx, a.gen
	go func() {
		cm, err := git.ReadCommit(ctx, root, hash)
		a.post.Post(func() {
			if a.gen != gen {
				return
			}
			if err != nil {
				a.note(locales.MsgGit()+gitErrLine(err), true)
				return
			}
			a.openCommit(cm)
		})
	}()
}

func (a *App) openFileHistory() {
	if a.focus == nil || a.focus.active == "" {
		a.note(locales.GitOpenFileForHistory(), true)
		return
	}
	a.openFileHistoryOf(a.focus.active)
}

func (a *App) openFileHistoryOf(path string) {
	root, rel, ok := a.repoRelativePath(path)
	if !ok {
		a.note(locales.GitFileOutsideRepo(), true)
		return
	}
	f := &a.git.fileHistory
	f.open, f.loading, f.root, f.path, f.err = true, true, root, rel, ""
	f.commits = nil
	f.seq++
	seq, gen, ctx := f.seq, a.gen, a.wsCtx
	go func() {
		commits, err := git.FileHistory(ctx, root, rel, 0, 100)
		a.post.Post(func() {
			if a.gen != gen || f.seq != seq {
				return
			}
			f.loading = false
			if err != nil {
				f.err = gitErrLine(err)
				return
			}
			f.commits = commits
		})
	}()
}

func (a *App) fileHistoryModal(c *ui.Context) {
	f := &a.git.fileHistory
	if !f.open {
		return
	}
	dismiss := func() { f.open = false }
	modal(c, locales.GitFileHistory(), 680, 440, dismiss, nil, func() {
		modalHeader(c, fmt.Sprintf(locales.GitHistoryOf(), filepath.Base(f.path)), f.path, dismiss)
		switch {
		case f.loading:
			modalHint(c, locales.MsgReadingHistory())
		case f.err != "":
			modalHint(c, f.err)
		case len(f.commits) == 0:
			modalHint(c, locales.MsgNoCommits())
		default:
			a.commitList(c, &f.list, f.commits, dismiss)
		}
	})
}

// commitList is a dialog's list of commits; choosing one opens it.
func (a *App) commitList(c *ui.Context, list *ui.ListState, commits []git.Commit, dismiss func()) {
	pal := theme.Of(c)
	ui.Column(c).Grow(1).FillWidth().Border(1, pal.Hairline).Radius(5).Clip().Children(func() {
		ui.List(c, list, len(commits), func(i int) {
			cm := commits[i]
			row := ui.Row(c).FillWidth().Padding(6, 8).Gap(8).AlignItems(ui.Center).Cursor(ui.CursorPointer).Label(cm.Subject)
			if row.Hovered() {
				row.Background(pal.Pill)
			}
			row.OnClick(func() {
				dismiss()
				a.openCommit(cm)
			})
			row.Children(func() {
				ui.Text(c, cm.Short).FontSize(widget.FontSize(c, 11)).TextColor(pal.Muted)
				ui.Text(c, cm.Subject).SingleLine().Grow(1).Shrink(1)
				ui.Text(c, cm.Author).MaxWidth(110).SingleLine().TextColor(pal.Muted)
				ui.Text(c, shortDate(cm.Date, time.Now())).SingleLine().TextColor(pal.Muted)
			})
		}).Grow(1).FillWidth()
	})
}

func (a *App) promptCompareBranches() {
	if a.git.root == "" {
		return
	}
	base := a.git.branch
	if base == "" {
		base = "HEAD"
	}
	a.ask(&gitPrompt{
		title: locales.GitCompareWithBranch(), detail: fmt.Sprintf(locales.GitBase(), base), placeholder: locales.GitBranchOrTag(), submitLabel: locales.GitCompareAction(),
		submit: func(target string, _ bool) { a.openCompareWith(target) },
	})
}

func (a *App) openCompareWith(target string) {
	if strings.TrimSpace(target) == "" {
		return
	}
	base := a.git.branch
	if base == "" {
		base = "HEAD"
	}
	a.openCompare(base, target)
}

// openDiffWithWorkingTree lists the files the working tree differs in
// from ref, as IntelliJ's Show Diff with Working Tree.
func (a *App) openDiffWithWorkingTree(ref string) { a.openCompare(ref, "") }

// openCompare compares base with target, the working tree when "".
func (a *App) openCompare(base, target string) {
	if a.git.root == "" {
		return
	}
	w := &a.git.compare
	w.open, w.loading, w.root, w.base, w.target, w.tab, w.err = true, true, a.git.root, base, target, 0, ""
	w.result = git.CompareResult{}
	w.seq++
	seq, gen, ctx, root := w.seq, a.gen, a.wsCtx, a.git.root
	go func() {
		res, err := git.CompareRefs(ctx, root, base, target)
		a.post.Post(func() {
			if a.gen != gen || w.seq != seq {
				return
			}
			w.loading = false
			if err != nil {
				w.err = gitErrLine(err)
				return
			}
			w.result = res
		})
	}()
}

func (a *App) openCompareFileDiff(root, base, target string, f git.FileStat) {
	if a.focus == nil {
		return
	}
	g := a.focus
	a.git.diffSeq++
	seq, gen, ctx := a.git.diffSeq, a.gen, a.wsCtx
	go func() {
		out, err := git.CompareFileDiff(ctx, root, base, target, f)
		patches, parseErr := editor.ParsePatch(string(out))
		a.post.Post(func() {
			if a.gen != gen || a.git.diffSeq != seq || a.layout.find(g) == nil {
				return
			}
			if err != nil {
				a.note(locales.MsgGitDiff()+gitErrLine(err), true)
				return
			}
			if parseErr != nil || len(patches) == 0 || patches[0].Binary {
				a.note(locales.MsgNoTextDiffAvailableFor()+f.Path, true)
				return
			}
			if g.diffs == nil {
				g.diffs = map[string]*gitDiffTab{}
			}
			key := diffTabPrefix + "compare:" + base + ".." + target + ":" + f.Path
			v := a.newGitDiffTab(patches[0], f.Path, false)
			v.root, v.rev = root, compareLabel(base, target)
			g.diffs[key] = v
			g.addTab(key, len(g.order))
			a.activateIn(g, key)
		})
	}()
}

func compareLabel(base, target string) string {
	if target == "" {
		target = locales.GitWorkingTree()
	}
	return base + " ↔ " + target
}

func (a *App) compareBranchesModal(c *ui.Context) {
	w := &a.git.compare
	if !w.open {
		return
	}
	pal := theme.Of(c)
	dismiss := func() { w.open = false }
	modal(c, locales.GitCompareBranches(), 740, 480, dismiss, nil, func() {
		modalHeader(c, compareLabel(w.base, w.target), "", dismiss)
		if w.target != "" {
			ui.Segmented(c, &w.tab,
				fmt.Sprintf(locales.GitChangedFiles(), len(w.result.Files)),
				fmt.Sprintf(locales.GitOnlyInBranch(), w.target, len(w.result.Forward)),
				fmt.Sprintf(locales.GitOnlyInBranch(), w.base, len(w.result.Backward)),
			)
		}
		commits := w.result.Forward
		if w.tab == 2 {
			commits = w.result.Backward
		}
		switch {
		case w.loading:
			modalHint(c, locales.MsgLoading())
		case w.err != "":
			modalHint(c, w.err)
		case w.tab == 0 && len(w.result.Files) == 0:
			modalHint(c, locales.GitNoFileDiffs())
		case w.tab == 0:
			ui.Column(c).Grow(1).FillWidth().Border(1, pal.Hairline).Radius(5).Clip().Children(func() {
				ui.List(c, &w.list, len(w.result.Files), func(i int) {
					f := w.result.Files[i]
					row := ui.Row(c).FillWidth().Padding(5, 8).Gap(8).AlignItems(ui.Center).Cursor(ui.CursorPointer).Label(f.Path)
					if row.Hovered() {
						row.Background(pal.Pill)
					}
					row.OnClick(func() {
						dismiss()
						a.openCompareFileDiff(w.root, w.base, w.target, f)
					})
					row.Children(func() {
						glyph.File(c, filepath.Base(f.Path), 14)
						ui.Text(c, f.Path).SingleLine().Grow(1).Shrink(1)
						if f.Added >= 0 {
							ui.Text(c, fmt.Sprintf("+%d", f.Added)).TextColor(a.edTheme.Added)
							ui.Text(c, fmt.Sprintf("−%d", f.Deleted)).TextColor(a.edTheme.Deleted)
						}
					})
				}).Grow(1).FillWidth()
			})
		case len(commits) == 0:
			modalHint(c, locales.MsgNoCommits())
		default:
			a.commitList(c, &w.list, commits, dismiss)
		}
	})
}

func (a *App) copyWorkingPatch() {
	if len(a.git.changes) == 0 {
		a.note(locales.GitNoPatchChanges(), true)
		return
	}
	changes := a.git.includedChanges()
	if len(changes) == 0 {
		changes = slices.Clone(a.git.changes)
	}
	ctx, batches, amend, gen := a.wsCtx, a.commitBatches(changes), a.git.amend, a.gen
	go func() {
		patch, err := commitDiff(ctx, batches, amend)
		a.post.Post(func() {
			if a.gen != gen {
				return
			}
			if err != nil {
				a.note(locales.MsgGit()+gitErrLine(err), true)
				return
			}
			a.services.WriteClipboard(patch)
			a.note(locales.GitCopiedPatch(), false)
		})
	}()
}

func (a *App) copyCommitPatch(root, hash string) {
	if root == "" || hash == "" {
		return
	}
	ctx, gen := a.wsCtx, a.gen
	go func() {
		patch, err := git.CommitPatch(ctx, root, hash)
		a.post.Post(func() {
			if a.gen != gen {
				return
			}
			if err != nil {
				a.note(locales.MsgGit()+gitErrLine(err), true)
				return
			}
			a.services.WriteClipboard(string(patch))
			a.note(fmt.Sprintf(locales.GitCopiedCommitPatch(), shortHash(hash)), false)
		})
	}()
}

func (a *App) promptApplyPatch() {
	if a.git.root == "" {
		return
	}
	ctx, root := a.wsCtx, a.git.root
	initial := a.services.ReadClipboard()
	if !strings.Contains(initial, "diff --git") && !strings.Contains(initial, "@@ ") {
		initial = ""
	}
	a.ask(&gitPrompt{
		title:       locales.GitApplyPatchTitle(),
		detail:      locales.GitApplyPatchDetail(),
		placeholder: "diff --git ...",
		submitLabel: locales.GitApplyPatchAction(),
		value:       initial,
		multiline:   true,
		submit: func(patch string, _ bool) {
			a.runBranchJob(locales.GitAppliedPatch(), func() error { return git.ApplyPatchText(ctx, root, patch) })
		},
	})
}
