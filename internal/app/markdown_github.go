// INPUT: Markdown 预览里的 HTML 块、链接与图片，文件夹与网络上的图片。
// OUTPUT: 按 GitHub 显示的 HTML（对齐、标题、图片尺寸、链接图片、details 折叠、列表与表格）、带链接的图片行，以及链接跳转。
// POS: Markdown 预览对 GitHub 语法的补充；CommonMark 与 GFM 由 mygo markdown 渲染。
// PROTOCOL: 变更时更新此头部，然后检查本包 doc.go

package app

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
	"github.com/egoist/mygo/ui/markdown/parser"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"

	"revtrail/internal/glyph"
	"revtrail/internal/theme"
	"revtrail/internal/widget"
)

// markdownPreview is what a Markdown file's preview keeps between frames:
// GitHub's HTML parsed, images loaded, details opened, and the places of
// its blocks for scrolling beside the source (markdown_sync.go).
type markdownPreview struct {
	inner   *markdown.Options // for Markdown inside HTML, without markdownBlock
	html    map[string][]*html.Node
	images  map[string]*markdownImage
	details map[string]bool

	scroll        ui.ScrollState
	page          ui.Rect
	marks, shown  []previewMark
	lineOff, line int // where counting the lines of blocks got to
	// folded is set from a <details> left open by its HTML block, closed,
	// until the block that ends it: the blocks between show only when it
	// is open, as GitHub's.
	inDetails, folded bool
	srcY, viewY       float32
	synced            bool
}

type markdownImage struct {
	src     ui.ImageSource
	err     error
	loading bool
}

func (d *doc) mdPreview() *markdownPreview {
	if d.preview == nil {
		d.preview = &markdownPreview{html: map[string][]*html.Node{}, images: map[string]*markdownImage{}, details: map[string]bool{}}
	}
	return d.preview
}

// markdownBlock marks block i for scrolling beside the source and builds
// what GitHub shows otherwise than CommonMark: HTML blocks, and paragraphs
// of images that are links, as badges.
func (a *App) markdownBlock(c *ui.Context, d *doc, md *parser.Document, i int) bool {
	ps := d.mdPreview()
	b := md.Block(i)
	ps.mark(c, md.Source(), b.Start)
	if ps.inDetails && b.Parent <= 0 {
		src := strings.ToLower(md.Lines(i))
		if b.Kind == parser.HTMLBlock && strings.Contains(src, "</details>") {
			ps.inDetails = false
			if strings.TrimSpace(strings.ReplaceAll(src, "</details>", "")) == "" {
				return true
			}
		} else if ps.folded {
			return true
		}
	}
	switch b.Kind {
	case parser.HTMLBlock:
		return a.markdownHTML(c, d, md.Lines(i), fmt.Sprint(b.Start))
	case parser.Paragraph:
		imgs, ok := linkedImages(md.Inlines(i))
		if !ok {
			return false
		}
		a.markdownImageRow(c, d, imgs, ui.Start)
		return true
	}
	return false
}

// htmlImage is an image GitHub shows: its address, description, size
// (0 for the image's own) and the link around it.
type htmlImage struct {
	src, alt, href string
	w, h           float32
}

// linkedImages returns a paragraph's images when it holds images alone,
// one of them at least inside a link, as a row of badges.
func linkedImages(in []parser.Inline) ([]htmlImage, bool) {
	var imgs []htmlImage
	linked := false
	var walk func(from, to int, href string) bool
	walk = func(from, to int, href string) bool {
		for k := from; k < to; k = in[k].Next {
			n := in[k]
			switch n.Kind {
			case parser.Image:
				imgs = append(imgs, htmlImage{src: n.Dest, alt: parser.PlainText(in[k+1 : n.Next]), href: href})
			case parser.Link:
				linked = true
				if !walk(k+1, n.Next, n.Dest) {
					return false
				}
			case parser.SoftBreak, parser.LineBreak:
			case parser.Text:
				if strings.TrimSpace(n.Text) != "" {
					return false
				}
			default:
				return false
			}
		}
		return true
	}
	return imgs, walk(0, len(in), "") && linked && len(imgs) > 0
}

