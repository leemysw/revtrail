// INPUT: search 包、ripgrep、lsp 工作区符号与最近文件。
// OUTPUT: Find in Files 的运行与 Search Everywhere 的状态、排序（rankQuick）与选择（chooseQuick）。
// POS: 两类搜索的状态与逻辑；Search Everywhere 的界面在 quickopen.go。
// PROTOCOL: 变更时更新此头部，然后检查本包 doc.go

package app

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"revtrail/internal/config"
	"revtrail/internal/langserver"
	"revtrail/internal/lsp"
	"revtrail/internal/nav"
	"revtrail/internal/search"
	uriutil "revtrail/internal/uri"
	"revtrail/internal/workspace"
)

// ----- The find navigator: text in the workspace, with ripgrep -----

// findState is the find navigator's: its query and options, and the
// results streaming in from ripgrep.
type findState struct {
	open                   bool
	opts                   search.Options
	field                  ui.Handle
	focus                  bool
	showFilters            bool
	mode                   findMode
	module                 string
	modules                []string
	moduleLoading          bool
	namedScope             string
	recentDirs             []string
	shallow, picking       bool
	savedScopes            map[string]config.FindScope
	editingScope           bool
	editName, originalName string
	editScope              config.FindScope
	maskEnabled            bool
	mask                   string
	running                bool
	seq                    int
	cancel                 context.CancelFunc
	timer                  *time.Timer
	ran                    search.Options // what the results are of
	results                []findLine
	stats                  search.Stats
	err                    error
	rg                     string // ripgrep's path, found once per workspace
	rgErr                  error
	list                   ui.ListState
	row                    int
}

type findLine struct {
	path    string
	matches []search.Match
}

func (f *findState) init() {
	f.row = -1
	f.list.Selected = &f.row
}

func (f *findState) reset() {
	if f.cancel != nil {
		f.cancel()
	}
	if f.timer != nil {
		f.timer.Stop()
	}
	opts := f.opts
	opts.Directory = ""
	*f = findState{opts: opts, maskEnabled: f.maskEnabled, mask: f.mask, showFilters: f.showFilters}
	f.init()
}

// findSoon searches once typing stops for a moment.
func (a *App) findSoon() {
	if a.find.timer != nil {
		a.find.timer.Stop()
	}
	a.find.timer = time.AfterFunc(250*time.Millisecond, func() { a.post.Post(a.runFind) })
}

// runFind searches the workspace for the query, dropping the search on
// its way; results come in batches as ripgrep finds them.
func (a *App) runFind() {
	f := &a.find
	if f.timer != nil {
		f.timer.Stop()
		f.timer = nil
	}
	if f.cancel != nil {
		f.cancel()
		f.cancel = nil
	}
	f.seq++
	f.results, f.stats, f.err = nil, search.Stats{}, nil
	f.list.ScrollTo(0, ui.Start)
	f.row = -1
	f.ran = f.opts
	if strings.TrimSpace(f.opts.Query) == "" || a.root == "" {
		f.running = false
		return
	}
	if f.rg == "" && f.rgErr == nil {
		f.rg, f.rgErr = findRipgrep(a.root)
	}
	if f.rgErr != nil {
		f.err = f.rgErr
		f.running = false
		return
	}
	if f.mode == findModule && f.modules == nil {
		a.loadFindModules()
		f.running = f.moduleLoading
		return
	}
	opts, scope := a.findTarget()
	seq, gen, root, rg := f.seq, a.gen, a.root, f.rg
	mode := f.mode
	ctx, cancel := context.WithCancel(a.wsCtx)
	f.cancel = cancel
	f.running = true
	go func() {
		defer cancel()
		var mu sync.Mutex
		var batch []search.File
		last := time.Now()
		flush := func() {
			mu.Lock()
			b := batch
			batch = nil
			mu.Unlock()
			if len(b) == 0 {
				return
			}
			a.post.Post(func() {
				if a.gen == gen && a.find.seq == seq {
					a.addFindResults(b)
				}
			})
		}
		st, err := search.RunScoped(ctx, rg, root, opts, scope, func(file search.File) {
			mu.Lock()
			batch = append(batch, file)
			mu.Unlock()
			if time.Since(last) > 80*time.Millisecond {
				last = time.Now()
				flush()
			}
		})
		flush()
		a.post.Post(func() {
			if a.gen != gen || a.find.seq != seq {
				return
			}
			a.find.running, a.find.cancel = false, nil
			a.find.stats = st
			if err == nil && mode == findDirectory && opts.Directory != "" {
				a.rememberFindDirectory(opts.Directory)
			}
			if err != nil && !errors.Is(err, context.Canceled) {
				a.find.err = err
			}
		})
	}()
}

