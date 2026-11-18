// Package arch tests the shape of the module: what lives where, and which
// package may import what. The Go compiler already refuses an import
// cycle; these rules keep the layers the design asks for, so that the tree
// does not quietly flatten back into one package.
package arch

import (
	"go/ast"
	"go/build"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// root is the module root, two directories up from internal/arch.
var root = filepath.Join("..", "..")

func goFiles(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(root, dir))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".go") {
			names = append(names, e.Name())
		}
	}
	slices.Sort(names)
	return names
}

// TestRootHoldsOnlyTheEntryPoint keeps the flat layout from coming back:
// the app's packages live under internal/, and the root holds app.go.
func TestRootHoldsOnlyTheEntryPoint(t *testing.T) {
	want := []string{"app.go"}
	if got := goFiles(t, "."); !slices.Equal(got, want) {
		t.Fatalf("the root holds %v, want only %v: move the package under internal/", got, want)
	}
}

// TestOnlyAppImportsTheWindowAPI keeps the app's state in one package: the
// packages under it may draw with mygo/ui, but a window, a menu, or the
// app's own event loop belongs to internal/app, through the port in
// internal/platform.
func TestOnlyAppImportsTheWindowAPI(t *testing.T) {
	for _, pkg := range internalPackages(t) {
		if pkg == "revtrail/internal/app" || pkg == "revtrail/internal/platform" {
			continue
		}
		info, err := build.ImportDir(filepath.Join(root, filepath.FromSlash(strings.TrimPrefix(pkg, "revtrail/"))), 0)
		if err != nil {
			t.Fatalf("%s: %v", pkg, err)
		}
		for _, imp := range info.Imports {
			if imp == "github.com/egoist/mygo" {
				t.Errorf("%s imports github.com/egoist/mygo: the window API belongs to internal/app", pkg)
			}
		}
	}
}

// TestNobodyImportsApp keeps internal/app at the top of the tree: the
// packages under it are what it composes, and none of them may depend on
// it, which is what would make a cycle the next time one of them needed a
// window.
func TestNobodyImportsApp(t *testing.T) {
	for _, pkg := range internalPackages(t) {
		if pkg == "revtrail/internal/app" {
			continue
		}
		info, err := build.ImportDir(filepath.Join(root, filepath.FromSlash(strings.TrimPrefix(pkg, "revtrail/"))), 0)
		if err != nil {
			t.Fatalf("%s: %v", pkg, err)
		}
		for _, imp := range info.Imports {
			if imp == "revtrail/internal/app" {
				t.Errorf("%s imports internal/app: the app composes its packages, not the other way round", pkg)
			}
		}
	}
}

// TestAppDoesNotRunRawGitCommands keeps git CLI subcommands and flags
// encapsulated inside internal/git instead of leaking into internal/app.
func TestAppDoesNotRunRawGitCommands(t *testing.T) {
	fset := token.NewFileSet()
	appDir := filepath.Join(root, "internal", "app")
	for _, name := range goFiles(t, filepath.Join("internal", "app")) {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, filepath.Join(appDir, name), nil, 0)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			pkg, ok := sel.X.(*ast.Ident)
			if ok && pkg.Name == "git" && (sel.Sel.Name == "Command" || sel.Sel.Name == "Network") {
				t.Errorf("%s calls git.%s: move the git operation into internal/git", fset.Position(sel.Pos()), sel.Sel.Name)
			}
			return true
		})
	}
}

// TestGitStartsProcessesInOnePlace keeps every git process of internal/git
// on the one environment git.go sets up: no locks, prompts or pagers, and
// English messages, which the package matches to classify failures.
func TestGitStartsProcessesInOnePlace(t *testing.T) {
	fset := token.NewFileSet()
	dir := filepath.Join(root, "internal", "git")
	for _, name := range goFiles(t, filepath.Join("internal", "git")) {
		if name == "git.go" || strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, 0)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			if sel, ok := n.(*ast.SelectorExpr); ok {
				if pkg, ok := sel.X.(*ast.Ident); ok && pkg.Name == "exec" && strings.HasPrefix(sel.Sel.Name, "Command") {
					t.Errorf("%s calls exec.%s: start git through command in git.go", fset.Position(sel.Pos()), sel.Sel.Name)
				}
			}
			return true
		})
	}
}