type htmlRender struct {
	a   *App
	d   *doc
	key string
	n   int
	// lastOpen tells whether the last details built is open.
	lastOpen bool
}

func (r *htmlRender) next() string {
	r.n++
	return r.key + "/" + strconv.Itoa(r.n)
}

// markdownHTML builds an HTML block as GitHub shows it; key names its
// elements and remembers its details.
func (a *App) markdownHTML(c *ui.Context, d *doc, src, key string) bool {
	ps := d.mdPreview()
	nodes, ok := ps.html[src]
	if !ok {
		body := &html.Node{Type: html.ElementNode, Data: "body", DataAtom: atom.Body}
		nodes, _ = html.ParseFragment(strings.NewReader(src), body)
		ps.html[src] = nodes
	}
	if len(nodes) == 0 {
		return false
	}
	r := &htmlRender{a: a, d: d, key: key}
	ui.Column(c.Key(key)).FillWidth().Gap(8).Children(func() { r.blocks(c, nodes, ui.Start) })
	lower := strings.ToLower(src)
	if strings.Count(lower, "<details") > strings.Count(lower, "</details>") {
		ps.inDetails, ps.folded = true, !r.lastOpen
	}
	return true
}

var htmlInline = map[string]bool{"a": true, "img": true, "b": true, "strong": true, "i": true, "em": true, "code": true, "span": true, "br": true,
	"sub": true, "sup": true, "kbd": true, "s": true, "del": true, "strike": true, "ins": true, "mark": true, "small": true, "picture": true, "source": true, "u": true}

func (r *htmlRender) blocks(c *ui.Context, nodes []*html.Node, align ui.Align) {
	var run []*html.Node
	flush := func() {
		if len(run) > 0 {
			r.inline(c, run, align)
			run = nil
		}
	}
	for _, n := range nodes {
		switch {
		case n.Type == html.TextNode:
			run = append(run, n)
		case n.Type == html.ElementNode && htmlInline[n.Data]:
			run = append(run, n)
		case n.Type == html.ElementNode:
			flush()
			r.block(c, n, align)
		}
	}
	flush()
}

func children(n *html.Node) []*html.Node {
	var out []*html.Node
	for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
		out = append(out, ch)
	}
	return out
}

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

func htmlAlign(n *html.Node, align ui.Align) ui.Align {
	if n.Data == "center" {
		return ui.Center
	}
	switch strings.ToLower(attr(n, "align")) {
	case "center", "middle":
		return ui.Center
	case "right":
		return ui.End
	case "left":
		return ui.Start
	}
	return align
}

func (r *htmlRender) block(c *ui.Context, n *html.Node, align ui.Align) {
	align = htmlAlign(n, align)
	switch n.Data {
	case "script", "style", "head", "title", "meta", "link", "template":
	case "h1", "h2", "h3", "h4", "h5", "h6":
		r.markdown(c, strings.Repeat("#", int(n.Data[1]-'0'))+" "+inlineMarkdown(children(n)), align)
	case "hr":
		r.markdown(c, "---", ui.Start)
	case "pre":
		r.markdown(c, "```\n"+strings.TrimRight(textOf(n), "\n")+"\n```", ui.Start)
	case "ul", "ol", "table", "blockquote", "dl":
		r.markdown(c, blockMarkdown(n, ""), align)
	case "details":
		r.details(c, n, align)
	default: // div, p, center, section and the rest hold blocks
		ui.Column(c.Key(r.next())).FillWidth().AlignItems(align).Gap(8).Children(func() { r.blocks(c, children(n), align) })
	}
}

