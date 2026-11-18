// INPUT: Markdown / SVG 文件与 mygo markdown 渲染。
// OUTPUT: 预览模式（仅编辑器 / 编辑器和预览 / 仅预览）切换、hasPreview、预览的选项（图片、链接、GitHub HTML）与 Markdown 渲染视图。
// POS: Markdown 与 SVG 标签页的预览。
// PROTOCOL: 变更时更新此头部，然后检查本包 doc.go

package app

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"github.com/egoist/mygo/ui/markdown"
	"github.com/egoist/mygo/ui/markdown/parser"

	"revtrail/internal/theme"
)

const maxMarkdownImageBytes = 20 << 20

var markdownHTTPClient = &http.Client{Timeout: 15 * time.Second}

func markdownImages(dir string) func(string) (ui.ImageSource, error) {
	local := markdown.FileImages(dir)
	return func(src string) (ui.ImageSource, error) {
		u, err := url.Parse(src)
		if err != nil {
			return nil, err
		}
		if u.Scheme != "http" && u.Scheme != "https" {
			decoded, err := url.PathUnescape(src)
			if err != nil {
				return nil, err
			}
			return local(decoded)
		}
		resp, err := markdownHTTPClient.Get(src)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, fmt.Errorf("image request: %s", resp.Status)
		}
		data, err := io.ReadAll(io.LimitReader(resp.Body, maxMarkdownImageBytes+1))
		if err != nil {
			return nil, err
		}
		if len(data) > maxMarkdownImageBytes {
			return nil, fmt.Errorf("image exceeds %d bytes", maxMarkdownImageBytes)
		}
		if strings.EqualFold(filepath.Ext(u.Path), ".svg") || strings.HasPrefix(strings.TrimSpace(string(data)), "<svg") || strings.Contains(resp.Header.Get("Content-Type"), "image/svg+xml") {
			return ui.ParseSVG(data)
		}
		return ui.DecodeBitmap(data)
	}
}

// markdownOptions are the preview's: images from the file's folder and the
// web, links as GitHub follows them, and GitHub's HTML (markdownBlock).
func (a *App) markdownOptions(d *doc) *markdown.Options {
	o := &markdown.Options{LoadImage: markdownImages(filepath.Dir(d.path))}
	o.OnLink = func(dest string) { a.markdownLink(d, dest) }
	o.Block = func(c *ui.Context, md *parser.Document, i int) bool { return a.markdownBlock(c, d, md, i) }
	return o
}

func isMarkdown(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".md" || ext == ".markdown" || ext == ".mdown" || ext == ".mkd"
}

// hasPreview reports whether a file's editor offers IntelliJ's Editor
// Only, Editor and Preview and Preview Only: Markdown and SVG files.
func hasPreview(path string) bool {
	return isMarkdown(path) || strings.EqualFold(filepath.Ext(path), ".svg")
}

type markdownMode uint8

const (
	markdownSource markdownMode = iota
	markdownRendered
	markdownSideBySide
)

func (a *App) toggleMarkdownPreview(g *group) {
	if !isMarkdown(g.active) {
		return
	}
	mode := markdownRendered
	if g.markdownModes[g.active] == markdownRendered && !g.hexPreview[g.active] {
		mode = markdownSource
	}
	a.setMarkdownMode(g, mode)
}

func (a *App) setMarkdownMode(g *group, mode markdownMode) {
	a.focus = g
	delete(g.hexPreview, g.active)
	g.markdownModes[g.active] = mode
	a.saveConfigSoon()
}

func (a *App) markdownView(c *ui.Context, g *group, d *doc) {
	if !isMarkdown(d.path) {
		a.svgPreview(c, g, d)
		return
	}
	if d.markdownOptions == nil {
		d.markdownOptions = a.markdownOptions(d)
	}
	ps := d.mdPreview()
	ps.startFrame()
	opts := d.markdownOptions
	if c.Theme().Dark && a.edTheme != nil {
		if opts.Theme == nil || opts.Theme.CodeColors != a.edTheme {
			opts.Theme = &markdown.Theme{
				Text: a.edTheme.Text, Strong: a.edTheme.Text, Code: a.edTheme.Text,
				Link: a.edTheme.TokenColor(editor.TokenLink), CodeColors: a.edTheme,
				CodeBlockBackground: theme.Dark.CodeBlock,
			}
		}
	} else {
		opts.Theme = nil
	}
	if v := d.buf.Version(); !d.markdownCached || v != d.markdownVersion {
		d.markdownText, d.markdownVersion, d.markdownCached = d.buf.String(), v, true
	}
	scroll := ui.Scroll(c.Key(d.path)).Grow(1).FillWidth().Focusable().TrackScroll(&ps.scroll)
	if a.focusGroupTarget == g && g.markdownModes[d.path] == markdownRendered {
		scroll.Focus()
	}
	if a.focusGroupTarget == nil && scroll.FocusWithin() && a.focus != g {
		a.focus = g
	}
	scroll.Children(func() {
		page := ui.Column(c).FillWidth().AlignItems(ui.Center).Padding(markdownPadTop, 40, 64, 40)
		ps.page = page.LayoutBounds()
		page.Children(func() {
			markdown.View(c, d.markdownText, d.markdownOptions).FillWidth().MaxWidth(850)
		})
	})
}
