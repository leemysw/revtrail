// INPUT: Go 工具链（PATH 中的 go）。
// OUTPUT: GoplsDir / GoplsPath / GoplsExists、GoplsInstallCommand、InstallGopls：用 go install 构建到暂存目录再原子落盘。
// POS: 托管 gopls 的唯一安装处。
// PROTOCOL: 变更时更新此头部，然后检查本包 doc.go

package tools

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"revtrail/internal/shellpath"
)

// GoplsDir contains only Revtrail's gopls installation, built by the Go
// toolchain into its bin folder.
func GoplsDir() string  { return under("gopls") }
func GoplsPath() string { return under("gopls", "bin", exe("gopls")) }
func GoplsExists() bool { return pathExists(GoplsDir()) }

// GoplsInstallCommand is how gopls is installed without Revtrail, shown in
// Settings beside Revtrail's own installation.
func GoplsInstallCommand() string {
	return "go install golang.org/x/tools/gopls@" + goplsVersion
}

// InstallGopls builds gopls into a stage with the Go toolchain, which is
// what ships gopls: installing Go alone does not bring it.
func InstallGopls(ctx context.Context, dest string) error {
	if err := shellpath.Wait(ctx); err != nil {
		return err
	}
	if dest == "" {
		return errors.New("cannot locate the application data folder")
	}
	if _, err := os.Lstat(dest); err == nil {
		return errors.New("a gopls installation already exists; remove it first")
	} else if !os.IsNotExist(err) {
		return err
	}
	gocmd, err := exec.LookPath("go")
	if err != nil {
		return errors.New("install Go first: gopls is built by the Go toolchain")
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0700); err != nil {
		return err
	}
	stage, err := os.MkdirTemp(filepath.Dir(dest), ".gopls-install-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	bin := filepath.Join(stage, "bin")
	if err := os.MkdirAll(bin, 0700); err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, gocmd, "install", "golang.org/x/tools/gopls@"+goplsVersion)
	cmd.Env = append(os.Environ(), "GOBIN="+bin)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("installation failed: %s", lastLine(string(output)))
	}
	if !regularFile(filepath.Join(bin, exe("gopls"))) {
		return errors.New("installation did not contain gopls")
	}
	return os.Rename(stage, dest)
}

// lastLine is the last thing a toolchain said, which is where it says what
// went wrong.
func lastLine(output string) string {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if line := strings.TrimSpace(lines[i]); line != "" {
			return short(errors.New(line))
		}
	}
	return "no output"
}
