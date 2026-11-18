// INPUT: git 储藏列表。
// OUTPUT: Stash Changes、Unstash 窗口、Commit 工具窗口的 Stash 标签页（储藏与其文件、查看差异）与储藏操作（应用、弹出、删除、建分支）。
// POS: 储藏的界面层。
// PROTOCOL: 变更时更新此头部，然后检查本包 doc.go

package app

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"revtrail/internal/git"
	"revtrail/internal/glyph"
	"revtrail/internal/theme"
	"revtrail/internal/widget"
	"revtrail/locales"
)

// stashWindow is IntelliJ's Unstash Changes dialog.
type stashWindow struct {
	open     bool
	loading  bool
	list     ui.ListState
	entries  []git.StashEntry
	selected int
	preview  string
	index    bool // Reinstate index
	err      string
	seq      uint64
}

func (a *App) promptStashChanges() {
	ctx, root := a.wsCtx, a.git.root
	a.ask(&gitPrompt{
		title:       locales.GitStashTitle(),
		detail:      locales.GitStashDetail(),
		placeholder: locales.GitStashMessagePlaceholder(),
		submitLabel: locales.GitStashAction(),
		toggleLabel: locales.GitIncludeUntracked(),
		optional:    true,
		submit: func(msg string, untracked bool) {
			a.runBranchJob(locales.GitStashed(), func() error { return git.StashSave(ctx, root, msg, untracked, false) })
		},
	})
}

func (a *App) openStashesModal() {
	a.git.stashes = stashWindow{open: true, seq: a.git.stashes.seq}
	a.loadStashes()
}

// loadStashes reads the list and the selected entry's diff.
func (a *App) loadStashes() {
	s := &a.git.stashes
	s.seq++
	s.loading, s.err = true, ""
	seq, gen, ctx, root, want := s.seq, a.gen, a.wsCtx, a.git.root, s.selected
	go func() {
		entries, err := git.Stashes(ctx, root)
		want = min(want, max(0, len(entries)-1))
		preview := ""
		if err == nil && len(entries) > 0 {
			preview, _ = git.StashShow(ctx, root, entries[want].Ref)
		}
		a.post.Post(func() {
			if a.gen != gen || s.seq != seq {
				return
			}
			s.loading = false
			s.entries, s.selected, s.preview = entries, want, preview
			if err != nil {
				s.err = gitErrLine(err)
			}
		})
	}()
}

func (a *App) selectStash(i int) {
	s := &a.git.stashes
	s.selected = i
	a.loadStashes()
}

func (a *App) stashesModal(c *ui.Context) {
	s := &a.git.stashes
	if !s.open {
		return
	}
	pal := theme.Of(c)
	dismiss := func() { s.open = false }
	modal(c, locales.GitUnstashChanges(), 720, 440, dismiss, nil, func() {
		ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(8).Children(func() {
			ui.Text(c, locales.GitStashes()).FontSize(widget.FontSize(c, 15)).Bold()
			ui.Box(c).Grow(1)
			ui.Button(c, locales.GitStashChanges()).Disabled(a.git.busy).OnClick(func() {
				dismiss()
				a.promptStashChanges()
			})
			ui.Button(c, locales.Close()).OnClick(dismiss)
		})
		switch {
		case s.loading && len(s.entries) == 0:
			modalHint(c, locales.MsgLoading())
			return
		case s.err != "":
			modalHint(c, s.err)
			return
		case len(s.entries) == 0:
			modalHint(c, locales.GitNoStashes())
			return
		}
		sel := s.entries[s.selected]
		ui.Row(c).Grow(1).FillWidth().Gap(10).MinHeight(0).Children(func() {
			ui.Column(c).Width(260).FillHeight().Border(1, pal.Hairline).Radius(5).Clip().Children(func() {
				ui.List(c, &s.list, len(s.entries), func(i int) {
					st := s.entries[i]
					row := ui.Column(c).FillWidth().Padding(6, 8).Gap(2).Label(st.Message)
					if i == s.selected {
						row.Background(pal.ListSelection)
					}
					row.OnClick(func() { a.selectStash(i) })
					row.Children(func() {
						title := st.Message
						if title == "" {
							title = st.Ref
						}
						ui.Text(c, title).SingleLine().FontSize(widget.FontSize(c, 12)).Bold()
						meta := st.Ref
						if st.Branch != "" {
							meta += " · " + st.Branch
						}
						ui.Text(c, meta+" · "+shortDate(st.Date, time.Now())).SingleLine().FontSize(widget.FontSize(c, 11)).TextColor(pal.Muted)
					})
				}).Grow(1).FillWidth()
			})
			ui.Scroll(c.Key(sel.Ref)).Grow(1).FillHeight().MinWidth(0).Border(1, pal.Hairline).Radius(5).Padding(8).Children(func() {
				preview := s.preview
				if strings.TrimSpace(preview) == "" {
					preview = locales.GitNoDiffPreview()
				}
				ui.Text(c, preview).FontSize(widget.FontSize(c, 11))
			})
		})
		ref := sel.Ref
		ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(6).Children(func() {
			ui.Button(c, locales.GitDrop()).Disabled(a.git.busy).OnClick(func() { a.dropStash(ref, a.loadStashes) })
			ui.Button(c, locales.GitClearAll()).Disabled(a.git.busy).OnClick(func() { a.clearStashes(len(s.entries)) })
			ui.Box(c).Grow(1)
			ui.Checkbox(c, &s.index, locales.GitReinstateIndex())
			ui.Button(c, locales.BranchNew()).Disabled(a.git.busy).OnClick(func() {
				dismiss()
				a.unstashAsBranch(ref)
			})
			index := s.index
			ui.Button(c, locales.GitApply()).Disabled(a.git.busy).OnClick(func() {
				dismiss()
				a.applyStash(ref, false, index)
			})
			ui.PrimaryButton(c, locales.GitPop()).Disabled(a.git.busy).OnClick(func() {
				dismiss()
				a.applyStash(ref, true, index)
			})
		})
	})
}