// details is GitHub's disclosure: the summary, and the rest once open.
func (r *htmlRender) details(c *ui.Context, n *html.Node, align ui.Align) {
	key := r.next()
	ps := r.d.mdPreview()
	open, set := ps.details[key]
	if !set {
		open = hasAttr(n, "open")
	}
	r.lastOpen = open
	var summary []*html.Node
	var rest []*html.Node
	for _, ch := range children(n) {
		if ch.Type == html.ElementNode && ch.Data == "summary" && summary == nil {
			summary = children(ch)
			continue
		}
		rest = append(rest, ch)
	}
	ui.Column(c.Key(key)).FillWidth().Gap(8).Children(func() {
		head := ui.ButtonBase(c.Key("summary")).FillWidth().Gap(6).AlignItems(ui.Center)
		head.Cursor(ui.CursorPointer)
		head.Children(func() {
			chevron := glyph.Draw(c, glyph.Chevron, 10, theme.Of(c).Muted)
			if open {
				chevron.Rotate(90)
			}
			text := strings.Join(strings.Fields(textOfNodes(summary)), " ")
			if text == "" {
				text = "Details"
			}
			ui.Text(c, text).Grow(1)
		})
		head.OnClick(func() { ps.details[key] = !open })
		if open {
			ui.Column(c.Key("body")).FillWidth().Padding(0, 0, 0, 18).Gap(8).Children(func() { r.blocks(c, rest, align) })
		}
	})
}

func hasAttr(n *html.Node, key string) bool {
	for _, a := range n.Attr {
		if a.Key == key {
			return true
		}
	}
	return false
}

func (r *htmlRender) inline(c *ui.Context, run []*html.Node, align ui.Align) {
	if imgs, ok := htmlImages(run, ""); ok && len(imgs) > 0 {
		r.a.markdownImageRow(c, r.d, imgs, align)
		return
	}
	md := strings.TrimSpace(inlineMarkdown(run))
	if md == "" {
		return
	}
	r.markdown(c, md, align)
}

func (r *htmlRender) markdown(c *ui.Context, src string, align ui.Align) {
	ui.Column(c.Key(r.next())).FillWidth().AlignItems(align).Children(func() {
		v := markdown.View(c, src, r.innerOptions())
		if align == ui.Start {
			v.FillWidth()
		}
	})
}

// innerOptions render Markdown made of HTML: the preview's look and links,
// without marking blocks again.
func (r *htmlRender) innerOptions() *markdown.Options {
	ps := r.d.mdPreview()
	outer := r.d.markdownOptions
	if ps.inner == nil {
		ps.inner = &markdown.Options{LoadImage: outer.LoadImage, OnLink: outer.OnLink}
	}
	ps.inner.Theme = outer.Theme
	return ps.inner
}

// htmlImages returns the images of a run that holds images alone, with
// the links around them.
func htmlImages(nodes []*html.Node, href string) ([]htmlImage, bool) {
	var imgs []htmlImage
	for _, n := range nodes {
		switch {
		case n.Type == html.TextNode:
			if strings.TrimSpace(n.Data) != "" {
				return nil, false
			}
		case n.Type != html.ElementNode:
		case n.Data == "br", n.Data == "source":
		case n.Data == "img":
			imgs = append(imgs, htmlImage{src: attr(n, "src"), alt: attr(n, "alt"), href: href, w: htmlSize(attr(n, "width")), h: htmlSize(attr(n, "height"))})
		case n.Data == "a" || n.Data == "picture" || n.Data == "span":
			h := href
			if n.Data == "a" {
				h = attr(n, "href")
			}
			inner, ok := htmlImages(children(n), h)
			if !ok {
				return nil, false
			}
			imgs = append(imgs, inner...)
		default:
			return nil, false
		}
	}
	return imgs, true
}

// htmlSize is a width or height in pixels, 0 for none or a percentage.
func htmlSize(s string) float32 {
	v, err := strconv.ParseFloat(strings.TrimSuffix(strings.TrimSpace(s), "px"), 32)
	if err != nil || v <= 0 {
		return 0
	}
	return float32(v)
}

var markdownEscaper = strings.NewReplacer("\\", "\\\\", "*", "\\*", "_", "\\_", "[", "\\[", "]", "\\]", "<", "\\<", "`", "\\`", "#", "\\#")

