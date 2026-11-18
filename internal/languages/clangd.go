// INPUT: REVTRAIL_CLANGD、REVTRAIL_CMAKE_LANGUAGE_SERVER 与 PATH。
// OUTPUT: C / C++ 与 CMake 语言服务器的查找与启动。
// POS: clangd 与 cmake-language-server 的接入。
// PROTOCOL: 变更时更新此头部，然后检查本包 doc.go

package languages

import (
	"errors"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo/ui/editor"
	"revtrail/internal/langserver"
	"revtrail/internal/lsp"
)

var ErrNoClangd = errors.New("clangd is not installed")

func ClangLanguage(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".c":
		return "c"
	case ".h", ".cc", ".cpp", ".cxx", ".c++", ".hh", ".hpp", ".hxx", ".h++", ".ipp", ".tpp":
		return "cpp"
	case ".m":
		return "objective-c"
	case ".mm":
		return "objective-cpp"
	}
	return ""
}

func FindClangd(root string) (string, error) {
	return findExecutable(root, "REVTRAIL_CLANGD", "clangd", ErrNoClangd, nil)
}

func StartClangd(logf func(string, ...any)) langserver.Starter {
	return starterFor(FindClangd, logf)
}

func NewClangdServer(root string, onState func(langserver.State, error), onDiagnostics func(*langserver.Server, string, editor.Snapshot, []lsp.Diagnostic, string), logf func(string, ...any)) *langserver.Server {
	return newRecoveringServer("clangd", root, StartClangd(logf), ClangLanguage, ErrNoClangd, FindClangd, onState, onDiagnostics, logf)
}

var ErrNoCMakeServer = errors.New("cmake-language-server is not installed")

func CMakeLanguage(path string) string {
	if filepath.Base(path) == "CMakeLists.txt" || strings.EqualFold(filepath.Ext(path), ".cmake") {
		return "cmake"
	}
	return ""
}

func FindCMakeServer(root string) (string, error) {
	return findExecutable(root, "REVTRAIL_CMAKE_LANGUAGE_SERVER", "cmake-language-server", ErrNoCMakeServer, nil)
}

func NewCMakeServer(root string, onState func(langserver.State, error), onDiagnostics func(*langserver.Server, string, editor.Snapshot, []lsp.Diagnostic, string), logf func(string, ...any)) *langserver.Server {
	return newRecoveringServer("cmake-language-server", root, starterFor(FindCMakeServer, logf), CMakeLanguage, ErrNoCMakeServer, FindCMakeServer, onState, onDiagnostics, logf)
}
