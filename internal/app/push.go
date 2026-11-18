// INPUT: git.PushPlan 与多仓库。
// OUTPUT: Push 对话框：仓库行、目标分支编辑、待推提交与文件 diff。
// POS: IntelliJ Push 对话框的对应实现。
// PROTOCOL: 变更时更新此头部，然后检查本包 doc.go

package app

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"revtrail/internal/git"
	"revtrail/internal/glyph"
	"revtrail/internal/theme"
	"revtrail/internal/widget"
	"revtrail/locales"
)

const pushPage = 100

type pushRepository struct {
	git.PushPlan
	checked, folded, pushed bool
	limit                   int
	err                     string
}
type pushWindow struct {
	repos                      []*pushRepository
	repo                       *pushRepository
	commit                     git.Commit
	detail                     *commitView
	diff                       *editor.DiffState
	file                       string
	loading, busy, hooks, tags bool
	tagMode                    string
	edit                       *pushRepository
	remote, target, url        string
	err                        string
	split, detailSplit         float32
	hideDetails, editAll       bool
	folders                    map[string]bool
}

func (a *App) reviewPush() { a.openPush("") }

func (a *App) openPush(preselect string) { a.openBranchPush(preselect, "") }

// openBranchPush opens the push window for preselect, with branch pushed
// from it: the current branch when "", as IntelliJ's Push… on a branch
// that is not checked out.
func (a *App) openBranchPush(preselect, branch string) {
	if a.git.push != nil || a.git.root == "" || a.git.busy && !a.git.autoFetching {
		return
	}
	w := &pushWindow{loading: true, hooks: true, tagMode: "all", split: 56, detailSplit: 67, folders: map[string]bool{}}
	a.git.push = w
	ctx, dir := a.wsCtx, a.root
	go func() {
		roots := git.Roots(ctx, dir)
		var repos []*pushRepository
		for _, r := range roots {
			b := ""
			if r == preselect {
				b = branch
			}
			p, err := git.ReadPushPlan(ctx, r, b, "", "", pushPage)
			row := &pushRepository{PushPlan: p, limit: pushPage}
			if err != nil {
				row.err = gitErrLine(err)
			}
			row.checked = err == nil && p.Head != "" && (preselect == "" || r == preselect) && (len(p.Commits) > 0 || p.Expected == "")
			repos = append(repos, row)
		}
		a.post.Post(func() {
			if a.git.push != w {
				return
			}
			w.repos, w.loading = repos, false
			a.git.roots = roots
			for _, r := range repos {
				if r.checked && len(r.Commits) > 0 {
					a.selectPushCommit(w, r, r.Commits[0])
					break
				}
			}
		})
	}()
}

func (a *App) selectPushCommit(w *pushWindow, r *pushRepository, cm git.Commit) {
	w.repo, w.commit, w.diff, w.file = r, cm, nil, ""
	w.folders = map[string]bool{}
	w.detail = a.readCommitViewAt(r.Root, cm)
}

func (a *App) editPushTarget(w *pushWindow, r *pushRepository) {
	w.editAll = false
	w.edit, w.remote, w.target, w.url, w.err = r, r.Remote, r.Target, "", ""
	if w.remote == "" {
		w.remote = "origin"
	}
}

func (a *App) reloadPushRow(w *pushWindow, r *pushRepository, remote, target, url string) {
	w.busy = true
	ctx, root, branch, limit := a.wsCtx, r.Root, r.Branch, r.limit
	go func() {
		var err error
		if url != "" {
			err = git.AddBranchRemote(ctx, root, target, remote, url)
		}
		var p git.PushPlan
		if err == nil {
			p, err = git.ReadPushPlan(ctx, root, branch, remote, target, limit)
		}
		a.post.Post(func() {
			if a.git.push != w {
				return
			}
			w.busy = false
			if err != nil {
				w.err = gitErrLine(err)
				return
			}
			r.PushPlan = p
			r.err = ""
			r.checked = p.Head != "" && (len(p.Commits) > 0 || p.Expected == "")
			w.edit, w.err = nil, ""
			if len(p.Commits) > 0 {
				a.selectPushCommit(w, r, p.Commits[0])
			} else {
				w.repo, w.detail, w.diff = r, nil, nil
			}
		})
	}()
}

func (w *pushWindow) ready() bool {
	any := false
	for _, r := range w.repos {
		if r.checked {
			if r.Remote == "" || r.err != "" || r.Head == "" {
				return false
			}
			any = true
		}
	}
	return any && !w.loading && !w.busy && w.edit == nil && !w.editAll
}

