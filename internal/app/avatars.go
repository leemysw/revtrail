// INPUT: avatar 包与提交作者邮箱。
// OUTPUT: 作者头像的异步加载、缓存与绘制。
// POS: Git 日志、提交详情里的头像来源。
// PROTOCOL: 变更时更新此头部，然后检查本包 doc.go

package app

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"revtrail/internal/avatar"
	"revtrail/internal/git"
	"revtrail/internal/theme"
)

// Authors' pictures, read off the UI thread once per address and kept
// for the app's life; initials show until one comes, and where there is
// none.

type avatarEntry struct {
	bitmap  *ui.Bitmap
	loading bool
}

var (
	avatarClient = &http.Client{Timeout: 15 * time.Second}
	avatarSlots  = make(chan struct{}, 4) // downloads at a time
)

// avatarFetch is avatar.Fetch in the app; tests take the network away.
var avatarFetch = avatar.Fetch

// avatarOf is the picture of email, or nil while it is read or when
// there is none.
func (a *App) avatarOf(email string) *ui.Bitmap {
	key := strings.ToLower(strings.TrimSpace(email))
	if key == "" {
		return nil
	}
	if a.avatars == nil {
		a.avatars = map[string]*avatarEntry{}
	}
	if e := a.avatars[key]; e != nil {
		return e.bitmap
	}
	e := &avatarEntry{loading: true}
	a.avatars[key] = e
	urls := avatar.URLs(key, hostName(a.git.web) == "GitHub", 128)
	go func() {
		avatarSlots <- struct{}{}
		defer func() { <-avatarSlots }()
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		data, err := avatarFetch(ctx, avatarClient, avatar.CacheDir(), urls)
		var bmp *ui.Bitmap
		if err == nil {
			bmp, _ = ui.DecodeBitmap(data)
		}
		a.post.Post(func() {
			e.loading, e.bitmap = false, bmp
		})
	}()
	return nil
}

// personAvatar is the picture of someone, size DIPs across, as Peek's:
// the picture cut to a circle, else the initials Avatar draws for its
// own size, scaled to this one.
func (a *App) personAvatar(c *ui.Context, name, email string, size float32) ui.Element {
	box := ui.Box(c.Key("avatar:"+email)).Size(size, size).Shrink(0).Radius(size / 2).Role(ui.RoleImage).Label(name)
	if bm := a.avatarOf(email); bm != nil {
		return box.Clip().Children(func() {
			ui.Image(c, bm).Fit(ui.Cover).Size(size, size)
		})
	}
	own := c.Theme().FontSize * 2.25
	k, off := size/own, (size-own)/2
	return box.Children(func() { ui.Avatar(c, name, nil).Absolute().Left(off).Top(off).Scale(k, k) })
}

// personLine is someone of a commit in its tab: their picture, their
// name with their part where they are not the author, and under it detail.
func (a *App) personLine(c *ui.Context, p git.Person, role, detail string) {
	ui.Row(c).FillWidth().Gap(10).AlignItems(ui.Center).Children(func() {
		a.personAvatar(c, p.Name, p.Email, 24)
		ui.Column(c).Grow(1).Shrink(1).Children(func() {
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				ui.Text(c, p.Name).Bold().SingleLine()
				if role != "" {
					ui.Text(c, role).TextColor(theme.Of(c).Muted).Background(theme.Of(c).Pill).Padding(0, 5).Radius(4).SingleLine()
				} else {
					ui.Text(c, p.Email).TextColor(theme.Of(c).Muted).SingleLine().Shrink(1)
				}
			})
			ui.Text(c, detail).TextColor(theme.Of(c).Muted).SingleLine()
		})
	})
}
