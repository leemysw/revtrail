// INPUT: git 操作需要的确认与输入。
// OUTPUT: 通用模态框、gitPrompt 输入框、本地改动冲突时的 Smart / Force Checkout 选择。
// POS: Git 相关对话框的公共件；树的复制 / 移动对话框也复用 gitPrompt。
// PROTOCOL: 变更时更新此头部，然后检查本包 doc.go

package app

import (
	"strings"

	"github.com/egoist/mygo/ui"

	"revtrail/internal/git"
	"revtrail/internal/theme"
	"revtrail/internal/widget"
	"revtrail/locales"
)

// modal draws a dialog over the dimmed window: width and height are the
// most it takes, 0 height to fit its content. Escape calls dismiss, and
// Return calls enter when it is not nil.
func modal(c *ui.Context, label string, width, height float32, dismiss, enter func(), body func()) {
	pal := theme.Of(c)
	w, h := c.Size()
	ui.Overlay(c, func() {
		back := ui.Box(c).Absolute().Left(0).Top(0).Right(0).Bottom(0).Center().Modal().Background(theme.Shade(.14))
		if back.OverlayShortcut(0, ui.KeyEscape) {
			dismiss()
		}
		if enter != nil && back.OverlayShortcut(0, ui.KeyEnter) {
			enter()
		}
		back.Children(func() {
			panel := ui.Column(c).Width(min(width, w-32)).Padding(18).Gap(12).
				Radius(9).Border(1, pal.PopoverEdge).Background(pal.Popover).Shadow(0, 8, 26, 0, theme.Shade(.30)).
				Role(ui.RoleAlertDialog).Label(label)
			if height > 0 {
				panel.Height(min(height, h-48))
			}
			panel.Children(body)
		})
	})
}

// modalHeader is a dialog's title line, with a Close button.
func modalHeader(c *ui.Context, title, detail string, dismiss func()) {
	ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(8).Children(func() {
		ui.Text(c, title).FontSize(widget.FontSize(c, 15)).Bold().SingleLine().Shrink(0)
		ui.Text(c, detail).FontSize(widget.FontSize(c, 11)).TextColor(theme.Of(c).Muted).SingleLine().Grow(1).Shrink(1)
		ui.Button(c, locales.Close()).OnClick(dismiss)
	})
}

// modalHint fills a dialog's body with one muted line: loading, an
// error, or nothing to show.
func modalHint(c *ui.Context, msg string) {
	ui.Box(c).Grow(1).Center().Children(func() { ui.Text(c, msg).TextColor(theme.Of(c).Muted) })
}

type gitChoiceOption struct {
	value, label, desc string
}

// gitChoiceDialog asks for one of a few ways to do an operation, as
// IntelliJ's Reset and Update Project dialogs.
type gitChoiceDialog struct {
	title, subtitle, groupLabel string
	selected                    string
	options                     []gitChoiceOption
	submitLabel                 string
	submit                      func(string)
}

func (a *App) gitChoiceModal(c *ui.Context) {
	d := a.git.choice
	if d == nil {
		return
	}
	pal := theme.Of(c)
	dismiss := func() { a.git.choice = nil }
	submit := func() {
		a.git.choice = nil
		d.submit(d.selected)
	}
	modal(c, d.title, 480, 0, dismiss, submit, func() {
		ui.Column(c).FillWidth().Gap(4).Children(func() {
			ui.Text(c, d.title).FillWidth().FontSize(widget.FontSize(c, 15)).Bold()
			ui.Text(c, d.subtitle).FillWidth().FontSize(widget.FontSize(c, 12)).TextColor(pal.Muted)
		})
		ui.Text(c, d.groupLabel).FillWidth().FontSize(widget.FontSize(c, 12)).Bold().TextColor(pal.Muted)
		ui.RadioGroup(c, func() {
			for _, opt := range d.options {
				ui.Column(c).FillWidth().Gap(2).Padding(2, 0).Children(func() {
					ui.Radio(c, &d.selected, opt.value, opt.label)
					if opt.desc != "" {
						ui.Text(c, opt.desc).FillWidth().Padding(0, 0, 0, 22).FontSize(widget.FontSize(c, 11)).TextColor(pal.Muted)
					}
				})
			}
		}).Label(d.groupLabel).FillWidth().Gap(6)
		ui.Row(c).FillWidth().Justify(ui.End).Gap(8).Children(func() {
			ui.Button(c, locales.Cancel()).MinWidth(80).OnClick(dismiss)
			ui.PrimaryButton(c, d.submitLabel).MinWidth(80).OnClick(submit)
		})
	})
}