func (a *App) submitPush(w *pushWindow, force bool) {
	if !w.ready() || a.git.busy {
		return
	}
	var rows []*pushRepository
	for _, r := range w.repos {
		if r.checked {
			rows = append(rows, r)
		}
	}
	ctx, hooks, tags := a.wsCtx, w.hooks, ""
	if w.tags {
		tags = w.tagMode
	}
	w.busy, a.git.busy = true, true
	w.err = ""
	go func() {
		var failures []string
		var done []*pushRepository
		for _, r := range rows {
			if ctx.Err() != nil {
				break
			}
			if err := r.Push(ctx, tags, hooks, force); err != nil {
				failures = append(failures, filepath.Base(r.Root)+": "+gitErrLine(err))
			} else {
				done = append(done, r)
			}
		}
		a.post.Post(func() {
			if a.git.push != w {
				return
			}
			for _, r := range done {
				r.checked, r.pushed = false, true
			}
			w.busy, a.git.busy = false, false
			if len(failures) == 0 {
				a.git.push = nil
				a.note(fmt.Sprintf(locales.PushComplete(), len(done)), false)
			} else {
				w.err = strings.Join(failures, "\n")
			}
			a.refreshGit()
		})
	}()
}

func (a *App) pushPanel(c *ui.Context, w *pushWindow) {
	width, height := c.Size()
	pal := theme.Of(c)
	ui.Overlay(c, func() {
		back := ui.Box(c).Absolute().Left(0).Right(0).Top(0).Bottom(0).Center().Modal().Background(theme.Shade(.16))
		close := func() {
			if w.busy {
				return
			}
			if w.edit != nil || w.editAll {
				w.editAll = false
				w.edit = nil
				w.err = ""
			} else if w.diff != nil {
				w.diff = nil
				w.file = ""
			} else {
				a.git.push = nil
			}
		}
		if back.OverlayShortcut(0, ui.KeyEscape) && a.actionConfirm == nil {
			close()
		}
		back.Children(func() {
			ui.Column(c).Width(min(880, width-24)).Height(min(560, height-32)).Radius(9).Clip().Border(1, pal.PopoverEdge).Background(pal.Toolbar).Shadow(0, 8, 26, 0, theme.Shade(.30)).Label(locales.PushReview()).Children(func() {
				ui.Text(c, locales.PushReview()).Bold().Padding(9, 14)
				widget.SplitPanes(c.Key("push-panes"), ui.SplitPercent, &w.split, func() {
					ui.Column(c).Fill().Children(func() {
						ui.Scroll(c.Key("repositories")).FillWidth().Grow(1).Children(func() {
							ui.Column(c).FillWidth().Padding(4).Children(func() {
								if w.loading {
									ui.Text(c, locales.MsgLoading())
								} else if len(w.repos) == 0 {
									ui.Text(c, locales.MsgThisFolderIsNotInAGitRepository())
								}
								for _, r := range w.repos {
									a.pushRepoRow(c, w, r)
								}
							})
						})
						if len(w.repos) > 1 {
							ui.Row(c).FillWidth().Padding(2, 6).Justify(ui.End).Children(func() {
								ui.ButtonBase(c).Label(locales.PushEditAll()).Disabled(w.busy).OnClick(func() { w.edit = nil; w.editAll = true; w.target = ""; w.err = "" }).Children(func() { ui.Text(c, locales.PushEditAll()).TextColor(c.Theme().Accent) })
							})
						}
					})
				}, func() { a.pushDetails(c, w) }).Grow(1).FillWidth()
				if w.editAll {
					a.pushAllTargetsEditor(c, w)
				} else if w.edit != nil {
					a.pushTargetEditor(c, w)
				}
				if w.err != "" {
					ui.Text(c, w.err).FillWidth().MaxLines(3).Tooltip(w.err).TextColor(pal.Err).Padding(4, 10)
				}
				ui.Row(c).FillWidth().Padding(8, 10).Gap(6).AlignItems(ui.Center).BorderWidth(1, 0, 0, 0).BorderColor(pal.Separator).Children(func() {
					ui.Checkbox(c, &w.tags, locales.PushTags()).Disabled(w.busy)
					mode := locales.PushTagsAll()
					if w.tagMode == "follow" {
						mode = locales.PushTagsReachable()
					}
					ui.Button(c, mode+" ▾").Disabled(!w.tags || w.busy).Menu(func(m *ui.Menu) {
						if m.Item(locales.PushTagsAll()).Chosen() {
							w.tagMode = "all"
						}
						if m.Item(locales.PushTagsReachable()).Chosen() {
							w.tagMode = "follow"
						}
					})
					ui.Checkbox(c, &w.hooks, locales.PushHooks()).Disabled(w.busy)
					ui.Box(c).Grow(1)
					ui.Button(c, locales.Cancel()).Disabled(w.busy).OnClick(func() { a.git.push = nil })
					ui.PrimaryButton(c, locales.CommitPushButton()).Disabled(!w.ready() || a.git.busy).OnClick(func() { a.submitPush(w, false) })
					ui.Button(c, "▾").Label(locales.ForcePush()).Disabled(!w.ready() || a.git.busy).Menu(func(m *ui.Menu) {
						if m.Item(locales.ForcePush()).Chosen() {
							a.actionConfirm = &confirmation{message: locales.ForcePushQuestion(), actions: []action{{locales.ForcePush(), func() { a.actionConfirm = nil; a.submitPush(w, true) }}, {locales.Cancel(), func() { a.actionConfirm = nil }}}}
						}
					})
				})
			})
		})
	})
}