func (a *App) addFindResults(files []search.File) {
	f := &a.find
	for _, file := range files {
		for rest := file.Matches; len(rest) > 0; {
			end := 1
			previewEnd := rest[0].Col - rest[0].PreStart + len(rest[0].Preview)
			for end < len(rest) && rest[end].Line == rest[0].Line && rest[end].End <= previewEnd {
				end++
			}
			f.results = append(f.results, findLine{file.Path, rest[:end]})
			rest = rest[end:]
		}
		f.stats.Files++
		f.stats.Matches += len(file.Matches)
	}
	if f.row < 0 && len(f.results) > 0 {
		f.row = 0
	}
}

func (a *App) openFindSelection() {
	f := &a.find
	if f.opts != f.ran {
		a.runFind()
		return
	}
	if f.row < 0 || f.row >= len(f.results) {
		return
	}
	line := f.results[f.row]
	m := line.matches[0]
	f.open = false
	a.cancelNav()
	pos := editor.Pos{Line: m.Line, Col: m.Col}
	a.openIn(a.focus, line.path, goTo{pos: &pos, selLen: m.End - m.Col})
}

// ----- Open Quickly: files by name, ranked by sahilm/fuzzy -----

type quickOpen struct {
	open          bool
	recent        bool // Recent Files, not every file
	all           bool // Search Everywhere's All tab: files, then actions
	allFiles      int  // in All, the results before the actions
	query         string
	ranked        string // the query results are of
	files         []string
	listing       bool
	truncated     bool
	err           error
	results       []search.Scored
	list          ui.ListState
	sel           int
	line          int // line to open, from a trailing :N in the query
	col           int // column to open, from a trailing :N:N
	gen           int
	symbols       []quickSymbol
	symbolCancel  context.CancelFunc
	symbolLoading bool
	symbolErr     error
	filePath      string
	fileVersion   uint64
	fileSymbols   []quickSymbol
	fileLoaded    bool
}

type quickSymbol struct {
	target target
	enc    string
	name   string
}

// maxQuickFiles bounds the files Open Quickly lists.
const maxQuickFiles = 200000

func (a *App) openQuickly() {
	a.openQuickPanel("")
}

// searchEverywhere is IntelliJ's Search Everywhere (Double Shift): its
// All tab, the files shown lately until something is typed, then the
// files and the actions that match.
func (a *App) searchEverywhere() {
	a.openQuickPanel("")
	a.quick.all = true
}

// quickAction reports whether result i of the plain query is an action:
// one of the All tab's after its files.
func (q *quickOpen) quickAction(i int) bool {
	return q.all && i >= q.allFiles && (q.query == "" || !strings.ContainsRune("#@>:", rune(q.query[0])))
}

// maxAllActions bounds the actions the All tab lists after its files.
const maxAllActions = 8

// openRecentFiles is IntelliJ's Recent Files (Cmd+E): the workspace's
// files shown lately, the latest first, the one shown now last, which
// typing filters.
func (a *App) openRecentFiles() {
	a.openQuickPanel("")
	a.quick.recent = true
}

// recentPaths lists Recent Files' paths relative to the workspace.
func (a *App) recentPaths() []string {
	var out []string
	for _, p := range slices.Backward(a.recent) {
		if a.root != "" && workspace.Within(a.root, p) {
			out = append(out, filepath.ToSlash(workspace.RelPath(a.root, p)))
		}
	}
	return out
}

func (a *App) openQuickPanel(query string) {
	a.find.open = false
	q := &a.quick
	q.gen++
	if q.symbolCancel != nil {
		q.symbolCancel()
		q.symbolCancel = nil
	}
	q.open, q.query, q.ranked, q.sel, q.recent, q.all = true, query, "\x00", 0, false, false
	q.line, q.col = 0, 0
	q.fileLoaded = false
	q.fileSymbols = nil
	q.list.Selected = &q.sel
	if !strings.HasPrefix(query, ":") && !strings.HasPrefix(query, "@") {
		a.ensureFiles()
	}
}

