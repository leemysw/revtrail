// INPUT: 提交信息编辑器、commitai 与最近提交信息。
// OUTPUT: 生成 / 历史提交信息等操作。
// POS: Commit 工具窗口的提交信息区。
// PROTOCOL: 变更时更新此头部，然后检查本包 doc.go

package app

import (
	"errors"
	"slices"
	"strings"

	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"

	"revtrail/internal/glyph"
	"revtrail/internal/theme"
	"revtrail/locales"
)

func (a *App) commitMessageActions(c *ui.Context, included int) {
	history := locales.CommitMessageHistory()
	b := toolWindowAction(c, glyph.CommitHistory, history, history+" (Ctrl+M)", 0, a.git.busy)
	if b.Clicked() || (!a.git.busy && c.Shortcut(ui.Ctrl, ui.KeyM)) {
		a.git.messageHistoryOpen = !a.git.messageHistoryOpen
	}
	ui.PopoverBase(c, b, &a.git.messageHistoryOpen, func(panel ui.Element) {
		pal := theme.Of(c)
		panel.Width(300).Padding(4).Background(pal.Popover).Radius(6).Border(1, pal.PopoverEdge)
		ui.Scroll(c).FillWidth().MaxHeight(260).Children(func() {
			ui.Column(c).FillWidth().Children(func() {
				if len(a.git.messageHistory) == 0 {
					ui.Text(c, locales.CommitHistoryEmpty()).Padding(7).TextColor(pal.Muted)
				}
				for _, msg := range a.git.messageHistory {
					title, _, _ := strings.Cut(msg, "\n")
					row := ui.ButtonBase(c.Key(msg)).FillWidth().Padding(5, 8).Radius(4).Label(title).Tooltip(msg)
					if row.Hovered() {
						row.Background(pal.Pill)
					}
					row.Children(func() { ui.Text(c, title).SingleLine().Ellipsis("…") })
					row.OnClick(func() { a.git.message = msg; a.git.messageHistoryOpen = false })
				}
			})
		})
	})
	if a.git.generatingMessage {
		ui.Spinner(c).Tooltip(locales.AiGenerating())
		return
	}
	title := locales.AiGenerateCommit()
	configured := a.cfg.AIBaseURL != "" && strings.TrimSpace(a.cfg.AIModel) != ""
	disabled := a.git.busy || configured && included == 0 && !a.git.amend
	tip := title
	if !configured {
		tip = locales.AiConfigure()
	} else if included == 0 && !a.git.amend {
		tip = locales.AiIncludeFirst()
	}
	generate := toolWindowAction(c, glyph.AISparkle, title, tip, 0, disabled)
	if generate.Clicked() {
		if !configured {
			a.host.OpenSettings()
			a.settingsGo("ai")
			a.host.RefreshSettings()
			return
		}
		a.generateCommitMessage()
	}
}

func (a *App) generateCommitMessage() {
	if !a.saveNamedFiles() {
		return
	}
	changes := a.git.includedChanges()
	batches := a.commitBatches(changes)
	ctx, root, gen := a.wsCtx, a.git.root, a.gen
	base, model, language := a.cfg.AIBaseURL, strings.TrimSpace(a.cfg.AIModel), i18n.Locale()
	original, amend, head := a.git.message, a.git.amend, a.git.head
	a.git.generatingMessage, a.git.commitError = true, ""
	go func() {
		patch, err := commitDiff(ctx, batches, amend)
		if err == nil && patch == "" {
			err = errors.New(locales.AiIncludeFirst())
		}
		var message string
		if err == nil {
			client, keyErr := aiClient(base)
			err = keyErr
			if err == nil {
				message, err = client.Generate(ctx, model, language, patch)
			}
		}
		if err == nil {
			var current string
			current, err = commitDiff(ctx, batches, amend)
			if err == nil && current != patch {
				err = errors.New(locales.AiSelectionChanged())
			}
		}
		a.post.Post(func() {
			if gen != a.gen {
				return
			}
			a.git.generatingMessage = false
			if a.git.root != root || a.git.busy || a.git.message != original || !slices.Equal(changes, a.git.includedChanges()) || a.git.amend != amend || a.git.head != head {
				return
			}
			if err != nil {
				a.git.commitError = err.Error()
				return
			}
			a.git.message = message
		})
	}()
}
