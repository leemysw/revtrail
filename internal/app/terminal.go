// INPUT: mygo 终端控件与 terminalide。
// OUTPUT: 终端面板、标签、分屏与启动。
// POS: 底部 Terminal 工具窗口。
// PROTOCOL: 变更时更新此头部，然后检查本包 doc.go

package app

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"revtrail/internal/terminalide"
	"revtrail/internal/theme"
	"revtrail/internal/widget"
	"revtrail/locales"
)

// Terminal sessions run in PTYs backed by Ghostty. Hiding the tool window keeps them alive.

type panelView uint8

const (
	panelTerminal panelView = iota
	panelOutput
	panelTasks
	panelGitLog
)

// termPanel is the bottom panel's state.
type termPanel struct {
	ide       *terminalide.Server
	selection terminalSelection
	open      bool
	view      panelView
	maximized bool
	tabs      []*termTab
	active    *termTab
	pct       float32 // the editors' share of the height, as a percentage
	lastShown *termTab
	profiles  []termProfile
	agents    []termProfile
	agent     int
	dialog    *termDialog
	pending   []func()
	loading   bool  // the emulator's library is loading
	loadErr   error // it could not
	focus     *termPane
	focused   bool // a terminal had the keyboard in the last frame
}

// Each session tab owns its split layout and remembers the focused terminal.
type termTab struct {
	name   string
	layout *termLayout
	active *termPane
}

// termPane is one terminal.
type termPane struct {
	term    *terminal.Terminal
	title   string
	command []string
	dir     string
}

// newTerm makes a terminal running the user's shell in the workspace.
// Its callbacks come from the terminal's goroutines: they go through
// post, and are dropped once the pane or the workspace is gone.
func (a *App) newTerm(p *termPane) error {
	gen := a.gen
	cmd, env := a.terminalLaunch(p.command)
	t, err := terminal.New(terminal.Options{
		Dir: p.dir, Command: cmd,
		Font:             a.terminalFont(),
		ShellIntegration: true,
		Cursor: func() terminal.CursorStyle {
			switch a.cfg.TermCursor {
			case "bar":
				return terminal.CursorBar
			case "underline":
				return terminal.CursorUnderline
			}
			return terminal.CursorBlock
		}(),
		NoBlink:     a.cfg.TermCursorNoBlink,
		OptionAsAlt: a.cfg.TermOptionAsAlt,
		CopyOnSelect: map[bool]terminal.CopyOnSelect{
			true: terminal.CopyOnSelectClipboard,
		}[a.cfg.TermCopyOnSelect],
		RightClick: func() terminal.RightClickAction {
			switch a.cfg.TermRightClick {
			case "paste":
				return terminal.RightClickPaste
			case "copyOrPaste":
				return terminal.RightClickCopyOrPaste
			}
			return terminal.RightClickContextMenu
		}(),
		ClipboardRead: func() terminal.ClipboardAccess {
			switch a.cfg.TermClipboard {
			case "allow":
				return terminal.ClipboardAllow
			case "deny":
				return terminal.ClipboardDeny
			}
			return terminal.ClipboardAsk
		}(),
		Scrollback: func() int {
			v := a.cfg.TermScrollback
			if v == 0 {
				v = 4
			}
			if v < 0 {
				return -1
			}
			return v << 20
		}(),
		Env:             env,
		MinimumContrast: float32(a.cfg.TermMinContrast),
		OnTitle: func(title string) {
			a.post.Post(func() {
				if a.gen == gen {
					p.title = title
				}
			})
		},
		OnExit: func(int) {
			a.post.Post(func() {
				if a.gen == gen {
					a.closePane(p)
				}
			})
		},
		OpenLink: func(link string) bool { return a.openTermLink(p, link) },
	})
	if err != nil {
		if len(a.term.tabs) == 0 {
			a.term.closeIDE()
		}
		return err
	}
	p.term = t
	return nil
}

// withTerminal runs fn once the emulator's library is loaded, which may
// download it the first time under go run and go test, off the UI
// thread; packaged apps carry it.
func (a *App) withTerminal(fn func()) {
	tp := &a.term
	if tp.loadErr != nil {
		return
	}
	if termLoaded {
		fn()
		return
	}
	tp.pending = append(tp.pending, fn)
	if tp.loading {
		return
	}
	tp.loading = true
	gen := a.gen
	go func() {
		err := terminal.Load()
		a.post.Post(func() {
			if a.gen != gen {
				return
			}
			queued := tp.pending
			tp.pending = nil
			tp.loading = false
			if err != nil {
				tp.loadErr = err
				return
			}
			termLoaded = true
			for _, start := range queued {
				start()
			}
		})
	}()
}

