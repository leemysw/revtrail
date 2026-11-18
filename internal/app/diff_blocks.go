// INPUT: git 工作区差异块与 BlockSelection。
// OUTPUT: 工作区 diff 视图的块勾选与块回滚确认。
// POS: 按块提交 / 回滚的界面层。
// PROTOCOL: 变更时更新此头部，然后检查本包 doc.go

package app

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"revtrail/internal/git"
	"revtrail/internal/glyph"
	"revtrail/internal/theme"
	"revtrail/internal/widget"
	"revtrail/internal/workspace"
	"revtrail/locales"
)

func (a *App) workingDiffView(c *ui.Context, v *gitDiffTab) ui.Element {
	if v.hash != "" || v.rev != "" || v.staged || len(v.hunks) == 0 || v.digest == "" {
		return editor.DiffView(c, v.state).Grow(1).FillWidth()
	}
	var view ui.Element
	ui.Column(c).Grow(1).FillWidth().Children(func() {
		i := slices.IndexFunc(a.git.changes, func(ch git.Change) bool { return ch.Root == v.root && ch.Path == v.name })
		current := i >= 0
		ch := git.Change{Root: v.root, Path: v.name}
		if current {
			ch = a.git.changes[i]
		}
		n := len(v.hunks)
		sel := a.git.blocks[changeID(ch)]
		stale := current && sel.Mask != "" && (sel.Digest != v.digest || len(sel.Mask) != n)
		mask := sel.Mask
		if !current || stale || mask == "" {
			bit := "0"
			if current && !stale && a.git.isIncluded(ch) {
				bit = "1"
			}
			mask = strings.Repeat(bit, n)
		}
		ui.Row(c).FillWidth().Height(25).Padding(0, 8).Gap(8).AlignItems(ui.Center).Background(theme.Of(c).Toolbar).Children(func() {
			ui.Text(c, workspace.RelPath(a.realRoot, filepath.Join(v.root, v.name))).SingleLine().Ellipsis("…").Grow(1).MinWidth(0).TextColor(theme.Of(c).Muted)
			switch {
			case stale:
				ui.Button(c, locales.DiffReviewAgain()).OnClick(func() {
					delete(a.git.blocks, changeID(ch))
					a.git.included[changeID(ch)] = false
					a.showGitDiff(ch, false)
				})
			case current:
				all := strings.Count(mask, "1") == n
				box := ui.Checkbox(c, &all, locales.DiffCurrentVersion()).Disabled(a.git.busy || ch.Status() == '!')
				if sel.Mask != "" {
					box.Mixed()
				}
				box.OnClick(func() { a.toggleIncluded([]git.Change{ch}) })
				ui.Text(c, fmt.Sprintf(locales.DiffBlockCount(), n, strings.Count(mask, "1"))).TextColor(theme.Of(c).Muted)
			}
		})
		box := ui.Box(c).Grow(1).FillWidth().Clip()
		box.Children(func() {
			view = editor.DiffView(c, v.state).Fill()
			if !current || ch.Status() == '!' || a.git.busy {
				return
			}
			share := v.state.SplitPercent
			if share == 0 {
				share = 50
			}
			for index, h := range v.hunks {
				y := v.state.LineTop(editor.DiffNew, h.NewStart)
				if h.OldEnd > h.OldStart {
					y = min(y, v.state.LineTop(editor.DiffOld, h.OldStart))
				}
				y -= v.state.ScrollY()
				if y < 0 || y >= v.state.ViewportHeight() {
					continue
				}
				row := ui.Row(c.Key(index)).Absolute().Top(float32(y)).Gap(1).AlignItems(ui.Center).Background(theme.Of(c).TabBar).Radius(2)
				if v.state.Layout == editor.DiffSplit {
					row.RightPercent(100 - share)
				} else {
					row.Right(12)
				}
				row.Children(func() {
					// A rename, creation or deletion also changes the directory entry.
					// Whole-file rollback owns those operations.
					if v.patch.OldName == v.name && v.patch.NewName == v.name {
						tip := fmt.Sprintf(locales.DiffRollbackBlock(), index+1)
						widget.GlyphButton(c, glyph.Forward, tip, tip, false, false).OnClick(func() { a.confirmBlockRollback(v, index) })
					}
					tip := fmt.Sprintf(locales.DiffIncludeBlock(), index+1)
					checked := mask[index] == '1'
					ui.Checkbox(c, &checked, "").Label(tip).Tooltip(tip).KeepFocus().OnClick(func() { a.toggleBlock(ch, v, index, mask) })
				})
			}
		})
	})
	return view
}

