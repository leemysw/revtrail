// INPUT: git.ReadMerge 的三方内容。
// OUTPUT: 合并冲突编辑器：冲突块、逐块决定、应用结果。
// POS: Resolve Conflicts 的界面层。
// PROTOCOL: 变更时更新此头部，然后检查本包 doc.go

package app

import (
	"bytes"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"revtrail/internal/git"
	"revtrail/internal/glyph"
	"revtrail/internal/syntax"
	"revtrail/internal/theme"
	"revtrail/internal/widget"
	"revtrail/internal/workspace"
	"revtrail/locales"
)

type mergeBlock struct {
	left, right, base string
	start, end        int
	sideStart         [2]int
	done              [2]bool
	applied           bool
}
type mergeWindow struct {
	ancestor                *editor.State
	showBase                bool
	root, path, choice, err string
	file                    *git.MergeFile
	blocks                  []mergeBlock
	views                   [3]*editor.State
	active                  int
	lastText                string
	version                 uint64
	manual, busy            bool
}

func (a *App) mergeEditor(path, text string, readOnly bool) *editor.State {
	buf := editor.NewBuffer(text)
	buf.SetLanguage(syntax.For(filepath.Base(path)))
	st := &editor.State{Buffer: buf, ReadOnly: readOnly, Theme: a.edTheme, Bindings: editorBindings()}
	a.applyEditorPrefs(st)
	return st
}

func (a *App) openMergeAt(root, path string) {
	if a.git.merge != nil || a.git.busy || !a.saveNamedFiles() {
		return
	}
	m := &mergeWindow{root: root, path: path, choice: "result"}
	a.git.merge = m
	ctx := a.wsCtx
	go func() {
		file, err := git.ReadMerge(ctx, root, path)
		a.post.Post(func() {
			if a.git.merge != m {
				return
			}
			if err != nil {
				m.err = err.Error()
				return
			}
			m.file = file
			if !file.Text {
				m.choice = ""
				return
			}
			a.initMergeText(m)
			if file.Edited {
				m.views[1] = a.mergeEditor(path, file.Working, false)
				m.manual = true
				m.remember()
			}
		})
	}()
}

func (a *App) initMergeText(m *mergeWindow) {
	path, file := m.path, m.file
	m.manual, m.active, m.blocks = false, 0, nil
	m.showBase = false
	m.views[0], m.views[2] = a.mergeEditor(path, file.Left.Text, true), a.mergeEditor(path, file.Right.Text, true)
	var text strings.Builder
	var projections [2]strings.Builder
	for _, part := range file.Parts {
		if !part.Conflict {
			common := strings.ReplaceAll(part.Common, "\r\n", "\n")
			text.WriteString(common)
			for i := range projections {
				projections[i].WriteString(common)
			}
			continue
		}
		block := mergeBlock{left: strings.ReplaceAll(part.Left, "\r\n", "\n"), right: strings.ReplaceAll(part.Right, "\r\n", "\n"), start: text.Len()}
		block.base = strings.ReplaceAll(part.Base, "\r\n", "\n")
		text.WriteString(block.base)
		block.end = text.Len()
		for side, fragment := range []string{block.left, block.right} {
			block.sideStart[side] = projections[side].Len()
			projections[side].WriteString(fragment)
		}
		m.blocks = append(m.blocks, block)
	}
	for side, col := range []int{0, 2} {
		projected := editor.NewBuffer(projections[side].String())
		source := m.views[col].Buffer
		d := editor.DiffState{Old: projected, New: source, ContextLines: -1}
		hunks := d.Hunks()
		h := 0
		delta := 0
		for i := range m.blocks {
			line := projected.Pos(m.blocks[i].sideStart[side]).Line
			for h < len(hunks) && hunks[h].OldEnd <= line {
				delta = hunks[h].NewEnd - hunks[h].OldEnd
				h++
			}
			mapped := line + delta
			if h < len(hunks) && line >= hunks[h].OldStart {
				mapped = hunks[h].NewStart
			}
			m.blocks[i].sideStart[side] = source.Offset(editor.Pos{Line: mapped})
		}
	}
	m.views[1] = a.mergeEditor(path, text.String(), false)
	if strings.Contains(file.Left.Text, "\r\n") {
		m.views[1].Buffer.SetLineEnding("\r\n")
	}
	m.remember()
	if len(m.blocks) > 0 {
		m.goTo(0)
	}
}

