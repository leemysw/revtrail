// INPUT: ripgrep 的 GitHub Release（版本、平台资产与 sha256）。
// OUTPUT: RipgrepDir / RipgrepPath / RipgrepExists、InstallRipgrep：下载、校验、解出 rg 并原子落盘。
// POS: 托管 ripgrep 的唯一安装处。
// PROTOCOL: 变更时更新此头部，然后检查本包 doc.go

package tools

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"revtrail/internal/shellpath"
)

// RipgrepReleaseURL is where ripgrep's releases are downloaded from; a
// test points it at its own server.
var RipgrepReleaseURL = "https://github.com/BurntSushi/ripgrep/releases/download"

// RipgrepDir contains only Revtrail's ripgrep installation.
func RipgrepDir() string  { return under("ripgrep") }
func RipgrepPath() string { return under("ripgrep", exe("rg")) }
func RipgrepExists() bool { return pathExists(RipgrepDir()) }

// ripgrepAsset is the name of ripgrep's release asset for this platform.
func ripgrepAsset() (string, error) {
	arch := map[string]string{
		"arm64": "aarch64",
		"amd64": "x86_64",
	}[runtime.GOARCH]
	switch runtime.GOOS {
	case "darwin":
		if arch != "" {
			return arch + "-apple-darwin.tar.gz", nil
		}
	case "linux":
		if arch != "" {
			// Static builds, which run on any distribution.
			return arch + "-unknown-linux-musl.tar.gz", nil
		}
	case "windows":
		if arch != "" {
			return arch + "-pc-windows-msvc.zip", nil
		}
	}
	return "", fmt.Errorf("no ripgrep release for %s on %s", runtime.GOARCH, runtime.GOOS)
}

// InstallRipgrep downloads ripgrep's release for this platform, checks it
// against the release's checksum, and stages it before it is exposed.
func InstallRipgrep(ctx context.Context, dest string) error {
	if err := shellpath.Wait(ctx); err != nil {
		return err
	}
	if dest == "" {
		return errors.New("cannot locate the application data folder")
	}
	if _, err := os.Lstat(dest); err == nil {
		return errors.New("a ripgrep installation already exists; remove it first")
	} else if !os.IsNotExist(err) {
		return err
	}
	asset, err := ripgrepAsset()
	if err != nil {
		return err
	}
	url := RipgrepReleaseURL + "/" + ripgrepVersion + "/ripgrep-" + ripgrepVersion + "-" + asset
	archive, err := fetch(ctx, url, downloadLimit)
	if err != nil {
		return err
	}
	sum, err := fetch(ctx, url+".sha256", 4096)
	if err != nil {
		return err
	}
	if err := verifyChecksum(archive, sum); err != nil {
		return err
	}
	binary, err := ripgrepBinary(archive, asset)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0700); err != nil {
		return err
	}
	stage, err := os.MkdirTemp(filepath.Dir(dest), ".ripgrep-install-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	if err := os.WriteFile(filepath.Join(stage, exe("rg")), binary, 0700); err != nil {
		return err
	}
	return os.Rename(stage, dest)
}

// ripgrepBinary takes rg out of the release archive.
func ripgrepBinary(archive []byte, asset string) ([]byte, error) {
	want := exe("rg")
	if strings.HasSuffix(asset, ".zip") {
		zr, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
		if err != nil {
			return nil, errors.New("the download is not a ZIP archive")
		}
		for _, f := range zr.File {
			if filepath.Base(f.Name) != want {
				continue
			}
			rc, err := f.Open()
			if err != nil {
				return nil, err
			}
			defer rc.Close()
			return readLimited(rc)
		}
		return nil, errors.New("the download did not contain ripgrep")
	}
	zr, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return nil, errors.New("the download is not a compressed archive")
	}
	defer zr.Close()
	tr := tar.NewReader(zr)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, errors.New("the download is not a tar archive")
		}
		if h.Typeflag == tar.TypeReg && filepath.Base(h.Name) == want {
			return readLimited(tr)
		}
	}
	return nil, errors.New("the download did not contain ripgrep")
}