func (a *App) pushRepoRow(c *ui.Context, w *pushWindow, r *pushRepository) {
	pal := theme.Of(c)
	name := a.repoLabel(r.Root)
	eligible := r.err == "" && r.Head != "" && !r.pushed
	ui.Row(c.Key(r)).FillWidth().Height(26).Gap(5).AlignItems(ui.Center).Children(func() {
		if len(r.Commits) > 0 {
			b := ui.ButtonBase(c).Size(13, 22).Center().Label(name + " ▸")
			b.Children(func() {
				i := glyph.Draw(c, glyph.Chevron, 9, pal.Muted)
				if !r.folded {
					i.Rotate(90)
				}
			})
			b.OnClick(func() { r.folded = !r.folded })
		} else {
			ui.Box(c).Width(13)
		}
		ui.Checkbox(c, &r.checked, "").Label(locales.PushInclude() + " " + name).Disabled(!eligible || w.busy)
		tint := pal.Text
		if !eligible || len(r.Commits) == 0 && !r.checked {
			tint = pal.Muted
		}
		b := ui.ButtonBase(c).Width(112).MinWidth(24).Shrink(1).Justify(ui.Start).Label(name).Tooltip(r.Root)
		b.Children(func() { ui.Text(c, name).TextColor(tint).SingleLine().Ellipsis("…") })
		b.OnClick(func() {
			w.repo = r
			w.detail = nil
			w.diff = nil
			if len(r.Commits) > 0 {
				a.selectPushCommit(w, r, r.Commits[0])
			}
		})
		if r.err != "" {
			ui.Text(c, r.err).SingleLine().Ellipsis("…").Tooltip(r.err).TextColor(pal.Muted)
			return
		}
		ui.Text(c, r.Branch+" →").TextColor(tint).SingleLine()
		label := r.Remote + " : " + r.Target
		if r.Remote == "" {
			label = locales.DefineRemote()
		}
		t := ui.ButtonBase(c).Shrink(1).Label(name + " → " + label).Disabled(!eligible || w.busy)
		t.Children(func() { ui.Text(c, label).SingleLine().Ellipsis("…").TextColor(c.Theme().Accent) })
		t.OnClick(func() { a.editPushTarget(w, r) })
		status := ""
		switch {
		case r.pushed:
			status = locales.PushFinished()
		case r.Head == "":
			status = locales.PushNoCommits()
		case r.Behind:
			status = locales.PushBehind()
		case r.Remote != "" && r.Expected == "":
			status = locales.PushNewBranch()
		case len(r.Commits) == 0:
			status = locales.PushUpToDate()
		}
		if status != "" {
			ui.Text(c, status).SingleLine().Ellipsis("…").Shrink(1).TextColor(pal.Muted).Tooltip(status)
		}
	})
	if r.folded {
		return
	}
	for _, cm := range r.Commits {
		b := ui.ButtonBase(c.Key(r.Root+cm.Hash)).FillWidth().Height(21).Padding(0, 5, 0, 38).Justify(ui.Start).Radius(3).Label(cm.Subject).Tooltip(cm.Subject)
		if w.repo == r && w.commit.Hash == cm.Hash && w.detail != nil {
			b.Background(pal.ListSelection)
		} else if b.Hovered() {
			b.Background(pal.Pill)
		}
		b.Children(func() { ui.Text(c, cm.Subject).SingleLine().Ellipsis("…") })
		b.OnClick(func() { a.selectPushCommit(w, r, cm) })
	}
	if r.More && !r.pushed {
		ui.Button(c, locales.PushShowMore()).Disabled(w.busy).OnClick(func() { r.limit += pushPage; a.reloadPushRow(w, r, r.Remote, r.Target, "") })
	}
}