func (m *mergeWindow) remember() {
	b := m.views[1].Buffer
	m.version, m.lastText = b.Version(), b.String()
}

// Keep conflict ranges anchored as the user edits the result. If an edit
// crosses a boundary, the editable file becomes a single manual resolution.
func (m *mergeWindow) syncEdits() {
	b := m.views[1].Buffer
	if b.Version() == m.version {
		return
	}
	old := m.lastText
	m.remember()
	now := m.lastText
	if m.manual {
		return
	}
	lo := 0
	for lo < len(old) && lo < len(now) && old[lo] == now[lo] {
		lo++
	}
	end, next := len(old), len(now)
	for end > lo && next > lo && old[end-1] == now[next-1] {
		end--
		next--
	}
	delta := len(now) - len(old)
	for i := range m.blocks {
		c := &m.blocks[i]
		switch {
		case lo >= c.start && end <= c.end:
			c.done = [2]bool{}
			c.applied = false
			c.end += delta
		case end <= c.start:
			c.start += delta
			c.end += delta
		case lo >= c.end:
		default:
			m.manual = true
			return
		}
	}
}
func (m *mergeWindow) goTo(i int) {
	m.active = i
	b := m.blocks[i]
	m.views[0].Select(b.sideStart[0], b.sideStart[0]+len(b.left))
	m.views[1].Select(b.start, b.end)
	m.views[2].Select(b.sideStart[1], b.sideStart[1]+len(b.right))
}
func (m *mergeWindow) replaceBlock(index int, text string) {
	b := &m.blocks[index]
	delta := len(text) - (b.end - b.start)
	m.views[1].Buffer.Edit(editor.Edit{Start: b.start, End: b.end, Text: text})
	b.end = b.start + len(text)
	for i := index + 1; i < len(m.blocks); i++ {
		m.blocks[i].start += delta
		m.blocks[i].end += delta
	}
	m.remember()
	m.goTo(index)
}

func (a *App) leaveMerge(m *mergeWindow, done func()) {
	if m.busy {
		return
	}
	close := func() {
		a.git.merge = nil
		if done != nil {
			done()
		}
	}
	dirty := m.choice == "left" || m.choice == "right"
	for _, b := range m.blocks {
		dirty = dirty || b.done[0] || b.done[1]
	}
	if m.views[1] != nil {
		dirty = dirty || m.views[1].Buffer.Modified()
	}
	if !dirty {
		close()
		return
	}
	a.actionConfirm = &confirmation{message: locales.MergeDiscard(), actions: []action{
		{locales.DiscardChanges(), func() { a.actionConfirm = nil; close() }},
		{locales.Cancel(), func() { a.actionConfirm = nil }},
	}}
}

func (a *App) applyMerge(m *mergeWindow) {
	path := workspace.DocPath(a.root, a.realRoot, filepath.Join(m.root, m.path))
	if d := a.docs[path]; d != nil && d.buf != nil && d.buf.Modified() {
		m.err = locales.FileChangedBanner()
		return
	}
	result := ""
	if m.choice == "result" {
		var out bytes.Buffer
		m.views[1].Buffer.WriteTo(&out)
		result = out.String()
	}
	m.busy = true
	ctx, root, choice := a.wsCtx, m.root, m.choice
	go func() {
		err := m.file.Apply(ctx, root, choice, result)
		a.post.Post(func() {
			if a.git.merge != m {
				return
			}
			m.busy = false
			if err != nil {
				m.err = err.Error()
				if m.file.Text {
					m.choice = "result"
				}
				return
			}
			a.git.merge = nil
			if d := a.docs[path]; d != nil {
				a.reloadDoc(d)
			}
			a.refreshGit()
		})
	}()
}