// docDirs are the directories that hold a package and so an L2 doc.go:
// the packages under internal/, locales and the tools under scripts.
// Colors are defined in internal/theme only, so both appearances stay in
// one place: everything else takes them from theme.Of(c) or theme's names.
func TestColorsLiveInTheme(t *testing.T) {
	fset := token.NewFileSet()
	for _, pkg := range internalPackages(t) {
		dir := strings.TrimPrefix(pkg, "revtrail/")
		if dir == "internal/theme" {
			continue
		}
		for _, name := range goFilesOrNone(filepath.Join(root, dir)) {
			if strings.HasSuffix(name, "_test.go") {
				continue
			}
			f, err := parser.ParseFile(fset, filepath.Join(root, dir, name), nil, 0)
			if err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			ast.Inspect(f, func(n ast.Node) bool {
				var sel *ast.SelectorExpr
				switch n := n.(type) {
				case *ast.CallExpr:
					sel, _ = n.Fun.(*ast.SelectorExpr)
				case *ast.CompositeLit:
					sel, _ = n.Type.(*ast.SelectorExpr)
				}
				if sel == nil {
					return true
				}
				if pkg, ok := sel.X.(*ast.Ident); ok && pkg.Name == "ui" && slices.Contains([]string{"RGB", "RGBA", "Hex", "Color"}, sel.Sel.Name) {
					t.Errorf("%s writes a color (ui.%s): define it in internal/theme", fset.Position(sel.Pos()), sel.Sel.Name)
				}
				return true
			})
		}
	}
}

func docDirs(t *testing.T) []string {
	t.Helper()
	var dirs []string
	for _, top := range []string{"internal", "locales", "scripts"} {
		err := filepath.WalkDir(filepath.Join(root, top), func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() && len(goFilesOrNone(path)) > 0 {
				rel, _ := filepath.Rel(root, path)
				dirs = append(dirs, filepath.ToSlash(rel))
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	slices.Sort(dirs)
	return dirs
}

// TestEveryPackageHasL2 holds the L2 of AGENTS.md's document protocol: a
// doc.go per package carrying the package comment, its parent, a line for
// each source file and the [PROTOCOL] line; no other file has a package
// comment.
func TestEveryPackageHasL2(t *testing.T) {
	for _, dir := range docDirs(t) {
		data, err := os.ReadFile(filepath.Join(root, dir, "doc.go"))
		if err != nil {
			t.Errorf("%s has no doc.go: write its L2 (see AGENTS.md)", dir)
			continue
		}
		doc := string(data)
		for _, want := range []string{"L2 | 父级:", "[PROTOCOL]:"} {
			if !strings.Contains(doc, want) {
				t.Errorf("%s/doc.go lacks %q", dir, want)
			}
		}
		fset := token.NewFileSet()
		for _, name := range goFiles(t, dir) {
			if name == "doc.go" || strings.HasSuffix(name, "_test.go") {
				continue
			}
			if !strings.Contains(doc, "- "+name+"：") {
				t.Errorf("%s/doc.go does not list %s", dir, name)
			}
			f, err := parser.ParseFile(fset, filepath.Join(root, dir, name), nil, parser.ParseComments|parser.PackageClauseOnly)
			if err != nil {
				t.Fatal(err)
			}
			if f.Doc != nil {
				t.Errorf("%s/%s has a package comment: it belongs in doc.go, leave a blank line after the L3 header", dir, name)
			}
		}
		for _, line := range strings.Split(doc, "\n") {
			if _, item, ok := strings.Cut(line, "//   - "); ok {
				name, _, _ := strings.Cut(item, "：")
				if strings.HasSuffix(name, ".go") && !strings.Contains(name, "*") {
					if _, err := os.Stat(filepath.Join(root, dir, name)); err != nil {
						t.Errorf("%s/doc.go lists %s, which does not exist", dir, name)
					}
				}
			}
		}
	}
}

// TestEverySourceFileHasL3 holds the L3 of the protocol: each source file
// but doc.go, tests and generated files opens with its INPUT, OUTPUT, POS
// and PROTOCOL lines, after a build constraint if it has one.
func TestEverySourceFileHasL3(t *testing.T) {
	for _, dir := range docDirs(t) {
		for _, name := range goFiles(t, dir) {
			if name == "doc.go" || strings.HasSuffix(name, "_test.go") {
				continue
			}
			data, err := os.ReadFile(filepath.Join(root, dir, name))
			if err != nil {
				t.Fatal(err)
			}
			lines := strings.Split(string(data), "\n")
			if strings.HasPrefix(lines[0], "// Code generated") {
				continue
			}
			if strings.HasPrefix(lines[0], "//go:build") {
				lines = lines[2:]
			}
			for i, want := range []string{"// INPUT: ", "// OUTPUT: ", "// POS: ", "// PROTOCOL: "} {
				if i >= len(lines) || !strings.HasPrefix(lines[i], want) {
					t.Errorf("%s/%s: line %d should start %q: write its L3 header (see AGENTS.md)", dir, name, i+1, want)
					break
				}
			}
		}
	}
}

// internalPackages lists the packages under internal/, as import paths.
func internalPackages(t *testing.T) []string {
	t.Helper()
	var pkgs []string
	err := filepath.WalkDir(filepath.Join(root, "internal"), func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if len(goFilesOrNone(path)) == 0 {
			return nil
		}
		pkgs = append(pkgs, "revtrail/"+filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(pkgs)
	return pkgs
}

func goFilesOrNone(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".go") {
			names = append(names, e.Name())
		}
	}
	return names
}