func (a *App) toggleBlock(ch git.Change, v *gitDiffTab, index int, mask string) {
	bits := []byte(mask)
	if bits[index] == '1' {
		bits[index] = '0'
	} else {
		bits[index] = '1'
	}
	selection := string(bits)
	if a.git.included == nil {
		a.git.included = map[string]bool{}
	}
	count := strings.Count(selection, "1")
	id := changeID(ch)
	a.git.included[id] = count > 0
	if count == 0 || count == len(bits) {
		delete(a.git.blocks, id)
	} else {
		if a.git.blocks == nil {
			a.git.blocks = map[string]git.BlockSelection{}
		}
		a.git.blocks[id] = git.BlockSelection{Digest: v.digest, Mask: selection}
	}
	a.saveConfigSoon()
}

func (a *App) confirmBlockRollback(v *gitDiffTab, index int) {
	a.actionConfirm = &confirmation{message: fmt.Sprintf(locales.DiffRollbackQuestion(), v.name), actions: []action{
		{locales.DiscardChanges(), func() {
			a.actionConfirm = nil
			path := workspace.DocPath(a.root, a.realRoot, filepath.Join(v.root, v.name))
			if d := a.docs[path]; d != nil && d.buf != nil && d.buf.Modified() {
				a.note(locales.MsgSaveOrCloseTheFileSUnsavedEdits(), true)
				return
			}
			a.git.busy = true
			ctx, gen := a.wsCtx, a.gen
			go func() {
				err := git.RollbackBlock(ctx, v.root, v.name, v.digest, index)
				var raw []byte
				var patches []editor.FilePatch
				if err == nil {
					raw, err = git.WorkingFileDiff(ctx, v.root, v.name, v.oldPath)
					if err == nil {
						patches, err = editor.ParsePatch(string(raw))
					}
				}
				a.post.Post(func() {
					if a.gen != gen {
						return
					}
					a.git.busy = false
					a.refreshGit()
					if err != nil {
						a.note(locales.MsgCouldNotDiscardGitChanges()+err.Error(), true)
						return
					}
					if d := a.docs[path]; d != nil {
						a.reloadDoc(d)
					}
					key := a.repoTabKey(v.root, diffTab(v.name, false))
					remaining := 0
					for _, group := range a.groups() {
						if group.diffs[key] == nil {
							continue
						}
						if len(patches) == 0 {
							a.closeTab(group, key)
							continue
						}
						nv := a.newGitDiffTab(patches[0], v.name, false)
						nv.root, nv.oldPath, nv.digest = v.root, v.oldPath, git.PatchDigest(raw)
						group.diffs[key] = nv
						remaining = len(nv.hunks)
					}
					id := changeID(git.Change{Root: v.root, Path: v.name})
					if sel := a.git.blocks[id]; sel.Digest == v.digest && len(sel.Mask) == len(v.hunks) {
						bits := sel.Mask[:index] + sel.Mask[index+1:]
						// Adjacent blocks may coalesce. Require a fresh choice in that case.

						a.git.included[id] = remaining == len(bits) && strings.Contains(bits, "1")
						delete(a.git.blocks, id)
						if remaining == len(bits) && strings.Contains(bits, "0") && strings.Contains(bits, "1") {
							a.git.blocks[id] = git.BlockSelection{Digest: git.PatchDigest(raw), Mask: bits}
						}
					}
				})
			}()
		}},
		{locales.Cancel(), func() { a.actionConfirm = nil }},
	}}
}