func (a *App) applyStash(ref string, pop, index bool) {
	ctx, root := a.wsCtx, a.git.root
	done := fmt.Sprintf(locales.GitAppliedStash(), ref)
	if pop {
		done = fmt.Sprintf(locales.GitPoppedStash(), ref)
	}
	a.runBranchJob(done, func() error { return git.StashApply(ctx, root, ref, pop, index) })
}

// dropStash drops ref once confirmed, then runs after.
func (a *App) dropStash(ref string, after func()) {
	ctx, root := a.wsCtx, a.git.root
	a.confirmStash(fmt.Sprintf(locales.GitDropStash(), ref), locales.GitDrop(), func() {
		a.startGitJob(gitJob{done: fmt.Sprintf(locales.GitDroppedStash(), ref), run: func() error { return git.StashDrop(ctx, root, ref) }, after: after})
	})
}

func (a *App) clearStashes(n int) {
	ctx, root := a.wsCtx, a.git.root
	a.confirmStash(fmt.Sprintf(locales.GitClearStashes(), n), locales.GitClearAll(), func() {
		a.git.stashes.open = false
		a.runBranchJob(locales.GitClearedStashes(), func() error { return git.StashClear(ctx, root) })
	})
}

func (a *App) unstashAsBranch(ref string) {
	ctx, root := a.wsCtx, a.git.root
	a.ask(&gitPrompt{
		title: locales.GitUnstashAsBranch(), detail: fmt.Sprintf(locales.GitFromRef(), ref), placeholder: "branch-name", submitLabel: locales.GitCreateBranch(),
		submit: func(name string, _ bool) {
			a.runBranchJob(fmt.Sprintf(locales.GitCreatedBranchFrom(), name, ref), func() error { return git.StashBranch(ctx, root, name, ref) })
		},
	})
}

// confirmStash asks over the stashes, which stay open behind.
func (a *App) confirmStash(message, label string, run func()) {
	a.actionConfirm = &confirmation{message: message, actions: []action{
		{label, func() { a.actionConfirm = nil; run() }},
		{locales.Cancel(), func() { a.actionConfirm = nil }},
	}}
}

// stashPanel is the Commit tool window's Stash tab, as IntelliJ's: the
// stashes, each opening onto the files it holds.
type stashPanel struct {
	loading   bool
	loadedFor string // the repository listed; "" to list again
	seq       uint64
	entries   []git.StashEntry
	err       string
	open      map[string]bool // stash hashes shown open
	files     map[string]stashFiles
	selected  string // a stash's hash, or stashFileKey of one of its files
}

type stashFiles struct {
	changed, untracked []git.FileStat
	err                string
}

func stashFileKey(hash, path string) string { return hash + "\x00" + path }

func (a *App) loadStashPanel() {
	s := &a.git.stash
	s.seq++
	s.loading, s.loadedFor = true, a.git.root
	seq, gen, ctx, root := s.seq, a.gen, a.wsCtx, a.git.root
	go func() {
		entries, err := git.Stashes(ctx, root)
		a.post.Post(func() {
			if a.gen != gen || s.seq != seq {
				return
			}
			s.loading, s.entries, s.err = false, entries, ""
			s.files = nil // a dropped or popped stash leaves no files behind
			if err != nil {
				s.err = gitErrLine(err)
			}
		})
	}()
}

