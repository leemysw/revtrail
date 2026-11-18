// INPUT: 当前文件的 lsp DocumentSymbols。
// OUTPUT: File Structure（@ 查询）的符号拉取、过滤与结构导航栏。
// POS: 当前文件符号视图。
// PROTOCOL: 变更时更新此头部，然后检查本包 doc.go

package app

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"

	"revtrail/internal/langserver"
	"revtrail/internal/lsp"
	"revtrail/internal/search"
	"revtrail/internal/theme"
	"revtrail/internal/widget"
	"revtrail/locales"
)

func (a *App) filterFileSymbols() {
	q := &a.quick
	query := strings.ToLower(strings.TrimSpace(q.query[1:]))
	q.symbols = nil
	for _, symbol := range q.fileSymbols {
		if strings.Contains(strings.ToLower(symbol.name), query) {
			q.symbols = append(q.symbols, symbol)
		}
	}
	q.results = make([]search.Scored, len(q.symbols))
}

func (a *App) searchFileSymbols() {
	q := &a.quick
	d := a.docs[q.filePath]
	m := a.serverFor(q.filePath)
	if d == nil || d.buf == nil || m == nil {
		q.symbolErr = errors.New(locales.MsgNoLanguageServerIsConfiguredForThisFile())
		return
	}
	gen, request, query := a.gen, q.gen, q.query
	q.symbolLoading = true
	q.symbolCancel = a.requestFileSymbols(d, m, func(symbols []quickSymbol, err error) {
		if a.gen != gen || q.gen != request || !q.open || q.query != query || a.activePath() != d.path {
			return
		}
		q.symbolCancel, q.symbolLoading, q.symbolErr = nil, false, err
		q.fileLoaded, q.fileSymbols = err == nil, symbols
		a.filterFileSymbols()
	})
}

func (a *App) requestFileSymbols(d *doc, m *langserver.Server, done func([]quickSymbol, error)) context.CancelFunc {
	snap, docURI, path := d.buf.Snapshot(), d.uri, d.path
	ctx, cancel := context.WithTimeout(a.wsCtx, 20*time.Second)
	unsupported := locales.Catalog.Format(i18n.Locale(), "fileSymbolsUnsupported", nil)
	go func() {
		defer cancel()
		client, err := m.Get(ctx)
		var symbols []quickSymbol
		if err == nil {
			err = m.Sync(ctx, client, docURI, snap)
		}
		if err == nil {
			if provider, ok := client.(interface {
				DocumentSymbols(context.Context, string) ([]lsp.DocumentSymbol, error)
			}); ok {
				var result []lsp.DocumentSymbol
				result, err = provider.DocumentSymbols(ctx, docURI)
				for _, symbol := range result {
					symbols = append(symbols, quickSymbol{name: symbol.Name, enc: client.Encoding(), target: target{path: path, pos: symbol.Position}})
				}
			} else {
				err = errors.New(unsupported)
			}
		}
		a.post.Post(func() {
			if a.docs[path] == d && d.buf.Version() == snap.Version() && a.serverFor(path) == m {
				done(symbols, err)
			}
		})
	}()
	return cancel
}

type structureView struct {
	doc     *doc
	version uint64
	server  *langserver.Server
	cancel  context.CancelFunc
	symbols []quickSymbol
	err     error
	list    ui.ListState
}

func (a *App) structureNavigator(c *ui.Context) {
	ui.Row(c).Height(editorTabBarHeight).FillWidth().AlignItems(ui.Center).Padding(0, 10).BorderWidth(0, 0, 1, 0).BorderColor(theme.Of(c).Hairline).Children(func() { ui.Text(c, locales.ToolStructure()).Bold().FontSize(widget.FontSize(c, 12)) })
	d := a.docs[a.activePath()]
	if d == nil || a.activeEditor() == nil {
		if a.structure != nil && a.structure.cancel != nil {
			a.structure.cancel()
		}
		a.structure = nil
		ui.Text(c, locales.FileSymbolsOpenText()).FontSize(widget.FontSize(c, 12)).TextColor(theme.Of(c).Muted).Padding(10)
		return
	}
	if !a.cfg.IsTrusted(a.root) {
		if ui.Button(c, locales.MsgTrustFolderToSearchSymbols()).Clicked() {
			a.trustAsk = true
		}
		return
	}
	m := a.serverFor(d.path)
	if m == nil {
		ui.Text(c, locales.MsgNoLanguageServerIsConfiguredForThisFile()).FontSize(widget.FontSize(c, 12)).TextColor(theme.Of(c).Muted).Padding(10)
		return
	}
	v := a.structure
	if v == nil || v.doc != d || v.version != d.buf.Version() || v.server != m {
		if v != nil && v.cancel != nil {
			v.cancel()
		}
		v = &structureView{doc: d, version: d.buf.Version(), server: m}
		a.structure = v
		v.cancel = a.requestFileSymbols(d, m, func(symbols []quickSymbol, err error) { v.cancel, v.symbols, v.err = nil, symbols, err })
	}
	switch {
	case v.cancel != nil:
		ui.Text(c, locales.MsgSearchingSymbols()).FontSize(widget.FontSize(c, 12)).TextColor(theme.Of(c).Muted).Padding(10)
	case v.err != nil:
		ui.Text(c, v.err.Error()).FontSize(widget.FontSize(c, 12)).TextColor(theme.Of(c).Muted).Padding(10)
	case len(v.symbols) == 0:
		ui.Text(c, locales.ToolNoStructure()).FontSize(widget.FontSize(c, 12)).TextColor(theme.Of(c).Muted).Padding(10)
	default:
		ui.List(c, &v.list, len(v.symbols), func(i int) {
			s := v.symbols[i]
			b := ui.ButtonBase(c).FillWidth().Height(24).Padding(0, 10).AlignItems(ui.Center).Label(s.name)
			if b.Hovered() {
				b.Background(theme.Of(c).Pill)
			}
			b.Children(func() { ui.Text(c, s.name).FontSize(widget.FontSize(c, 12)).SingleLine().Shrink(1) })
			b.OnClick(func() {
				if from, ok := a.here(); ok {
					a.cancelNav()
					a.jump(a.focus, from, s.target, s.enc)
				}
			})
		}).Grow(1).FillWidth()
	}
}