func (a *App) pushTargetEditor(c *ui.Context, w *pushWindow) {
	r := w.edit
	ui.Row(c.Key(r.Root)).FillWidth().Padding(6, 10).Gap(6).AlignItems(ui.Center).Children(func() {
		ui.Text(c, a.repoLabel(r.Root)).Bold()
		if len(r.Remotes) == 0 {
			ui.TextInput(c, &w.remote).Width(75).Label(locales.PushRemote())
			ui.TextInput(c, &w.url).Placeholder(locales.PushRemoteURL()).Label(locales.PushRemoteURL()).Grow(1)
		} else {
			ui.Button(c, w.remote+" ▾").Menu(func(m *ui.Menu) {
				for _, n := range r.Remotes {
					if m.Item(n).Checked(n == w.remote).Chosen() {
						w.remote = n
					}
				}
			})
		}
		ui.TextInput(c, &w.target).Grow(1).Placeholder(locales.PushTarget()).Label(locales.PushTarget())
		ui.Button(c, locales.Cancel()).Disabled(w.busy).OnClick(func() { w.edit = nil; w.err = "" })
		ui.Button(c, locales.PushApply()).Disabled(w.busy || strings.TrimSpace(w.target) == "" || w.remote == "" || len(r.Remotes) == 0 && strings.TrimSpace(w.url) == "").OnClick(func() { a.reloadPushRow(w, r, w.remote, strings.TrimSpace(w.target), strings.TrimSpace(w.url)) })
	})
}

