// INPUT: HTML 文件的文档、所在窗口（mygo.Window.NewWebView）。
// OUTPUT: isHTML；htmlPreview：HTML 文件在网页视图里的预览，保存后重新载入；closeHTMLPreview 释放网页视图。
// POS: 编辑器预览模式中 HTML 的一种，与 Markdown、SVG 的预览并列。
// PROTOCOL: 变更时更新此头部，然后检查本包 doc.go

package app

import (
	"path/filepath"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"revtrail/internal/theme"
	"revtrail/locales"
)

func isHTML(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".html", ".htm", ".xhtml":
		return true
	}
	return false
}

// htmlView is an HTML file's preview, as IntelliJ's: the file in a web
// view, which loads it from disk, so that its pictures, styles and scripts
// next to it load too, and again each time it is saved. The page may not
// call the app.
type htmlView struct {
	web     *mygo.WebView
	err     error
	loaded  bool
	version uint64 // of the buffer the page shows
}

func (a *App) htmlPreview(c *ui.Context, g *group, d *doc) {
	hv := d.html
	if hv == nil {
		hv = &htmlView{}
		d.html = hv
		if h, ok := a.host.(*windowHost); ok {
			hv.web, hv.err = h.win.NewWebView(mygo.WebViewOptions{Page: mygo.PageOptions{Untrusted: true}})
		}
	}
	if hv.web == nil {
		ui.Column(c).Grow(1).FillWidth().Center().Children(func() {
			t := ui.Text(c, locales.HtmlPreviewUnavailable()).TextColor(theme.Of(c).Muted)
			if hv.err != nil {
				t.Tooltip(hv.err.Error())
			}
		})
		return
	}
	// Edits show once saved, as the page reads the file.
	if v := d.buf.Version(); !hv.loaded || v != hv.version && !d.buf.Modified() {
		page := hv.web.Page()
		switch {
		case d.untitled:
			page.LoadHTML(d.buf.String(), "")
		case hv.loaded:
			page.Reload() // where the page was scrolled to stays
		default:
			if err := page.LoadFile(d.path); err != nil {
				page.LoadHTML(d.buf.String(), "")
			}
		}
		hv.loaded, hv.version = true, v
	}
	view := ui.WebView(c.Key(d.path), hv.web).Grow(1).FillWidth().Label(locales.HtmlPreview())
	if a.focusGroupTarget == g && g.markdownModes[d.path] == markdownRendered {
		hv.web.Focus()
	}
	if a.focusGroupTarget == nil && view.FocusWithin() && a.focus != g {
		a.focus = g
	}
}

// closeHTMLPreview lets the web view of a document's preview go.
func (d *doc) closeHTMLPreview() {
	if d.html != nil && d.html.web != nil {
		d.html.web.Destroy()
	}
	d.html = nil
}