// gitPrompt asks for a name or a text, with an optional checkbox, as
// IntelliJ's Create New Branch dialog with its Checkout branch option.
type gitPrompt struct {
	title, detail, placeholder, submitLabel string
	value                                   string
	toggleLabel                             string
	toggle                                  bool
	optional, multiline                     bool
	focused                                 bool
	submit                                  func(value string, toggle bool)
}

func (a *App) ask(p *gitPrompt) {
	if !a.git.busy {
		a.git.prompt = p
	}
}

func (a *App) gitPromptModal(c *ui.Context) {
	p := a.git.prompt
	if p == nil {
		return
	}
	width := float32(420)
	if p.multiline {
		width = 640
	}
	value := func() string {
		if p.multiline {
			return p.value
		}
		return strings.TrimSpace(p.value)
	}
	ready := p.optional || strings.TrimSpace(p.value) != ""
	dismiss := func() { a.git.prompt = nil }
	submit := func() {
		if ready {
			a.git.prompt = nil
			p.submit(value(), p.toggle)
		}
	}
	modal(c, p.title, width, 0, dismiss, nil, func() {
		ui.Text(c, p.title).FillWidth().FontSize(widget.FontSize(c, 15)).Bold()
		if p.detail != "" {
			ui.Text(c, p.detail).FillWidth().FontSize(widget.FontSize(c, 12)).TextColor(theme.Of(c).Muted)
		}
		var input ui.Element
		if p.multiline {
			input = ui.TextArea(c, &p.value).FillWidth().Height(180).Placeholder(p.placeholder).Label(p.title)
		} else {
			input = ui.TextInput(c, &p.value).FillWidth().Height(28).Placeholder(p.placeholder).Label(p.title)
		}
		if p.multiline && input.Shortcut(ui.Cmd, ui.KeyEnter) || !p.multiline && input.Submitted() {
			submit()
		}
		if !p.focused {
			input.Focus()
			p.focused = true
		}
		if p.toggleLabel != "" {
			ui.Checkbox(c, &p.toggle, p.toggleLabel)
		}
		ui.Row(c).FillWidth().Justify(ui.End).Gap(8).Children(func() {
			ui.Button(c, locales.Cancel()).OnClick(dismiss)
			ui.PrimaryButton(c, p.submitLabel).Disabled(!ready).OnClick(submit)
		})
	})
}

// localProblem is the text of IntelliJ's Git Checkout Problem dialog for
// one operation. force is "" when the operation offers no Force variant.
type localProblem struct {
	title, message, detail string
	smart, force, dont     string
}

func checkoutProblem() localProblem {
	return localProblem{
		title: locales.GitCheckoutProblem(), message: locales.GitCheckoutOverwrite(), detail: locales.GitSmartCheckoutDetail(),
		smart: locales.GitSmartCheckout(), force: locales.GitForceCheckout(), dont: locales.GitDontCheckout(),
	}
}

func mergeProblem() localProblem {
	return localProblem{
		title: locales.GitMergeProblem(), message: locales.GitMergeOverwrite(), detail: locales.GitSmartMergeDetail(),
		smart: locales.GitSmartMerge(), dont: locales.GitDontMerge(),
	}
}

// askLocalChanges asks what to do when git refused an operation because
// it would overwrite local changes. Smart sets them aside and brings them
// back after; Force drops them.
func (a *App) askLocalChanges(p localProblem, retry func(git.LocalChanges)) {
	choose := func(mode git.LocalChanges) func() {
		return func() { a.actionConfirm = nil; retry(mode) }
	}
	actions := []action{{p.smart, choose(git.StashChanges)}}
	if p.force != "" {
		actions = append(actions, action{p.force, choose(git.DiscardChanges)})
	}
	actions = append(actions, action{p.dont, func() { a.actionConfirm = nil }})
	a.actionConfirm = &confirmation{title: p.title, message: p.message, detail: p.detail, actions: actions}
}