// angle writes a destination between angle brackets, which keep
// parentheses and spaces from changing the syntax.
func angle(dest string) string {
	return "<" + strings.NewReplacer("\\", "%5C", "<", "%3C", ">", "%3E", " ", "%20", "\n", "%0A").Replace(dest) + ">"
}

// inlineMarkdown writes HTML text as Markdown's.
func inlineMarkdown(nodes []*html.Node) string {
	var sb strings.Builder
	for _, n := range nodes {
		switch n.Type {
		case html.TextNode:
			// HTML collapses runs of white space to one space.
			t := n.Data
			words := strings.Join(strings.Fields(t), " ")
			if t != "" && isSpace(t[0]) {
				words = " " + words
			}
			if t != "" && isSpace(t[len(t)-1]) && strings.TrimSpace(t) != "" {
				words += " "
			}
			sb.WriteString(markdownEscaper.Replace(words))
		case html.ElementNode:
			inner := inlineMarkdown(children(n))
			switch n.Data {
			case "a":
				sb.WriteString("[" + inner + "](" + angle(attr(n, "href")) + ")")
			case "img":
				sb.WriteString("![" + markdownEscaper.Replace(attr(n, "alt")) + "](" + angle(attr(n, "src")) + ")")
			case "b", "strong":
				sb.WriteString("**" + inner + "**")
			case "i", "em":
				sb.WriteString("*" + inner + "*")
			case "code", "kbd":
				sb.WriteString("`" + textOf(n) + "`")
			case "s", "del", "strike":
				sb.WriteString("~~" + inner + "~~")
			case "br":
				sb.WriteString("\\\n")
			case "sub", "sup", "mark", "ins", "u", "small":
				sb.WriteString("<" + n.Data + ">" + inner + "</" + n.Data + ">")
			case "script", "style":
			default:
				sb.WriteString(inner)
			}
		}
	}
	return sb.String()
}

// blockMarkdown writes lists, tables, quotes and definitions as Markdown's,
// each line after indent.
func blockMarkdown(n *html.Node, indent string) string {
	var sb strings.Builder
	switch n.Data {
	case "ul", "ol":
		i := 1
		if v, err := strconv.Atoi(attr(n, "start")); err == nil {
			i = v
		}
		for _, li := range children(n) {
			if li.Type != html.ElementNode || li.Data != "li" {
				continue
			}
			marker := "- "
			if n.Data == "ol" {
				marker = strconv.Itoa(i) + ". "
				i++
			}
			var text []*html.Node
			var nested strings.Builder
			for _, ch := range children(li) {
				if ch.Type == html.ElementNode && (ch.Data == "ul" || ch.Data == "ol") {
					nested.WriteString(blockMarkdown(ch, indent+strings.Repeat(" ", len(marker))))
					continue
				}
				text = append(text, ch)
			}
			sb.WriteString(indent + marker + strings.TrimSpace(inlineMarkdown(text)) + "\n" + nested.String())
		}
	case "table":
		var rows [][]string
		var walk func(*html.Node)
		walk = func(x *html.Node) {
			for _, ch := range children(x) {
				if ch.Type != html.ElementNode {
					continue
				}
				if ch.Data == "tr" {
					var cells []string
					for _, cell := range children(ch) {
						if cell.Type == html.ElementNode && (cell.Data == "td" || cell.Data == "th") {
							cells = append(cells, strings.ReplaceAll(strings.TrimSpace(inlineMarkdown(children(cell))), "|", "\\|"))
						}
					}
					rows = append(rows, cells)
					continue
				}
				walk(ch)
			}
		}
		walk(n)
		if len(rows) == 0 {
			return ""
		}
		cols := 0
		for _, row := range rows {
			cols = max(cols, len(row))
		}
		line := func(cells []string) {
			for len(cells) < cols {
				cells = append(cells, "")
			}
			sb.WriteString(indent + "| " + strings.Join(cells, " | ") + " |\n")
		}
		sep := make([]string, cols)
		for i := range sep {
			sep[i] = "---"
		}
		line(rows[0])
		line(sep)
		for _, row := range rows[1:] {
			line(row)
		}
	case "blockquote":
		for _, l := range strings.Split(strings.TrimSpace(inlineMarkdown(children(n))), "\n") {
			sb.WriteString(indent + "> " + l + "\n")
		}
	case "dl":
		for _, ch := range children(n) {
			if ch.Type != html.ElementNode {
				continue
			}
			text := strings.TrimSpace(inlineMarkdown(children(ch)))
			if ch.Data == "dt" {
				text = "**" + text + "**"
			}
			sb.WriteString(indent + text + "\n\n")
		}
	}
	return sb.String()
}