func (a *App) mergePanel(c *ui.Context, m *mergeWindow) {
	ui.Overlay(c, func() {
		frame := ui.Column(c.Key(m)).Absolute().Left(8).Right(8).Top(toolbarHeight+2).Bottom(8).Modal().Radius(7).Border(1, theme.Of(c).PopoverEdge).Background(theme.Of(c).Editor)
		frame.Children(func() {
			ui.Row(c).FillWidth().Height(34).Padding(0, 10).Gap(8).AlignItems(ui.Center).Background(theme.Of(c).Toolbar).Children(func() {
				ui.Text(c, locales.MergeTitle()).Bold()
				ui.Text(c, m.path).SingleLine().Ellipsis("…").Grow(1)
				if len(m.blocks) > 0 && !m.manual {
					if widget.GlyphButton(c, glyph.Back, locales.MergePrev(), locales.MergePrev(), false, m.active == 0).Clicked() {
						m.goTo(m.active - 1)
					}
					ui.Text(c, fmt.Sprintf("%d / %d", m.active+1, len(m.blocks)))
					if widget.GlyphButton(c, glyph.Forward, locales.MergeNext(), locales.MergeNext(), false, m.active+1 >= len(m.blocks)).Clicked() {
						m.goTo(m.active + 1)
					}
				}
			})
			if m.err != "" {
				ui.Text(c, m.err).FillWidth().Wrap().TextColor(theme.Of(c).Err).Padding(6)
			}
			switch {
			case m.file == nil:
				ui.Column(c).Grow(1).Center().Children(func() {
					if m.err == "" {
						ui.Spinner(c)
					}
				})
			case !m.file.Text:
				ui.Row(c).Grow(1).FillWidth().Center().Gap(12).Children(func() {
					for i, s := range []git.MergeStage{m.file.Left, m.file.Right} {
						key, label := "left", locales.MergeYours()
						if i == 1 {
							key, label = "right", locales.MergeTheirs()
						}
						if s.Blob == "" {
							label += " · " + locales.MergeDeleted()
						}
						if m.choice == key {
							label = "✓ " + label
						}
						ui.Button(c, label).Disabled(m.busy).OnClick(func() { m.choice = key })
					}
				})
			default:
				m.syncEdits()
				views := m.views
				if m.showBase {
					views[1] = m.ancestor
				}
				var before [3]float64
				for i, st := range views {
					before[i] = st.ScrollY()
				}
				ui.Row(c).Grow(1).FillWidth().Children(func() {
					for i, label := range []string{locales.MergeYours(), locales.MergeResult(), locales.MergeTheirs()} {
						ui.Column(c.Key(i)).Grow(1).MinWidth(0).FillHeight().BorderWidth(0, 1, 0, 0).BorderColor(theme.Of(c).Hairline).Children(func() {
							ui.Row(c).FillWidth().Height(32).Padding(0, 6).Gap(4).AlignItems(ui.Center).Background(theme.Of(c).TabBar).Children(func() {
								if i == 1 && m.showBase {
									label = locales.MergeBase()
								}
								ui.Text(c, label).Bold().Grow(1)
								if i == 1 && m.file.Base.Blob != "" {
									toggle := locales.MergeBase()
									if m.showBase {
										toggle = locales.MergeResult()
									}
									ui.Button(c, toggle).OnClick(func() {
										if m.ancestor == nil {
											m.ancestor = a.mergeEditor(m.path, m.file.Base.Text, true)
										}
										m.showBase = !m.showBase
									})
								}

							})
							ui.Row(c).Grow(1).FillWidth().Children(func() {
								if !m.manual && !m.showBase && i != 0 {
									a.mergeGutter(c, m, views[i], i)
								}
								views[i].ReadOnly = i != 1 || m.showBase || m.busy
								ed := editor.View(c.Key(views[i]), views[i]).Grow(1).MinWidth(0).FillHeight()
								if !m.manual && !(m.showBase && i == 1) {
									st := views[i]
									theme := a.edTheme
									ed.DrawOver(func(p *ui.Painter, r ui.Rect) {
										p.Clip(r, 0, func() {
											for _, b := range m.blocks {
												start, end := b.start, b.end
												if i == 0 {
													start = b.sideStart[0]
													end = start + len(b.left)
												} else if i == 2 {
													start = b.sideStart[1]
													end = start + len(b.right)
												}
												line := st.Buffer.Pos(start).Line
												last := st.Buffer.Pos(max(start, end-1)).Line
												y := r.Y + float32(st.LineTop(line)-st.ScrollY())
												h := max(float32(2), float32(st.LineTop(last+1)-st.LineTop(line)))
												if y+h < r.Y || y > r.Y+r.H {
													continue
												}
												tint := theme.Deleted
												if b.resolved() || i == 0 && b.done[0] || i == 2 && b.done[1] {
													tint = theme.Added
												}
												p.Fill(ui.Rect{X: r.X, Y: y, W: r.W, H: h}, tint.Alpha(.13), 0)
												p.Fill(ui.Rect{X: r.X, Y: y, W: 2, H: h}, tint.Alpha(.85), 0)
											}
										})
									})
								}
								if !m.manual && !m.showBase && i == 0 {
									a.mergeGutter(c, m, views[i], i)
								}
							})
						})
					}
				})
				for i, st := range views {
					if delta := st.ScrollY() - before[i]; delta != 0 {
						for j, other := range views {
							if j != i {
								other.SetScrollY(before[j] + delta)
							}
						}
						break
					}
				}
			}
			ui.Row(c).FillWidth().MinHeight(38).Padding(5, 8).Gap(6).AlignItems(ui.Center).Wrap().Background(theme.Of(c).Toolbar).Children(func() {
				canApply := m.file != nil && m.choice != "" && !m.showBase
				if m.file != nil && m.file.Text {
					remaining := 0
					for _, b := range m.blocks {
						if !b.resolved() {
							remaining++
						}
					}
					if !m.manual && len(m.blocks) > 0 {
						ui.Text(c, fmt.Sprintf(locales.MergeRemaining(), remaining)).TextColor(theme.Of(c).Muted)
					}
					ui.Button(c, locales.MergeFileActions()).Disabled(m.busy).Menu(func(menu *ui.Menu) {
						if menu.Item(locales.MergeRestart()).Chosen() {
							a.confirmMergeChoice(locales.MergeRestart(), func() { a.initMergeText(m) })
						}
						if menu.Item(locales.MergeWorking()).Chosen() {
							a.confirmMergeChoice(locales.MergeWorking(), func() { m.manual = true; m.views[1].Buffer.Set(strings.ReplaceAll(m.file.Working, "\r\n", "\n")) })
						}
						for i, label := range []string{locales.MergeWholeLeft(), locales.MergeWholeRight()} {
							if menu.Item(label).Chosen() {
								a.confirmMergeChoice(label, func() {
									if i == 0 {
										m.choice = "left"
									} else {
										m.choice = "right"
									}
									a.applyMerge(m)
								})
							}
						}
					})
					if remaining > 0 && !m.manual {
						canApply = false
					}
				}
				ui.Box(c).Grow(1)
				ui.Button(c, locales.Cancel()).Disabled(m.busy).OnClick(func() { a.leaveMerge(m, nil) })
				ui.PrimaryButton(c, locales.MergeApply()).Disabled(!canApply || m.busy).OnClick(func() {
					if m.manual || m.choice == "result" && strings.Contains(m.views[1].Buffer.String(), "<<<<<<<") {
						a.confirmMergeChoice(locales.MergeManualConfirm(), func() { a.applyMerge(m) })
					} else {
						a.applyMerge(m)
					}
				})
			})
		})
		if frame.OverlayShortcut(0, ui.KeyEscape) && !m.busy && a.actionConfirm == nil {
			a.leaveMerge(m, nil)
		}
	})
}

func (a *App) confirmMergeChoice(message string, apply func()) {
	a.actionConfirm = &confirmation{message: message, actions: []action{{locales.MergeContinue(), func() { a.actionConfirm = nil; apply() }}, {locales.Cancel(), func() { a.actionConfirm = nil }}}}
}