var termLoaded bool

// toggleTerminal shows or hides the panel, with a terminal in it.
func (a *App) toggleTerminal() {
	tp := &a.term
	if tp.open && tp.view == panelTerminal {
		a.hideTerminal()
		return
	}
	a.showTerminal()
}

func (a *App) hideTerminal() {
	tp := &a.term
	tp.open, tp.maximized, tp.focused = false, false, false
	tp.focus = nil
	if a.focus != nil {
		a.focusEditor = a.focus.eds[a.focus.active]
	}
}

func (a *App) showTerminal() {
	if a.root == "" {
		return
	}
	tp := &a.term
	tp.open, tp.view = true, panelTerminal
	a.focusEditor = nil
	if tp.active != nil {
		tp.focus = tp.active.active
	} else if !tp.loading {
		a.newTerminalTab()
	}
}

func (a *App) newTerminalTab() { a.newTerminalTabIn("") }

// newTerminalTabIn starts a terminal in dir, the workspace when "", as
// IntelliJ's Open In › Terminal.
func (a *App) newTerminalTabIn(dir string) {
	var cmd []string
	if a.cfg.TermShell != "" {
		cmd = []string{a.cfg.TermShell}
	}
	a.startTerminalIn(termProfile{locales.TermLocal(), cmd}, dir)
}

func (a *App) startTerminal(profile termProfile) { a.startTerminalIn(profile, "") }

func (a *App) startTerminalIn(profile termProfile, dir string) {
	if a.root == "" {
		return
	}
	tp := &a.term
	tp.open, tp.view = true, panelTerminal
	if dir == "" {
		dir = a.root
	}
	a.focusEditor = nil
	a.withTerminal(func() {
		p := &termPane{title: profile.name, command: profile.cmd, dir: dir}
		if err := a.newTerm(p); err != nil {
			a.note(locales.MsgCouldNotStartATerminal()+err.Error(), true)
			return
		}
		name := profile.name
		for i := 2; slices.ContainsFunc(tp.tabs, func(t *termTab) bool { return t.name == name }); i++ {
			name = fmt.Sprintf("%s (%d)", profile.name, i)
		}
		tab := &termTab{name: name, layout: &termLayout{pane: p}, active: p}
		tp.tabs = append(tp.tabs, tab)
		tp.active = tab
		if tp.open && tp.view == panelTerminal {
			tp.focus = p
		}
	})
}

func (a *App) splitTerminal() { a.splitTerminalPane(false) }

func (a *App) splitTerminalPane(below bool) {
	tp := &a.term
	tab := tp.active
	if tab == nil {
		a.newTerminalTab()
		return
	}
	old := tab.active
	dir := old.dir
	if cwd := old.term.Dir(); filepath.IsAbs(cwd) && (len(old.command) == 0 || old.command[0] != "ssh") {
		dir = cwd
	}
	p := &termPane{command: old.command, dir: dir, title: tab.name}
	if err := a.newTerm(p); err != nil {
		a.note(locales.MsgCouldNotStartATerminal()+err.Error(), true)
		return
	}
	n := tab.layout.node(old)
	*n = termLayout{first: &termLayout{pane: old}, second: &termLayout{pane: p}, pct: 50, below: below}
	tp.open, tp.view, tab.active, tp.focus = true, panelTerminal, p, p
}

type termLayout struct {
	pane          *termPane
	first, second *termLayout
	below         bool
	pct           float32
}

func (n *termLayout) node(p *termPane) *termLayout {
	if n.pane != nil {
		if n.pane == p {
			return n
		}
		return nil
	}
	if v := n.first.node(p); v != nil {
		return v
	}
	return n.second.node(p)
}
func (n *termLayout) leaves(out []*termPane) []*termPane {
	if n.pane != nil {
		return append(out, n.pane)
	}
	return n.second.leaves(n.first.leaves(out))
}
func (n *termLayout) remove(p *termPane) *termLayout {
	if n.pane == p {
		return nil
	}
	if n.pane != nil {
		return n
	}
	n.first = n.first.remove(p)
	n.second = n.second.remove(p)
	if n.first == nil {
		return n.second
	}
	if n.second == nil {
		return n.first
	}
	return n
}

