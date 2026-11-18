// INPUT: REVTRAIL_GOPLS、REVTRAIL_TYPESCRIPT_LANGUAGE_SERVER、PATH 与 Go bin 目录。
// OUTPUT: gopls / tsrs 的查找与启动、可恢复的 Server。
// POS: 各语言服务器的登记处；新增语言在这里加。
// PROTOCOL: 变更时更新此头部，然后检查本包 doc.go

package languages

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/egoist/mygo/ui/editor"

	"revtrail/internal/langserver"
	"revtrail/internal/lsp"
	"revtrail/internal/shellpath"
	"revtrail/internal/uri"
	"revtrail/internal/workspace"
)

// ErrNoGopls is returned when no gopls is installed.
var ErrNoGopls = errors.New("gopls is not installed")

// FindGopls finds gopls: $REVTRAIL_GOPLS, then the copies an installation added
// (in the order given), then PATH, then Go's bin directories. A gopls inside the workspace is never used.
func FindGopls(root string, installed ...string) (string, error) {
	var candidates []string
	if p := os.Getenv("REVTRAIL_GOPLS"); p != "" {
		candidates = append(candidates, p)
	}
	candidates = append(candidates, installed...)
	if p, err := exec.LookPath("gopls"); err == nil {
		candidates = append(candidates, p)
	}
	exe := "gopls"
	if runtime.GOOS == "windows" {
		exe = "gopls.exe"
	}
	if p := os.Getenv("GOBIN"); p != "" {
		candidates = append(candidates, filepath.Join(p, exe))
	}
	if p := os.Getenv("GOPATH"); p != "" {
		for _, dir := range filepath.SplitList(p) {
			candidates = append(candidates, filepath.Join(dir, "bin", exe))
		}
	}
	if home, err := os.UserHomeDir(); err == nil {
		candidates = append(candidates, filepath.Join(home, "go", "bin", exe))
	}
	for _, p := range candidates {
		if validOutsideWorkspace(root, p) {
			return p, nil
		}
	}
	return "", ErrNoGopls
}

func validOutsideWorkspace(root, p string) bool {
	if !filepath.IsAbs(p) {
		return false
	}
	resolved, err := filepath.EvalSymlinks(p)
	if err != nil {
		return false
	}
	realRoot := root
	if r, err := filepath.EvalSymlinks(root); err == nil {
		realRoot = r
	}
	if root != "" && (workspace.Within(root, p) || workspace.Within(realRoot, resolved)) {
		return false
	}
	fi, err := os.Stat(resolved)
	return err == nil && fi.Mode().IsRegular()
}

func findExecutable(root, envVar, bin string, missing error, managed func() string) (string, error) {
	p := os.Getenv(envVar)
	if p == "" && managed != nil {
		p = managed()
	}
	if p == "" {
		p, _ = exec.LookPath(bin)
	}
	if !validOutsideWorkspace(root, p) {
		return "", missing
	}
	return p, nil
}

func startClient(ctx context.Context, root, path string, args []string, logf func(string, ...any), onExit func(error)) (langserver.Client, error) {
	return lsp.Start(ctx, path, args, lsp.Options{
		RootPath: root,
		RootURI:  uri.FromPath(root),
		Name:     filepath.Base(root),
		Logf:     logf,
		OnExit:   onExit,
	})
}

func starterFor(find func(string) (string, error), logf func(string, ...any)) langserver.Starter {
	return func(ctx context.Context, root string, onExit func(error)) (langserver.Client, error) {
		if err := shellpath.Wait(ctx); err != nil {
			return nil, err
		}
		path, err := find(root)
		if err != nil {
			return nil, err
		}
		return startClient(ctx, root, path, nil, logf, onExit)
	}
}

func newRecoveringServer(
	name, root string,
	start langserver.Starter,
	lang func(string) string,
	missing error,
	find func(string) (string, error),
	onState func(langserver.State, error),
	onDiagnostics func(*langserver.Server, string, editor.Snapshot, []lsp.Diagnostic, string),
	logf func(string, ...any),
) *langserver.Server {
	return langserver.New(langserver.Options{
		Name:          name,
		Root:          root,
		Start:         start,
		Language:      lang,
		OnState:       onState,
		OnDiagnostics: onDiagnostics,
		Logf:          logf,
		Recover: func(err error) bool {
			if !errors.Is(err, missing) {
				return false
			}
			_, findErr := find(root)
			return findErr == nil
		},
	})
}

// StartGopls is the langserver.Starter of the real gopls.
func StartGopls(logf func(string, ...any)) langserver.Starter {
	return starterFor(func(root string) (string, error) { return FindGopls(root) }, logf)
}

var ErrNoTypeScriptServer = errors.New("tsrs is not installed")

// NewTypeScriptServer returns the manager of tsrs for root. When tsrs was
// missing, it starts once tsrs is found, as another window or process may
// have installed it since.
func NewTypeScriptServer(root string, start langserver.Starter, onState func(langserver.State, error),
	onDiagnostics func(*langserver.Server, string, editor.Snapshot, []lsp.Diagnostic, string), logf func(string, ...any)) *langserver.Server {
	return newRecoveringServer("tsrs", root, start, TypeScriptLanguage, ErrNoTypeScriptServer, FindTypeScript, onState, onDiagnostics, logf)
}

func TypeScriptLanguage(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".ts":
		return "typescript"
	case ".tsx":
		return "typescriptreact"
	case ".js", ".mjs", ".cjs":
		return "javascript"
	case ".jsx":
		return "javascriptreact"
	}
	return ""
}

func FindTypeScript(root string) (string, error) {
	return findExecutable(root, "REVTRAIL_TYPESCRIPT_LANGUAGE_SERVER", "tsrs", ErrNoTypeScriptServer, func() string {
		if ManagedTypeScriptInstalled() {
			return ManagedTypeScriptEntry()
		}
		return ""
	})
}

func StartTypeScript(logf func(string, ...any)) langserver.Starter {
	return func(ctx context.Context, root string, onExit func(error)) (langserver.Client, error) {
		if err := shellpath.Wait(ctx); err != nil {
			return nil, err
		}
		path, err := FindTypeScript(root)
		if err != nil {
			return nil, err
		}
		args := []string{"--lsp", "-stdio"}
		if path == ManagedTypeScriptEntry() {
			runtimePath, err := exec.LookPath("node")
			if err != nil {
				runtimePath, err = exec.LookPath("bun")
			}
			if err != nil {
				return nil, errors.New("install Node.js or Bun to run tsrs")
			}
			args = append([]string{path}, args...)
			path = runtimePath
		}
		return startClient(ctx, root, path, args, logf, onExit)
	}
}
