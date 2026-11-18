// INPUT: 图片文件与平台剪贴板。
// OUTPUT: 图片预览与复制图片；SVG 文件按其文本画出的预览（svgPreview）。
// POS: 图片标签页与 SVG 预览的查看器。
// PROTOCOL: 变更时更新此头部，然后检查本包 doc.go

package app

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo/ui"

	"revtrail/internal/theme"
	"revtrail/internal/widget"
	"revtrail/internal/workspace"
	"revtrail/locales"
)

func isImagePath(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".bmp":
		return true
	}
	return false
}

func readImage(path string) (*ui.Bitmap, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, workspace.MaxSize+1))
	if err != nil {
		return nil, err
	}
	if len(data) > workspace.MaxSize {
		return nil, fmt.Errorf("image is too large to preview")
	}
	config, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("cannot decode image: %w", err)
	}
	if config.Width <= 0 || config.Height <= 0 || int64(config.Width)*int64(config.Height) > 64_000_000 {
		return nil, fmt.Errorf("image dimensions are too large to preview")
	}
	bitmap, err := ui.DecodeBitmap(data)
	if err != nil {
		return nil, fmt.Errorf("cannot decode image: %w", err)
	}
	return bitmap, nil
}

func (a *App) imagePreview(c *ui.Context, g *group, d *doc) {
	w, h := d.image.Size()
	preview := ui.Column(c).Grow(1).FillWidth().Padding(20).Gap(10).AlignItems(ui.Center).
		Focusable().FocusRing(false).Label(locales.MsgImagePreviewArea()).OnShortcut(ui.Cmd, ui.KeyC, func() { a.copyPreviewImage(d.path) })
	if a.focusImage == d.path && a.focus == g {
		preview.Focus()
		a.focusImage = ""
	}
	preview.Children(func() {
		ui.Box(c).Grow(1).FillWidth().MinHeight(0).Center().Children(func() {
			ui.Image(c, d.image).Fill().Fit(ui.ScaleDown).Label(locales.MsgImagePreview())
		})
		ui.Text(c, fmt.Sprintf("%d × %d · %s", w, h, widget.ByteSize(d.size))).FontSize(widget.FontSize(c, 11)).TextColor(theme.Of(c).Muted).SingleLine()
	})
}

func (a *App) copyPreviewImage(path string) {
	go func() {
		bitmap, err := imagePNG(path)
		a.post.Post(func() {
			if err == nil {
				err = a.host.CopyImage(bitmap)
			}
			if err != nil {
				a.note(locales.MsgCouldNotCopyImage()+describeErr(err), true)
			}
		})
	}()
}

func imagePNG(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, workspace.MaxSize+1))
	if err != nil {
		return nil, err
	}
	if len(data) > workspace.MaxSize {
		return nil, fmt.Errorf("image is too large to copy")
	}
	config, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	if config.Width <= 0 || config.Height <= 0 || int64(config.Width)*int64(config.Height) > 64_000_000 {
		return nil, fmt.Errorf("image dimensions are too large to copy")
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	var out bytes.Buffer
	if err := png.Encode(&out, img); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// svgPreview draws an SVG file's text as an image, as IntelliJ's preview,
// following the edits; text that does not parse says so.
func (a *App) svgPreview(c *ui.Context, g *group, d *doc) {
	if v := d.buf.Version(); !d.svgCached || v != d.svgVersion {
		d.svg, d.svgErr = ui.ParseSVG([]byte(d.buf.String()))
		d.svgVersion, d.svgCached = v, true
	}
	pane := ui.Column(c.Key(d.path)).Grow(1).FillWidth().MinHeight(0).Padding(20).Gap(10).AlignItems(ui.Center).
		Focusable().FocusRing(false).Label(locales.MsgImagePreviewArea())
	if a.focusGroupTarget == g && g.markdownModes[d.path] == markdownRendered {
		pane.Focus()
	}
	if a.focusGroupTarget == nil && pane.FocusWithin() && a.focus != g {
		a.focus = g
	}
	pane.Children(func() {
		ui.Box(c).Grow(1).FillWidth().MinHeight(0).Center().Children(func() {
			if d.svgErr != nil || d.svg == nil {
				ui.Text(c, locales.MsgSvgInvalid()).TextColor(theme.Of(c).Muted)
				return
			}
			ui.Image(c, d.svg).Fill().Fit(ui.ScaleDown).Label(locales.MsgImagePreview())
		})
		if d.svgErr == nil && d.svg != nil {
			w, h := d.svg.Size()
			ui.Text(c, fmt.Sprintf("%g × %g", w, h)).FontSize(widget.FontSize(c, 11)).TextColor(theme.Of(c).Muted).SingleLine()
		}
	})
}