func (a *App) toggleStash(hash string) {
	s := &a.git.stash
	if s.open == nil {
		s.open = map[string]bool{}
	}
	s.open[hash] = !s.open[hash]
	if !s.open[hash] {
		return
	}
	if _, ok := s.files[hash]; ok {
		return
	}
	gen, ctx, root := a.gen, a.wsCtx, a.git.root
	go func() {
		changed, untracked, err := git.StashFiles(ctx, root, hash)
		a.post.Post(func() {
			if a.gen != gen {
				return
			}
			if s.files == nil {
				s.files = map[string]stashFiles{}
			}
			f := stashFiles{changed: changed, untracked: untracked}
			if err != nil {
				f.err = gitErrLine(err)
			}
			s.files[hash] = f
		})
	}()
}

// selectedStash is the stash of the selected row, and the file when the
// row is a file's.
func (a *App) selectedStash() (*git.StashEntry, *git.FileStat, bool) {
	s := &a.git.stash
	hash, path, isFile := strings.Cut(s.selected, "\x00")
	for i := range s.entries {
		if s.entries[i].Hash != hash {
			continue
		}
		if !isFile {
			return &s.entries[i], nil, false
		}
		f := s.files[hash]
		for j := range f.changed {
			if f.changed[j].Path == path {
				return &s.entries[i], &f.changed[j], false
			}
		}
		for j := range f.untracked {
			if f.untracked[j].Path == path {
				return &s.entries[i], &f.untracked[j], true
			}
		}
	}
	return nil, nil, false
}

// openStashFile shows a stashed file's diff against the commit the stash
// was made on; an untracked file's against nothing.
func (a *App) openStashFile(st git.StashEntry, f git.FileStat, untracked bool) {
	cm := git.Commit{Hash: st.Hash, Short: st.Ref}
	if untracked {
		cm.Hash += "^3"
	}
	a.openCommitFile(a.git.root, a.focus, cm, f)
}

// stashView is the Stash tab: its toolbar, then the stashes.
func (a *App) stashView(c *ui.Context) {
	s := &a.git.stash
	if s.loadedFor != a.git.root && !s.loading {
		a.loadStashPanel()
	}
	pal := theme.Of(c)
	st, file, untracked := a.selectedStash()
	ui.ScrollHorizontal(c).FillWidth().Height(editorTabBarHeight).BorderWidth(0, 0, 1, 0).BorderColor(pal.Hairline).Padding(0, sidebarInset).Gap(toolWindowActionGap).AlignItems(ui.Center).Children(func() {
		if toolWindowAction(c, glyph.Refresh, locales.GitRefreshStashes(), locales.GitRefreshStashes(), 0, s.loading).Clicked() {
			a.loadStashPanel()
		}
		if toolWindowAction(c, glyph.ShowDiff, locales.CommitShowDiff(), locales.CommitShowDiff(), 0, file == nil).Clicked() {
			a.openStashFile(*st, *file, untracked)
		}
		ui.Box(c).Size(1, 14).Background(pal.Separator)
		if toolWindowAction(c, glyph.Unstash, locales.GitApply(), locales.GitApply(), 0, st == nil || a.git.busy).Clicked() {
			a.applyStash(st.Ref, false, false)
		}
		if toolWindowAction(c, glyph.Minus, locales.GitDrop(), locales.GitDrop(), 0, st == nil || a.git.busy).Clicked() {
			a.dropStash(st.Ref, nil)
		}
	})
	switch {
	case s.err != "":
		ui.Text(c, s.err).TextColor(pal.Muted).Padding(12, 10)
		return
	case len(s.entries) == 0 && s.loading:
		ui.Text(c, locales.MsgLoading()).TextColor(pal.Muted).Padding(12, 10)
		return
	case len(s.entries) == 0:
		ui.Text(c, locales.GitNoStashes()).TextColor(pal.Muted).Padding(12, 10)
		return
	}
	ui.Scroll(c.Key("stash-tree")).Grow(1).FillWidth().Children(func() {
		ui.Column(c).FillWidth().Padding(4, 0, 6, 0).Children(func() {
			for _, entry := range s.entries {
				a.stashRow(c, entry)
				if !s.open[entry.Hash] {
					continue
				}
				f, ok := s.files[entry.Hash]
				switch {
				case !ok:
					ui.Text(c, locales.MsgLoading()).TextColor(pal.Dim).Padding(2, 8, 2, sidebarInset+32)
				case f.err != "":
					ui.Text(c, f.err).TextColor(pal.Dim).Padding(2, 8, 2, sidebarInset+32)
				}
				for _, fs := range f.changed {
					a.stashFileRow(c, entry, fs, false)
				}
				for _, fs := range f.untracked {
					a.stashFileRow(c, entry, fs, true)
				}
			}
		})
	})
}