func (a *App) pushDetails(c *ui.Context, w *pushWindow) {
	pal := theme.Of(c)
	ui.Column(c).Fill().Clip().Children(func() {
		v := w.detail
		if v == nil {
			logWindowHint(c, locales.GitWindowSelect())
			return
		}
		if v.loading {
			logWindowHint(c, locales.MsgLoading())
			return
		}
		if v.err != "" {
			logWindowHint(c, v.err)
			return
		}
		if w.diff != nil {
			ui.Button(c, "‹ "+w.file).Label(locales.GitWindowDetails()).OnClick(func() { w.diff = nil; w.file = "" })
			editor.DiffView(c, w.diff).Grow(1).FillWidth()
			return
		}
		ui.Row(c).FillWidth().Height(28).Padding(0, 5).AlignItems(ui.Center).Children(func() {
			toolWindowAction(c, glyph.CommitView, locales.GitWindowDetails(), locales.GitWindowDetails(), 0, false).OnClick(func() { w.hideDetails = !w.hideDetails })
			ui.Box(c).Grow(1)
			toolWindowAction(c, glyph.ExpandChanges, locales.CommitExpandAll(), locales.CommitExpandAll(), 0, false).OnClick(func() { clear(w.folders) })
			toolWindowAction(c, glyph.CollapseChanges, locales.CommitCollapseAll(), locales.CommitCollapseAll(), 0, false).OnClick(func() {
				for _, f := range v.details.Files {
					if name, _, has := strings.Cut(f.Path, "/"); has {
						w.folders[name] = true
					}
				}
			})
		})
		files := func() {
			ui.Scroll(c.Key("push-files:" + v.commit.Hash)).Fill().Children(func() {
				ui.Column(c).FillWidth().Padding(6).Children(func() {
					ui.Text(c, a.repoLabel(w.repo.Root)+"   "+locales.MsgDFileS(len(v.details.Files))).Bold().Padding(3)
					seen := map[string]bool{}
					for _, f := range v.details.Files {
						parts := strings.Split(f.Path, "/")
						hidden := false
						for i, n := range parts[:len(parts)-1] {
							dir := strings.Join(parts[:i+1], "/")
							if !seen[dir] {
								seen[dir] = true
								b := ui.ButtonBase(c.Key(dir)).FillWidth().Height(22).Padding(0, 4, 0, float32(10+i*13)).Gap(5).AlignItems(ui.Center).Justify(ui.Start).Label(dir)
								b.Children(func() {
									ic := glyph.Draw(c, glyph.Chevron, 9, pal.Muted)
									if !w.folders[dir] {
										ic.Rotate(90)
									}
									glyph.Draw(c, glyph.WelcomeFolder, 13, pal.Muted)
									ui.Text(c, n)
								})
								b.OnClick(func() { w.folders[dir] = !w.folders[dir] })
							}
							if w.folders[dir] {
								hidden = true
								break
							}
						}
						if hidden {
							continue
						}
						b := ui.ButtonBase(c.Key(f.Path)).FillWidth().Height(22).Padding(0, 4, 0, float32(18+(len(parts)-1)*13)).Gap(5).AlignItems(ui.Center).Justify(ui.Start).Label(f.Path)
						b.Children(func() {
							glyph.File(c, filepath.Base(f.Path), 14)
							ui.Text(c, filepath.Base(f.Path)).SingleLine().Ellipsis("…")
						})
						b.OnClick(func() { a.pushFileDiff(w, f) })
					}
				})
			})
		}
		details := func() {
			ui.Scroll(c.Key("push-message")).Fill().Children(func() {
				ui.Column(c).FillWidth().Padding(10).Gap(6).BorderWidth(1, 0, 0, 0).BorderColor(pal.Separator).Children(func() {
					ui.Text(c, v.details.Body).Selectable()
					ui.Text(c, fmt.Sprintf("%s · %s <%s> · %s", v.commit.Short, v.commit.Author, v.commit.Email, v.commit.Date.Format("2006-01-02 15:04"))).TextColor(pal.Muted).Selectable()
				})
			})
		}
		if w.hideDetails {
			files()
		} else {
			widget.SplitPanes(c.Key("push-details"), ui.SplitVerticalPercent, &w.detailSplit, files, details).FillWidth().Grow(1)
		}

	})
}
func (a *App) pushFileDiff(w *pushWindow, f git.FileStat) {
	ctx, r, cm := a.wsCtx, w.repo, w.commit
	w.file = f.Path
	go func() {
		raw, err := git.CommitFileDiff(ctx, r.Root, cm.Hash, f)
		patches, parseErr := editor.ParsePatch(string(raw))
		a.post.Post(func() {
			if a.git.push != w || w.repo != r || w.commit.Hash != cm.Hash || w.file != f.Path {
				return
			}
			if err != nil {
				w.err = gitErrLine(err)
				return
			}
			if parseErr != nil || len(patches) == 0 || patches[0].Binary {
				w.err = locales.MsgNoTextDiffAvailableFor() + f.Path
				return
			}
			w.diff = a.newGitDiffTab(patches[0], f.Path, false).state
		})
	}()
}

func (a *App) pushAllTargetsEditor(c *ui.Context, w *pushWindow) {
	ui.Row(c).FillWidth().Padding(6, 10).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Text(c, locales.PushAllTargets())
		ui.TextInput(c, &w.target).Grow(1).Label(locales.PushTarget())
		ui.Button(c, locales.Cancel()).Disabled(w.busy).OnClick(func() { w.editAll = false; w.err = "" })
		ui.Button(c, locales.PushApply()).Disabled(w.busy || strings.TrimSpace(w.target) == "").OnClick(func() {
			ctx, target := a.wsCtx, strings.TrimSpace(w.target)
			w.busy = true
			go func() {
				plans := map[*pushRepository]git.PushPlan{}
				var fail string
				for _, r := range w.repos {
					if r.Head == "" || r.Remote == "" || r.pushed {
						continue
					}
					p, err := git.ReadPushPlan(ctx, r.Root, r.Branch, r.Remote, target, r.limit)
					if err != nil {
						fail = filepath.Base(r.Root) + ": " + gitErrLine(err)
						break
					}
					plans[r] = p
				}
				a.post.Post(func() {
					if a.git.push != w {
						return
					}
					w.busy = false
					w.err = fail
					if fail != "" {
						return
					}
					for r, p := range plans {
						r.PushPlan = p
						r.err = ""
					}
					w.editAll = false
					w.detail, w.diff = nil, nil
					if r := w.repo; r != nil && len(r.Commits) > 0 {
						a.selectPushCommit(w, r, r.Commits[0])
					}
				})
			}()
		})
	})
}
