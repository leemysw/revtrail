// INPUT: 项目树行与键入的字符。
// OUTPUT: 项目树中直接键入的快速定位。
// POS: 树内速搜。
// PROTOCOL: 变更时更新此头部，然后检查本包 doc.go

package app

import (
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"

	"revtrail/internal/glyph"
	"revtrail/internal/theme"
	"revtrail/locales"
)

type treeSearch struct {
	query, marked   string
	focus, hadFocus bool
	pattern         *regexp.Regexp
}

func (s *treeSearch) active() bool    { return s.pattern != nil || s.marked != "" || s.focus }
func (s *treeSearch) composing() bool { return s.marked != "" }

// Keep the IME attached to the outline until it commits the first character.
func (s *treeSearch) input(ev ui.InputEvent) bool {
	switch ev.Kind {
	case ui.InputCompose:
		s.marked = ev.Text
	case ui.InputText:
		if strings.TrimSpace(ev.Text) == "" && s.query == "" {
			return false
		}
		s.marked = ""
		s.query += ev.Text
		s.focus = true
	default:
		return false
	}
	return true
}

func (a *App) treeSearchField(c *ui.Context) {
	s := &a.filter.speed
	glyph.Draw(c, glyph.Search, 13, theme.Of(c).Muted)
	if s.composing() {
		ui.Text(c, s.marked).Underline().SingleLine().Grow(1)
		return
	}
	field := ui.TextInput(c.Key("tree-speed-search"), &s.query).Label(locales.Search()).Grow(1).MinWidth(0).Height(24).Padding(0, 6)
	if s.focus || field.Changed() {
		if s.query != "" {
			s.pattern = regexp.MustCompile("(?i)" + regexp.QuoteMeta(s.query))
			a.selectTreeMatch(0)
		} else {
			*s = treeSearch{}
			a.filter.toList = true
			return
		}
	}
	if s.focus {
		field.Focus()
		n := utf8.RuneCountInString(s.query)
		field.SetTextSelection(n, n)
		s.focus = false
	}
	if field.Focused() {
		s.hadFocus = true
	} else if s.hadFocus {
		*s = treeSearch{}
		return
	}
	field.OnTextKey(func(k ui.TextKey) bool {
		if k.Mods != 0 {
			return false
		}
		switch k.Key {
		case ui.KeyDown:
			a.selectTreeMatch(1)
		case ui.KeyUp:
			a.selectTreeMatch(-1)
		case ui.KeyEnter:
			if !a.selectTreeMatch(0) {
				return true
			}
			path := a.tree.Item(a.treeRow)
			*s = treeSearch{}
			if a.isTreeDir(path) {
				a.tree.Open.Add(path)
				a.filter.toList = true
			} else {
				a.openFromTree(path)
			}
		default:
			return false
		}
		return true
	})
}

// Match only rows visible in the tree's current expansion state.
func (a *App) selectTreeMatch(direction int) bool {
	n := a.tree.Rows()
	start := a.treeRow
	step := direction
	if step == 0 {
		start = max(0, start)
		step = 1
	} else {
		start += step
	}
	for i := range n {
		idx := ((start+i*step)%n + n) % n
		path := a.tree.Item(idx)
		if !isPlaceholder(path) && a.filter.speed.pattern.MatchString(filepath.Base(path)) {
			a.setTreeRow(idx)
			a.tree.List.ScrollIntoView(idx)
			return true
		}
	}
	return false
}

func (s *treeSearch) filename(c *ui.Context, name string) ui.Element {
	if s.pattern == nil {
		return ui.Text(c, name)
	}
	var spans []ui.Span
	at := 0
	for _, match := range s.pattern.FindAllStringIndex(name, -1) {
		spans = append(spans, ui.Span{Text: name[at:match[0]]}, ui.Span{Text: name[match[0]:match[1]], Background: theme.Of(c).Match, Weight: 600})
		at = match[1]
	}
	if at == 0 {
		return ui.Text(c, name)
	}
	return ui.RichText(c, append(spans, ui.Span{Text: name[at:]})...)
}
