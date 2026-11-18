package tools

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// ripgrepArchive builds a release archive as ripgrep's are: a folder with
// rg inside it.
func ripgrepArchive(t *testing.T, asset string) []byte {
	t.Helper()
	var buf bytes.Buffer
	if strings.HasSuffix(asset, ".zip") {
		zw := zip.NewWriter(&buf)
		header := &zip.FileHeader{Name: "ripgrep-" + ripgrepVersion + "/" + exe("rg")}
		header.SetMode(0755)
		file, err := zw.CreateHeader(header)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = file.Write([]byte("ripgrep fixture")); err != nil {
			t.Fatal(err)
		}
		if err := zw.Close(); err != nil {
			t.Fatal(err)
		}
		return buf.Bytes()
	}
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	body := []byte("#!/bin/sh\necho ripgrep\n")
	dir := "ripgrep-" + ripgrepVersion + "-" + strings.TrimSuffix(asset, ".tar.gz")
	if err := tw.WriteHeader(&tar.Header{Name: dir + "/", Typeflag: tar.TypeDir, Mode: 0755}); err != nil {
		t.Fatal(err)
	}
	if err := tw.WriteHeader(&tar.Header{Name: dir + "/" + exe("rg"), Typeflag: tar.TypeReg, Mode: 0755, Size: int64(len(body))}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(body); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// serveRipgrepRelease serves an archive and its checksum, as the release
// does, and points the installer at it.
func serveRipgrepRelease(t *testing.T, archive []byte, sum string) {
	t.Helper()
	asset, err := ripgrepAsset()
	if err != nil {
		t.Skip(err)
	}
	name := "ripgrep-" + ripgrepVersion + "-" + asset
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch filepath.Base(r.URL.Path) {
		case name:
			w.Write(archive)
		case name + ".sha256":
			w.Write([]byte(sum))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	old := RipgrepReleaseURL
	RipgrepReleaseURL = srv.URL
	t.Cleanup(func() { RipgrepReleaseURL = old })
}

func checksum(data []byte) string {
	b := sha256.Sum256(data)
	return hex.EncodeToString(b[:])
}

func TestInstallRipgrepInstallsReleaseBinary(t *testing.T) {
	asset, err := ripgrepAsset()
	if err != nil {
		t.Skip(err)
	}
	archive := ripgrepArchive(t, asset)
	serveRipgrepRelease(t, archive, checksum(archive)+"  ripgrep.tar.gz\n")
	dest := filepath.Join(t.TempDir(), "tools", "ripgrep")
	if err := InstallRipgrep(context.Background(), dest); err != nil {
		t.Fatal(err)
	}
	rg := filepath.Join(dest, exe("rg"))
	fi, err := os.Stat(rg)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && fi.Mode().Perm()&0100 == 0 {
		t.Fatalf("rg is not executable: %v", fi.Mode())
	}
	// A second installation does not replace the first.
	if err := InstallRipgrep(context.Background(), dest); err == nil {
		t.Fatal("overwrote an existing installation")
	}
}

func TestInstallRipgrepRejectsAnotherChecksum(t *testing.T) {
	asset, err := ripgrepAsset()
	if err != nil {
		t.Skip(err)
	}
	archive := ripgrepArchive(t, asset)
	serveRipgrepRelease(t, archive, checksum([]byte("something else"))+"  ripgrep.tar.gz\n")
	parent := t.TempDir()
	err = InstallRipgrep(context.Background(), filepath.Join(parent, "ripgrep"))
	if err == nil || !strings.Contains(err.Error(), "checksum") {
		t.Fatalf("error = %v", err)
	}
	if entries, err := os.ReadDir(parent); err != nil || len(entries) != 0 {
		t.Fatalf("a failed installation left files: %v, %v", entries, err)
	}
}

func TestInstallGoplsBuildsWithGoToolchain(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture")
	}
	bin := t.TempDir()
	script := "#!/bin/sh\necho \"$@\" > \"$GOBIN/args\"\nmkdir -p \"$GOBIN\"\nprintf gopls > \"$GOBIN/gopls\"\n"
	if err := os.WriteFile(filepath.Join(bin, "go"), []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+"/usr/bin:/bin")
	dest := filepath.Join(t.TempDir(), "tools", "gopls")
	if err := InstallGopls(context.Background(), dest); err != nil {
		t.Fatal(err)
	}
	if !regularFile(filepath.Join(dest, "bin", "gopls")) {
		t.Fatal("gopls was not installed")
	}
	args, err := os.ReadFile(filepath.Join(dest, "bin", "args"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(args)) != "install golang.org/x/tools/gopls@"+goplsVersion {
		t.Fatalf("go install arguments %q", args)
	}
	// Without a Go toolchain, the row says what is missing.
	t.Setenv("PATH", t.TempDir())
	err = InstallGopls(context.Background(), filepath.Join(t.TempDir(), "gopls"))
	if err == nil || !strings.Contains(err.Error(), "install Go first") {
		t.Fatalf("error = %v", err)
	}
}

func TestRipgrepArchivesExtractExpectedBinary(t *testing.T) {
	for _, asset := range []string{"fixture.tar.gz", "fixture.zip"} {
		t.Run(asset, func(t *testing.T) {
			data := ripgrepArchive(t, asset)
			got, err := ripgrepBinary(data, asset)
			if err != nil {
				t.Fatal(err)
			}
			want := "#!/bin/sh\necho ripgrep\n"
			if strings.HasSuffix(asset, ".zip") {
				want = "ripgrep fixture"
			}
			if string(got) != want {
				t.Fatalf("binary %q, want %q", got, want)
			}
			if _, err := ripgrepBinary(data[:len(data)/2], asset); err == nil {
				t.Fatal("accepted truncated archive")
			}
		})
	}
}
