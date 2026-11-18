// INPUT: 命令行参数与工作目录。
// OUTPUT: Run、OpenPath、Split：解析参数并以独立进程启动 App。
// POS: revtrail 命令的实现。
// PROTOCOL: 变更时更新此头部，然后检查本包 doc.go

package cli

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// OpenArg is the argument the app process is started with to open a path,
// which tells it apart from a command line invocation.
const OpenArg = "--revtrail-cli-open"

// Run opens what the arguments name, or explains how to. It starts the
// app and returns; see the usage errors when the arguments name nothing
// Revtrail can open.
func Run(args []string, stderr io.Writer) error {
	if len(args) > 0 && (args[0] == "--help" || args[0] == "-h") {
		_, err := fmt.Fprintln(stderr, "Usage: revtrail [file-or-folder]")
		return err
	}
	if len(args) > 0 && args[0] == "--" {
		args = args[1:]
	}
	if len(args) > 1 {
		return fmt.Errorf("usage: revtrail [file-or-folder]")
	}
	path := "."
	if len(args) == 1 {
		path = args[0]
	}
	abs, err := Openable(path)
	if err != nil {
		return err
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return err
	}
	cmd := exec.Command(exe, OpenArg, abs)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("cannot start Revtrail: %w", err)
	}
	return cmd.Process.Release()
}

// Openable resolves a path the command line names, which must be a file or
// a folder that exists.
func Openable(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	fi, err := os.Stat(abs)
	if err != nil {
		return "", fmt.Errorf("cannot open %s: %w", path, err)
	}
	if !fi.IsDir() && !fi.Mode().IsRegular() {
		return "", fmt.Errorf("cannot open %s: expected a file or folder", path)
	}
	return abs, nil
}

// Invocation reports whether the process was started as `revtrail` rather than
// as the app itself, which takes the command line.
func Invocation(arg0 string) bool {
	name := filepath.Base(arg0)
	return name == "revtrail" || strings.EqualFold(name, "revtrail.exe")
}

// OpenPath is the path an app process is asked to open by its arguments:
// after OpenArg, as `revtrail` starts it, or alone, as a file manager or a
// shell does. A relative path is taken from wd, the current directory
// when wd is "".
func OpenPath(args []string, wd string) (string, bool) {
	var path string
	switch {
	case len(args) > 1 && args[0] == OpenArg:
		path = args[1]
	case len(args) > 0 && !strings.HasPrefix(args[0], "-"):
		path = args[0]
	default:
		return "", false
	}
	if wd != "" && !filepath.IsAbs(path) {
		path = filepath.Join(wd, path)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", false
	}
	return abs, true
}

// Split is the folder to open for path, and the file in it to show, ""
// when path is a folder itself or is not there.
func Split(path string) (dir, file string) {
	if fi, err := os.Stat(path); err == nil && !fi.IsDir() {
		return filepath.Dir(path), path
	}
	return path, ""
}