// ensureFiles lists the workspace's files for Open Quickly and the tree's
// filter, once: the list lasts until the workspace changes or a refresh
// forgets it.
func (a *App) ensureFiles() {
	q := &a.quick
	if q.files != nil || q.listing || a.root == "" {
		return
	}
	q.listing = true
	gen, root := a.gen, a.root
	rg, _ := findRipgrep(root) // "" lists by walking
	ctx, ignored := a.wsCtx, a.ignored
	go func() {
		files, truncated, err := search.ListFiles(ctx, rg, root, maxQuickFiles, ignored.Dir)
		a.post.Post(func() {
			if a.gen != gen {
				return
			}
			q.listing, q.files, q.truncated, q.err = false, files, truncated, err
			if q.files == nil {
				q.files = []string{}
			}
			if !strings.HasPrefix(q.query, ">") && !strings.HasPrefix(q.query, "#") && !strings.HasPrefix(q.query, "@") {
				q.ranked = "\x00" // rank files again
			}
			a.filter.forQuery = "" // filter again over the new list
		})
	}()
}

// rankQuick ranks the files for the query when it changed.
func (a *App) rankQuick() {
	q := &a.quick
	if strings.HasPrefix(q.query, "@") {
		path, version := "", uint64(0)
		if st := a.activeEditor(); st != nil {
			path, version = a.activePath(), st.Buffer.Version()
		}
		if path != q.filePath || version != q.fileVersion {
			q.filePath, q.fileVersion, q.fileLoaded, q.fileSymbols = path, version, false, nil
			q.ranked = "\x00"
		}
	}
	if q.ranked == q.query {
		return
	}
	q.ranked = q.query
	q.gen++
	if q.symbolCancel != nil {
		q.symbolCancel()
		q.symbolCancel = nil
	}
	q.symbolLoading, q.symbolErr = false, nil
	q.symbols = nil
	if strings.HasPrefix(q.query, ":") {
		q.results = nil
		q.line, q.col = 0, 0
		if path, line, col, ok := splitQuickPosition(q.query); ok && path == "" && a.activeEditor() != nil {
			q.line, q.col = line, col
			q.results = []search.Scored{{Path: fmt.Sprintf("%s %d:%d", commandLabel("Go to Line"), line, max(col, 1))}}
		}
	} else if strings.HasPrefix(q.query, ">") {
		names := commandNames()
		q.results = search.Fuzzy(names, strings.TrimSpace(q.query[1:]), len(names))
	} else if strings.HasPrefix(q.query, "@") {
		q.results = nil
		if q.filePath != "" && a.cfg.IsTrusted(a.root) {
			if q.fileLoaded {
				a.filterFileSymbols()
			} else {
				a.searchFileSymbols()
			}
		}
	} else if strings.HasPrefix(q.query, "#") {
		q.results = nil
		if a.root != "" && a.cfg.IsTrusted(a.root) && strings.TrimSpace(q.query[1:]) != "" {
			a.searchSymbols(strings.TrimSpace(q.query[1:]))
		}
	} else {
		a.ensureFiles()
		query := q.query
		q.line, q.col = 0, 0
		if path, line, col, ok := splitQuickPosition(query); ok && line > 0 {
			q.line, q.col = line, col
			query = path
		}
		switch {
		case q.recent || q.all && strings.TrimSpace(query) == "":
			q.results = search.Files(a.recentPaths(), query, maxRecentFiles)
		case q.all:
			q.results = search.Files(q.files, query, 30)
		default:
			q.results = search.Files(q.files, query, 60)
		}
		q.allFiles = len(q.results)
		if q.all && strings.TrimSpace(query) != "" && q.line == 0 {
			q.results = append(q.results, search.Fuzzy(commandNames(), strings.TrimSpace(query), maxAllActions)...)
		}
	}
	q.sel = 0
	if q.recent && strings.TrimSpace(q.query) == "" && len(q.results) > 1 {
		q.sel = 1 // as IntelliJ's, Enter goes back to the file shown before
	}
}

