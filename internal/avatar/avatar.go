// INPUT: 作者邮箱、网络与 appdir 缓存目录。
// OUTPUT: URLs、Fetch：GitHub / Gravatar 头像，磁盘缓存一周。
// POS: 头像的下载与缓存。
// PROTOCOL: 变更时更新此头部，然后检查本包 doc.go

package avatar

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"revtrail/internal/appdir"
)

var noreply = regexp.MustCompile(`^(?:(\d+)\+)?([^@]+)@users\.noreply\.github\.com$`)

// URLs are where to look for the picture of email, in order: GitHub's
// for one of its noreply addresses, or for any address of a repository
// on GitHub, then Gravatar's, which answers 404 for an address it does
// not know.
func URLs(email string, onGitHub bool, size int) []string {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return nil
	}
	if size <= 0 {
		size = 64
	}
	s := strconv.Itoa(size)
	if m := noreply.FindStringSubmatch(email); m != nil {
		if m[1] != "" {
			return []string{"https://avatars.githubusercontent.com/u/" + m[1] + "?s=" + s}
		}
		return []string{"https://github.com/" + url.PathEscape(m[2]) + ".png?size=" + s}
	}
	var urls []string
	if onGitHub {
		urls = append(urls, "https://avatars.githubusercontent.com/u/e?email="+url.QueryEscape(email)+"&s="+s)
	}
	sum := sha256.Sum256([]byte(email))
	return append(urls, "https://www.gravatar.com/avatar/"+hex.EncodeToString(sum[:])+"?d=404&s="+s)
}

// ErrNone is Fetch's answer for an address with no picture anywhere.
var ErrNone = errors.New("no avatar")

// cacheTTL is how long a picture, or the knowledge that there is none,
// is kept.
const cacheTTL = 7 * 24 * time.Hour

// Fetch returns the picture at the first of urls that has one, from the
// cache in dir ("" for none) while it is fresh.
func Fetch(ctx context.Context, client *http.Client, dir string, urls []string) ([]byte, error) {
	if len(urls) == 0 {
		return nil, ErrNone
	}
	sum := sha256.Sum256([]byte(strings.Join(urls, "\n")))
	file := ""
	if dir != "" {
		file = filepath.Join(dir, hex.EncodeToString(sum[:16]))
		if fi, err := os.Stat(file); err == nil && time.Since(fi.ModTime()) < cacheTTL {
			data, err := os.ReadFile(file)
			if err == nil && len(data) == 0 {
				return nil, ErrNone
			}
			if err == nil {
				return data, nil
			}
		}
	}
	data, err := download(ctx, client, urls)
	if err != nil && !errors.Is(err, ErrNone) {
		return nil, err // offline, say: ask again next time
	}
	if file != "" && os.MkdirAll(dir, 0o755) == nil {
		_ = os.WriteFile(file, data, 0o644) // empty for none
	}
	return data, err
}

func download(ctx context.Context, client *http.Client, urls []string) ([]byte, error) {
	for _, u := range urls {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			return nil, err
		}
		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		resp.Body.Close()
		if err != nil {
			return nil, err
		}
		if resp.StatusCode == http.StatusOK && strings.HasPrefix(resp.Header.Get("Content-Type"), "image/") {
			return data, nil
		}
		if resp.StatusCode != http.StatusNotFound {
			return nil, errors.New(resp.Status)
		}
	}
	return nil, ErrNone
}

// CacheDir is where pictures are kept, or "".
func CacheDir() string {
	dir, err := appdir.CachePath("avatars")
	if err != nil {
		return ""
	}
	return dir
}