func (a *App) closePane(p *termPane) {
	tp := &a.term
	for _, tab := range tp.tabs {
		if tab.layout.node(p) == nil {
			continue
		}
		if tab.layout.pane == p {
			a.closeTermTab(tab)
			return
		}
		p.term.Close()
		tab.layout = tab.layout.remove(p)
		if tab.active == p {
			tab.active = tab.layout.leaves(nil)[0]
			if tp.active == tab && tp.open && tp.view == panelTerminal {
				tp.focus = tab.active
			}
		}
		return
	}
}

func (a *App) closeTermTab(tab *termTab) {
	tp := &a.term
	i := slices.Index(tp.tabs, tab)
	if i < 0 {
		return
	}
	tp.tabs = slices.Delete(tp.tabs, i, i+1)
	for _, p := range tab.layout.leaves(nil) {
		p.term.Close()
	}
	if tp.active != tab {
		return
	}
	tp.active = nil
	tp.focus = nil
	if len(tp.tabs) == 0 {
		tp.closeIDE()
		if tp.view == panelTerminal {
			a.hideTerminal()
		}
	} else {
		tp.active = tp.tabs[min(i, len(tp.tabs)-1)]
		if tp.open && tp.view == panelTerminal {
			tp.focus = tp.active.active
		}
	}
}

// closeTerminals ends every terminal, as the workspace closes.
func (a *App) closeTerminals() {
	a.term.closeIDE()
	for _, tab := range a.term.tabs {
		for _, p := range tab.layout.leaves(nil) {
			if p.term != nil {
				p.term.Close()
			}
		}
	}
	pct := a.term.pct
	a.term = termPanel{pct: pct, loadErr: a.term.loadErr}
}

func shellName() string {
	if sh := os.Getenv("SHELL"); sh != "" {
		return filepath.Base(sh)
	}
	return "shell"
}

// termLink matches a path printed with a line, and a column: main.go:12:3.
var termLink = regexp.MustCompile(`^(.+?)(?::(\d+))?(?::(\d+))?$`)

// openTermLink opens a file path a terminal printed, which Cmd+click
// opens, in the editor, at its line and column; other links open with
// the system. It runs on the UI thread.
func (a *App) openTermLink(p *termPane, link string) bool {
	if strings.Contains(link, "://") {
		return false
	}
	m := termLink.FindStringSubmatch(link)
	if m == nil {
		return false
	}
	path := m[1]
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			path = filepath.Join(home, path[2:])
		}
	}
	if !filepath.IsAbs(path) {
		dir := a.root
		if p.term != nil && p.term.Dir() != "" {
			dir = p.term.Dir()
		}
		path = filepath.Join(dir, path)
	}
	fi, err := os.Stat(path)
	if err != nil || fi.IsDir() {
		return false
	}
	var pos *editor.Pos
	if m[2] != "" {
		line, _ := strconv.Atoi(m[2])
		col, _ := strconv.Atoi(m[3])
		pos = &editor.Pos{Line: max(line-1, 0), Col: max(col-1, 0)}
	}
	a.cancelNav()
	a.openFile(path, pos)
	if st := a.activeEditor(); st != nil {
		a.focusEditor = st
	}
	return true
}

// ----- View -----

// editorsAndPanel lays out the editors over the bottom panel, which the
// divider between them resizes, as Xcode's debug area.
func (a *App) editorsAndPanel(c *ui.Context, editors func()) {
	tp := &a.term
	if tp.pct <= 0 {
		tp.pct = float32(a.cfg.EditorPanelPct())
	}
	switch {
	case !tp.open:
		editors()
	case tp.maximized:
		a.terminalPanel(c)
	default:
		if widget.SplitPanes(c, ui.SplitVerticalPercent, &tp.pct, editors, func() { a.terminalPanel(c) }).Grow(1).FillWidth().Changed() {
			a.cfg.PanelPct = float64(tp.pct)
			a.saveConfigSoon()
		}
	}
}

