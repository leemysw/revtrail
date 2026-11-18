// INPUT: appdir 的工具目录。
// OUTPUT: 工具版本号、InstallTimeout、Remove 与下载、校验、路径等共用部分。
// POS: 托管工具的共用基础；具体工具在 ripgrep.go / gopls.go。
// PROTOCOL: 变更时更新此头部，然后检查本包 doc.go

package tools

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"revtrail/internal/appdir"
)

// Tools Revtrail installs itself, so a tool can be had from Settings without a
// package manager. Their copies live in the application data folder beside
// the language servers Revtrail installs, and the workspace is never written to.
// As with tsrs, $REVTRAIL_RG or $REVTRAIL_GOPLS comes first, then Revtrail's copy, then
// PATH: remove the copy to go back to your own.

// The versions Revtrail downloads. Bump one and the next installation takes it;
// an installed copy is left alone until it is removed and installed again.
const (
	ripgrepVersion = "15.2.0"
	goplsVersion   = "v0.24.0"
)

// downloadLimit keeps a download out of memory's way when a server answers
// with something other than a release.
// InstallTimeout is how long an installation may take: a download, or a
// build of gopls by the Go toolchain.
const InstallTimeout = 10 * time.Minute

const downloadLimit = 64 << 20

func dir() string {
	dir, err := appdir.ConfigPath("tools")
	if err != nil {
		return ""
	}
	return dir
}

// exe is name as this platform spells its executables.
func exe(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}

func under(elem ...string) string {
	dir := dir()
	if dir == "" {
		return ""
	}
	return filepath.Join(append([]string{dir}, elem...)...)
}

func pathExists(path string) bool {
	if path == "" {
		return false
	}
	_, err := os.Lstat(path)
	return err == nil
}

func Remove(dir string) error {
	if dir == "" {
		return errors.New("cannot locate the application data folder")
	}
	return os.RemoveAll(dir)
}

func regularFile(path string) bool {
	if path == "" {
		return false
	}
	fi, err := os.Stat(path)
	return err == nil && fi.Mode().IsRegular()
}

// fetch reads url, which may be a release or its checksum.
func fetch(ctx context.Context, url string, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download failed: %s", short(err))
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download failed: %s", resp.Status)
	}
	b, err := readLimited(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, fmt.Errorf("download failed: %s", short(err))
	}
	if int64(len(b)) > limit {
		return nil, errors.New("the download is larger than expected")
	}
	return b, nil
}

// readLimited reads r, refusing a member of an archive that is too big.
func readLimited(r io.Reader) ([]byte, error) {
	b, err := io.ReadAll(io.LimitReader(r, downloadLimit+1))
	if err != nil {
		return nil, err
	}
	if len(b) > downloadLimit {
		return nil, errors.New("the download is larger than expected")
	}
	return b, nil
}

// verifyChecksum compares a download with the release's "<sha256>  <name>"
// line, so a truncated or replaced download is not installed.
func verifyChecksum(data, sum []byte) error {
	fields := strings.Fields(string(sum))
	if len(fields) == 0 {
		return errors.New("the release had no checksum")
	}
	got := sha256.Sum256(data)
	if !strings.EqualFold(fields[0], hex.EncodeToString(got[:])) {
		return errors.New("the download did not match its checksum")
	}
	return nil
}

// short keeps an error one line long enough for a settings row.
func short(err error) string {
	s := err.Error()
	if len(s) > 80 {
		s = s[:77] + "…"
	}
	return s
}