func isSpace(b byte) bool { return b == ' ' || b == '\n' || b == '\t' || b == '\r' }

func textOf(n *html.Node) string {
	if n.Type == html.TextNode {
		return n.Data
	}
	return textOfNodes(children(n))
}

func textOfNodes(nodes []*html.Node) string {
	var sb strings.Builder
	for _, n := range nodes {
		sb.WriteString(textOf(n))
	}
	return sb.String()
}

// markdownImageRow shows images side by side, wrapping, at the size their
// HTML gives, each opening its link when clicked.
func (a *App) markdownImageRow(c *ui.Context, d *doc, imgs []htmlImage, align ui.Align) {
	ui.Row(c).FillWidth().Wrap().Gap(4).Justify(align).AlignItems(ui.End).Children(func() {
		for i, im := range imgs {
			el := a.markdownImageView(c.Key(i), d, im)
			if im.href != "" {
				href := im.href
				el.Cursor(ui.CursorPointer).Tooltip(href).OnClick(func() { a.markdownLink(d, href) })
			}
		}
	})
}

func (a *App) markdownImageView(c *ui.Context, d *doc, im htmlImage) ui.Element {
	ps := d.mdPreview()
	e := ps.images[im.src]
	if e == nil {
		e = &markdownImage{loading: true}
		ps.images[im.src] = e
		load := d.markdownOptions.LoadImage
		src := im.src
		go func() {
			img, err := load(src)
			a.post.Post(func() { e.src, e.err, e.loading = img, err, false })
		}()
	}
	box := ui.ButtonBase(c).Shrink(1).MinWidth(0).Label(im.alt)
	box.Children(func() {
		if e.src == nil {
			alt := im.alt
			if alt == "" {
				alt = "image"
			}
			ui.Text(c, "["+alt+"]").FontSize(widget.FontSize(c, 12)).TextColor(theme.Of(c).Muted)
			return
		}
		w, h := ui.ImageSize(e.src)
		switch {
		case im.w > 0 && im.h > 0:
			w, h = im.w, im.h
		case im.w > 0 && w > 0:
			w, h = im.w, h*im.w/w
		case im.h > 0 && h > 0:
			w, h = w*im.h/h, im.h
		}
		ui.Image(c, e.src).Width(w).Height(h).MaxWidth(850).Fit(ui.Contain).Label(im.alt)
	})
	return box
}

// markdownLink follows a link of the preview as GitHub does: the web and
// mail outside, #a-heading to its heading, and a path, from the file's
// folder or from the project's with a leading /, to that file.
func (a *App) markdownLink(d *doc, dest string) {
	u, err := url.Parse(dest)
	if err != nil {
		return
	}
	switch {
	case u.Scheme == "http" || u.Scheme == "https" || u.Scheme == "mailto":
		a.services.OpenURL(dest)
	case u.Scheme != "":
	case u.Path == "":
		if o := d.markdownOptions; o != nil && u.Fragment != "" {
			o.Anchor, o.AnchorVersion = u.Fragment, o.AnchorVersion+1
		}
	default:
		p := filepath.FromSlash(u.Path)
		if strings.HasPrefix(u.Path, "/") && a.root != "" {
			p = filepath.Join(a.root, p)
		} else if !filepath.IsAbs(p) {
			p = filepath.Join(filepath.Dir(d.path), p)
		}
		info, err := os.Stat(p)
		switch {
		case err != nil:
		case info.IsDir():
			a.selectInProject(p)
		default:
			a.userOpen(p)
		}
	}
}