func (a *App) terminalPanel(c *ui.Context) {
	tp := &a.term
	// The split's divider, or the toolbar's border when maximized, is its
	// edge. GoLand draws the Git log in a tool window's background, where
	// its splits' dividers show, and the terminal in the editor's.
	bg := theme.Of(c).Editor
	if tp.view == panelGitLog {
		bg = theme.Of(c).Sidebar
	}
	panel := ui.Column(c).Fill().Background(bg)
	panel.Children(func() {
		if tp.view == panelGitLog {
			a.gitLogWindow(c)
			return
		}
		a.terminalHeader(c)
		ui.Column(c).Grow(1).FillWidth().Children(func() {
			switch {
			case tp.view == panelTasks:
				a.taskView(c)
			case tp.view == panelOutput:
				a.logView(c)
			case tp.loadErr != nil:
				a.message(c, locales.TerminalUnavailable(), tp.loadErr.Error(), "", nil)
			case tp.loading:
				ui.Row(c).Grow(1).FillWidth().Center().Gap(8).Children(func() {
					ui.Spinner(c)
					ui.Text(c, locales.MsgLoadingTheTerminal()).TextColor(theme.Of(c).Muted)
				})
			case tp.active != nil:
				a.paneSplit(c, tp.active, tp.active.layout)
			}
		})
	})
	focused := tp.view == panelTerminal && panel.FocusWithin()
	if focused && !tp.focused {
		a.saveOnFocusLoss()
	}
	tp.focused = focused
}

func (a *App) paneSplit(c *ui.Context, tab *termTab, n *termLayout) {
	if n.pane != nil {
		a.termPaneView(c, tab, n.pane)
		return
	}
	split := ui.SplitPercent
	if n.below {
		split = ui.SplitVerticalPercent
	}
	widget.SplitPanes(c.Key(n), split, &n.pct, func() { a.paneSplit(c, tab, n.first) }, func() { a.paneSplit(c, tab, n.second) }).Fill()
}

func (a *App) termPaneView(c *ui.Context, tab *termTab, p *termPane) {
	tp := &a.term
	box := ui.Column(c.Key(p)).Grow(1).Fill()
	if tab.layout.pane == nil && tab.active == p {
		box.DrawOver(func(pt *ui.Painter, r ui.Rect) {
			pt.Fill(ui.Rect{X: r.X, Y: r.Y, W: r.W, H: 1}, c.Theme().Accent, 0)
		})
	}
	box.Children(func() {
		v := terminal.View(c, p.term).Grow(1).Fill().Label(locales.MsgTerminal()+p.title).FocusBind(&tp.focus, p)
		if v.FocusWithin() {
			tab.active = p
		}
	})
}

func (a *App) terminalFont() terminal.Font {
	f := terminal.Font{Family: a.cfg.TermFontFamily, Size: float32(a.cfg.TerminalFontSize()), Weight: a.cfg.TermFontWeight, Thicken: a.cfg.TermThicken}
	const promptFonts = "MesloLGS NF, MesloLGS Nerd Font Mono, JetBrainsMono Nerd Font Mono"
	if f.Family == "" {
		f.Family = promptFonts + ", Menlo, monospace"
	}
	if fallback := strings.TrimSpace(a.cfg.TermFontFallback); fallback != "" {
		f.Family += ", " + fallback
	}
	f.Family += ", Symbols Nerd Font Mono"
	if a.cfg.TermFontFamily != "" {
		f.Family += ", " + promptFonts + ", monospace"
	}
	if a.cfg.TermLineHeight >= -20 && a.cfg.TermLineHeight <= 100 {
		f.LineHeight = float32(1 + a.cfg.TermLineHeight/100)
	}
	if a.cfg.TermNoLigatures {
		f.Features = append([]string{"-liga", "-calt"}, f.Features...)
	}
	if len(a.cfg.TermFontFeatures) > 0 {
		f.Features = append(slices.Clone(a.cfg.TermFontFeatures), f.Features...)
	}
	return f
}

// applyTerminalPrefs gives running terminals the Terminal pane's settings.
func (a *App) applyTerminalPrefs() {
	cos := terminal.CopyOnSelect(0)
	if a.cfg.TermCopyOnSelect {
		cos = terminal.CopyOnSelectClipboard
	}
	for _, tab := range a.term.tabs {
		for _, p := range tab.layout.leaves(nil) {
			if p.term != nil {
				p.term.SetFont(a.terminalFont())
				p.term.SetCopyOnSelect(cos)
				p.term.SetMinimumContrast(float32(a.cfg.TermMinContrast))
			}
		}
	}
}