func (a *App) stashRow(c *ui.Context, entry git.StashEntry) {
	s := &a.git.stash
	pal := theme.Of(c)
	title := entry.Message
	if title == "" {
		title = entry.Ref
	}
	row := ui.Row(c.Key(entry.Hash)).FillWidth().Radius(4).MinHeight(23).Padding(0, 8, 0, sidebarInset).Gap(4).AlignItems(ui.Center).Tooltip(entry.Ref)
	if s.selected == entry.Hash {
		row.Background(a.git.selectionColor(c))
	} else if row.Hovered() {
		row.Background(pal.Pill)
	}
	row.ContextMenu(func(m *ui.Menu) {
		s.selected = entry.Hash
		a.stashMenu(m, entry)
	})
	row.Children(func() {
		arrow := ui.ButtonBase(c).Size(12, 20).Center().KeepFocus().Label(title + " ▸")
		arrow.Children(func() {
			icon := glyph.Draw(c, glyph.Chevron, 9, pal.Muted)
			if s.open[entry.Hash] {
				icon.Rotate(90)
			}
		})
		arrow.OnClick(func() { a.toggleStash(entry.Hash) })
		b := ui.ButtonBase(c).Grow(1).Shrink(1).KeepFocus().Label(title)
		b.Children(func() {
			ui.Row(c).FillWidth().Gap(6).AlignItems(ui.Center).Children(func() {
				ui.Text(c, title).SingleLine().Ellipsis("…").Shrink(1)
				if entry.Branch != "" {
					ui.Text(c, entry.Branch).TextColor(pal.Dim).SingleLine().Shrink(2)
				}
				ui.Box(c).Grow(1)
				ui.Text(c, shortDate(entry.Date, time.Now())).FontSize(widget.FontSize(c, 11)).TextColor(pal.Dim).SingleLine().Shrink(0)
			})
		})
		if b.Clicked() {
			s.selected = entry.Hash
		}
		if b.DoubleClicked() {
			a.toggleStash(entry.Hash)
		}
	})
}

func (a *App) stashFileRow(c *ui.Context, entry git.StashEntry, f git.FileStat, untracked bool) {
	s := &a.git.stash
	pal := theme.Of(c)
	key := stashFileKey(entry.Hash, f.Path)
	status := f.Status // an untracked file comes back as added
	color := pal.GitFileColor(status)
	row := ui.Row(c.Key(key)).FillWidth().Radius(4).Padding(1, 8, 1, sidebarInset+32).Gap(6).AlignItems(ui.Center)
	if s.selected == key {
		row.Background(a.git.selectionColor(c))
		color = pal.Text
	} else if row.Hovered() {
		row.Background(pal.Pill)
	}
	row.ContextMenu(func(m *ui.Menu) {
		s.selected = key
		if m.Item(locales.CommitShowDiff()).Chosen() {
			a.openStashFile(entry, f, untracked)
		}
		m.Separator()
		a.stashMenu(m, entry)
	})
	row.Children(func() {
		b := ui.ButtonBase(c).Grow(1).Shrink(1).Padding(4, 0).KeepFocus().Label(locales.CommitOpenDiff() + " " + f.Path).Tooltip(f.Path)
		b.Children(func() {
			ui.Row(c).FillWidth().Gap(6).AlignItems(ui.Center).Children(func() {
				glyph.File(c, filepath.Base(f.Path), 15)
				ui.Text(c, filepath.Base(f.Path)).TextColor(color).SingleLine().Shrink(1)
				if dir := filepath.Dir(f.Path); dir != "." {
					ui.Text(c, filepath.ToSlash(dir)).TextColor(pal.Dim).SingleLine().Shrink(1)
				}
			})
		})
		if b.Clicked() {
			s.selected = key
			a.openStashFile(entry, f, untracked)
		}
	})
}

// stashMenu is a stash's menu, as IntelliJ's Stash tab.
func (a *App) stashMenu(m *ui.Menu, entry git.StashEntry) {
	if m.Item(locales.GitApply()).Disabled(a.git.busy).Chosen() {
		a.applyStash(entry.Ref, false, false)
	}
	if m.Item(locales.GitPop()).Disabled(a.git.busy).Chosen() {
		a.applyStash(entry.Ref, true, false)
	}
	if m.Item(locales.GitUnstashAsBranch() + "…").Disabled(a.git.busy).Chosen() {
		a.unstashAsBranch(entry.Ref)
	}
	m.Separator()
	if m.Item(locales.GitDrop()).Disabled(a.git.busy).Chosen() {
		a.dropStash(entry.Ref, nil)
	}
}