func (a *App) searchSymbols(query string) {
	q := &a.quick
	ctx, cancel := context.WithTimeout(a.wsCtx, 20*time.Second)
	q.symbolCancel, q.symbolLoading = cancel, true
	gen, ranked, request := a.gen, q.ranked, q.gen
	servers := a.allServers()
	go func() {
		defer cancel()
		select {
		case <-time.After(200 * time.Millisecond):
		case <-ctx.Done():
			return
		}
		type answer struct {
			symbols []quickSymbol
			err     error
		}
		answers := make(chan answer, len(servers))
		count := 0
		for _, m := range servers {
			if m == nil {
				continue
			}
			count++
			go func(m *langserver.Server) {
				c, err := m.Get(ctx)
				var out []quickSymbol
				if err == nil {
					if client, ok := c.(interface {
						WorkspaceSymbols(context.Context, string) ([]lsp.WorkspaceSymbol, error)
					}); ok {
						var symbols []lsp.WorkspaceSymbol
						symbols, err = client.WorkspaceSymbols(ctx, query)
						for _, s := range symbols {
							if path, e := uriutil.ToPath(s.Location.URI); e == nil {
								out = append(out, quickSymbol{target: target{path: path, pos: s.Location.Range.Start}, enc: c.Encoding(), name: s.Name})
							}
						}
					}
				}
				answers <- answer{out, err}
			}(m)
		}
		var symbols []quickSymbol
		var firstErr error
		for i := 0; i < count; i++ {
			var result answer
			select {
			case result = <-answers:
			case <-ctx.Done():
				firstErr = ctx.Err()
				i = count
				continue
			}
			symbols = append(symbols, result.symbols...)
			if firstErr == nil {
				firstErr = result.err
			}
			if len(result.symbols) > 0 {
				partial := append([]quickSymbol(nil), symbols...)
				a.post.Post(func() {
					if a.gen != gen || !q.open || q.query != ranked || q.ranked != ranked || q.gen != request {
						return
					}
					q.symbols = partial
					q.results = make([]search.Scored, len(partial))
				})
			}
		}
		a.post.Post(func() {
			if a.gen != gen || !q.open || q.query != ranked || q.ranked != ranked || q.gen != request {
				return
			}
			q.symbolLoading, q.symbolCancel = false, nil
			q.symbols = symbols
			if len(symbols) == 0 {
				q.symbolErr = firstErr
			}
			q.results = make([]search.Scored, len(symbols))
			q.sel = 0
		})
	}()
}

func (a *App) chooseQuick(i int) {
	q := &a.quick
	a.rankQuick()
	if i < 0 || i >= len(q.results) {
		return
	}
	if strings.HasPrefix(q.query, ":") {
		st := a.activeEditor()
		if st == nil {
			return
		}
		if here, ok := a.here(); ok {
			a.hist.Push(here)
		}
		a.cancelNav()
		offset := st.Buffer.Offset(editor.Pos{Line: q.line - 1, Col: max(q.col-1, 0)})
		st.Select(offset, offset)
		a.focusEditor = st
		q.open = false
		return
	}
	p := q.results[i].Path
	q.open = false
	if strings.HasPrefix(q.query, "#") || strings.HasPrefix(q.query, "@") {
		s := q.symbols[i]
		if a.focus != nil {
			if st := a.activeEditor(); st != nil {
				pos := st.CursorPos()
				a.hist.Push(nav.Entry{Path: a.activePath(), Line: pos.Line, ByteColumn: pos.Col})
			}
			a.cancelNav()
			pos := s.target.pos
			a.openIn(a.focus, s.target.path, goTo{lsp: &pos, enc: s.enc})
		}
		return
	}
	if strings.HasPrefix(q.query, ">") || q.quickAction(i) {
		for _, cmd := range commands() {
			if commandLabel(cmd.name) == p {
				cmd.run(a)
				return
			}
		}
		return
	}
	if q.line > 0 {
		path := joinRoot(a.root, p)
		pos := editor.Pos{Line: q.line - 1, Col: q.col}
		if q.col > 0 {
			pos.Col = q.col - 1
		}
		a.openFile(path, &pos)
	} else if q.recent || q.all {
		a.userOpen(joinRoot(a.root, p))
	} else {
		a.previewOpen(joinRoot(a.root, p))
	}
}

func joinRoot(root, rel string) string { return filepath.Join(root, filepath.FromSlash(rel)) }

// splitQuickPosition splits a trailing :line or :line:col from an Open
// Quickly query, so that "foo.go:42" opens foo.go at line 42.
func splitQuickPosition(q string) (string, int, int, bool) {
	parts := strings.Split(q, ":")
	if len(parts) >= 3 {
		if line, err := strconv.Atoi(parts[len(parts)-2]); err == nil && line > 0 {
			if col, err := strconv.Atoi(parts[len(parts)-1]); err == nil && col >= 0 {
				return strings.Join(parts[:len(parts)-2], ":"), line, col, true
			}
		}
	}
	if len(parts) >= 2 {
		if line, err := strconv.Atoi(parts[len(parts)-1]); err == nil && line > 0 {
			return strings.Join(parts[:len(parts)-1], ":"), line, 0, true
		}
	}
	return q, 0, 0, false
}

func (a *App) openFindPopup() {
	a.quick.open = false
	a.find.open, a.find.focus = true, true
	a.runFind()
}
