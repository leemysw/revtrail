// INPUT: ripgrep（REVTRAIL_RG）、工作区与查找选项。
// OUTPUT: RunScoped、ListFiles、Fuzzy、ModuleRoots 等。
// POS: 文本搜索与文件列举。
// PROTOCOL: 变更时更新此头部，然后检查本包 doc.go

package search

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/sahilm/fuzzy"

	"revtrail/internal/workspace"
)

// ErrNoRipgrep is returned when ripgrep is not installed.
var ErrNoRipgrep = errors.New("ripgrep (rg) is not installed")

// FindRipgrep finds rg: $REVTRAIL_RG, then the candidates an installation added
// (in the order given), then PATH. One inside the workspace is never used.
func FindRipgrep(root string, installed ...string) (string, error) {
	var candidates []string
	if p := os.Getenv("REVTRAIL_RG"); p != "" {
		candidates = append(candidates, p)
	}
	candidates = append(candidates, installed...)
	if p, err := exec.LookPath("rg"); err == nil {
		candidates = append(candidates, p)
	}
	for _, p := range candidates {
		if !filepath.IsAbs(p) {
			continue
		}
		if rel, err := filepath.Rel(root, p); root != "" && err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			continue
		}
		if fi, err := os.Stat(p); err == nil && fi.Mode().IsRegular() {
			return p, nil
		}
	}
	return "", ErrNoRipgrep
}

// InstallHint tells how to install ripgrep on this platform.
func InstallHint() string {
	switch runtime.GOOS {
	case "darwin":
		return "brew install ripgrep"
	case "windows":
		return "winget install BurntSushi.ripgrep.MSVC"
	}
	return "install your distribution's ripgrep package"
}

// Options say what to find and where.
type Options struct {
	Query         string
	CaseSensitive bool
	WholeWord     bool
	Regexp        bool
	// Include and Exclude are comma-separated globs, as ripgrep's -g.
	Include, Exclude string
	Directory        string // absolute or relative to the project
	Shallow          bool
	// MaxMatches stops the search once this many are found; 0 is 20000.
	MaxMatches int
}

// Match is one place the query was found.
type Match struct {
	Line int // from 0
	// Col and End are bytes into the line.
	Col, End int
	// Preview is the line, or part of a long one; PreStart is where the
	// match starts in it and PreEnd where it ends.
	Preview          string
	PreStart, PreEnd int
}

// File is the matches in one file.
type File struct {
	Path    string
	Matches []Match
}

// Stats sum a search up.
type Stats struct {
	Files, Matches int
	Truncated      bool // MaxMatches was reached
}

// ignoreGlobs keep ripgrep out of the folders the tree dims and never
// scans, besides what .gitignore leaves out.
func ignoreGlobs() []string {
	var out []string
	for _, d := range workspace.IgnoredNames() {
		out = append(out, "-g", "!"+d+"/")
	}
	return out
}

func argsForPaths(o Options, paths, excludeDirs []string) []string {
	args := []string{"--json", "--max-filesize", "4M", "--max-columns", "4000", "--no-config"}
	if !o.Regexp {
		args = append(args, "--fixed-strings")
	}
	if o.CaseSensitive {
		args = append(args, "--case-sensitive")
	} else {
		args = append(args, "--ignore-case")
	}
	if o.WholeWord {
		args = append(args, "--word-regexp")
	}
	if o.Shallow {
		args = append(args, "--max-depth", "1")
	}
	args = appendGlobs(args, o.Include, o.Exclude)
	for _, dir := range excludeDirs {
		args = append(args, "-g", "!/"+globLiteral.Replace(dir)+"/**")
	}
	args = append(args, "-e", o.Query, "--")
	return append(args, paths...)
}

func appendGlobs(args []string, include, exclude string) []string {
	args = append(args, ignoreGlobs()...)
	for g := range strings.SplitSeq(include, ",") {
		if g = strings.TrimSpace(g); g != "" {
			args = append(args, "-g", g)
		}
	}
	for g := range strings.SplitSeq(exclude, ",") {
		if g = strings.TrimSpace(g); g != "" {
			args = append(args, "-g", "!"+g)
		}
	}
	return args
}

// rgEvent is the part of ripgrep's JSON lines the search reads.
type rgEvent struct {
	Type string `json:"type"`
	Data struct {
		Path       rgText `json:"path"`
		Lines      rgText `json:"lines"`
		LineNumber int    `json:"line_number"`
		Submatches []struct {
			Start int `json:"start"`
			End   int `json:"end"`
		} `json:"submatches"`
	} `json:"data"`
}

type rgText struct {
	Text string `json:"text"`
}

// FileScope narrows the search independently of the popup's file mask.
// Files contains absolute paths; a non-nil empty set matches nothing.
type FileScope struct {
	Files            map[string]bool
	ExcludeDirs      []string
	Include, Exclude string
}

func RunScoped(ctx context.Context, rg, root string, o Options, scope FileScope, emit func(File)) (Stats, error) {
	if o.Query == "" {
		return Stats{}, errors.New("empty query")
	}
	if o.Directory != "" {
		if filepath.IsAbs(o.Directory) {
			root = filepath.Clean(o.Directory)
		} else {
			root = filepath.Join(root, o.Directory)
		}
	}
	if o.MaxMatches <= 0 {
		o.MaxMatches = 20000
	}
	if scope.Files == nil && (scope.Include == "" || o.Include == "") {
		if scope.Include != "" {
			o.Include = scope.Include
		}
		o.Exclude = strings.Trim(o.Exclude+","+scope.Exclude, ",")
		return runPaths(ctx, rg, root, o, []string{"."}, scope.ExcludeDirs, emit)
	}
	files := scope.Files
	// Explicit file operands bypass ripgrep's globs. Intersect the two
	// file lists before passing those operands to the content search.
	for _, filter := range [][2]string{{scope.Include, scope.Exclude}, {o.Include, o.Exclude}} {
		if filter[0] == "" && filter[1] == "" {
			continue
		}
		matched, err := matchingFiles(ctx, rg, root, filter[0], filter[1])
		if err != nil {
			return Stats{}, err
		}
		if files != nil {
			maps.DeleteFunc(matched, func(p string, _ bool) bool { return !files[p] })
		}
		files = matched
	}
	paths := slices.Sorted(maps.Keys(files))
	if scope.Files != nil {
		// A Git change can name a submodule directory or a file removed since status was read.
		paths = slices.DeleteFunc(paths, func(p string) bool {
			info, err := os.Stat(p)
			return os.IsNotExist(err) || err == nil && !info.Mode().IsRegular()
		})
	}
	var total Stats
	for len(paths) > 0 {
		// Bound argv in bytes, including on platforms with a small command-line limit.
		end, size := 1, len(paths[0])
		for end < len(paths) && size+len(paths[end])+1 < 8000 {
			size += len(paths[end]) + 1
			end++
		}
		stats, err := runPaths(ctx, rg, root, o, paths[:end], scope.ExcludeDirs, emit)
		total.Matches += stats.Matches
		total.Files += stats.Files
		total.Truncated = stats.Truncated
		if err != nil || total.Truncated {
			return total, err
		}
		o.MaxMatches -= stats.Matches
		paths = paths[end:]
	}
	return total, nil
}

func matchingFiles(ctx context.Context, rg, root, include, exclude string) (map[string]bool, error) {
	cmd := exec.CommandContext(ctx, rg, appendGlobs([]string{"--files", "--null", "--no-config"}, include, exclude)...)
	cmd.Dir = root
	data, err := cmd.Output()
	matched := make(map[string]bool)
	if ee, ok := err.(*exec.ExitError); ok && ee.ExitCode() == 1 {
		return matched, nil
	}
	if err != nil {
		return nil, err
	}
	for raw := range bytes.SplitSeq(data, []byte{0}) {
		if len(raw) > 0 {
			matched[filepath.Join(root, string(raw))] = true
		}
	}
	return matched, nil
}

func ModuleRoots(ctx context.Context, rg, root string) ([]string, error) {
	files, err := matchingFiles(ctx, rg, root, "go.mod,*.iml", "")
	if err != nil {
		return nil, err
	}
	dirs := map[string]bool{".": true}
	for path := range files {
		rel, err := filepath.Rel(root, filepath.Dir(path))
		if err != nil {
			return nil, err
		}
		dirs[rel] = true
	}
	return slices.Sorted(maps.Keys(dirs)), nil
}

var globLiteral = strings.NewReplacer(`\`, `\\`, "*", `\*`, "?", `\?`, "[", `\[`, "{", `\{`, "}", `\}`)

func runPaths(ctx context.Context, rg, root string, o Options, paths, excludeDirs []string, emit func(File)) (Stats, error) {
	limit := o.MaxMatches
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	cmd := exec.CommandContext(ctx, rg, argsForPaths(o, paths, excludeDirs)...)
	cmd.Dir = root
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.StdoutPipe()
	if err != nil {
		return Stats{}, err
	}
	if err := cmd.Start(); err != nil {
		return Stats{}, fmt.Errorf("starting ripgrep: %w", err)
	}
	var st Stats
	var cur *File
	flush := func() {
		if cur != nil && len(cur.Matches) > 0 {
			st.Files++
			emit(*cur)
		}
		cur = nil
	}
	sc := bufio.NewScanner(out)
	sc.Buffer(make([]byte, 64<<10), 16<<20)
	for !st.Truncated && sc.Scan() {
		var ev rgEvent
		if json.Unmarshal(sc.Bytes(), &ev) != nil {
			continue
		}
		switch ev.Type {
		case "begin":
			flush()
			p := filepath.FromSlash(ev.Data.Path.Text)
			if !filepath.IsAbs(p) {
				p = filepath.Join(root, p)
			}
			cur = &File{Path: p}
		case "end":
			flush()
		case "match":
			// A line that is not UTF-8 comes as bytes, and is not shown.
			if cur == nil || ev.Data.Lines.Text == "" {
				continue
			}
			line := strings.TrimRight(ev.Data.Lines.Text, "\r\n")
			for _, sm := range ev.Data.Submatches {
				if sm.End > len(line) || sm.Start >= sm.End {
					continue
				}
				cur.Matches = append(cur.Matches, preview(ev.Data.LineNumber-1, line, sm.Start, sm.End))
				st.Matches++
				if st.Matches >= limit {
					st.Truncated = true
					break
				}
			}
		}
	}
	flush()
	if st.Truncated {
		cancel()
	}
	werr := cmd.Wait()
	switch {
	case st.Truncated:
		return st, nil
	case ctx.Err() != nil:
		return st, ctx.Err()
	case werr != nil:
		var ee *exec.ExitError
		if errors.As(werr, &ee) && ee.ExitCode() == 1 {
			return st, nil // ripgrep's "nothing found"
		}
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			msg, _, _ = strings.Cut(msg, "\n")
			return st, errors.New(msg)
		}
		return st, werr
	}
	return st, nil
}

// preview cuts a long line around a match, at character boundaries.
func preview(line int, text string, start, end int) Match {
	m := Match{Line: line, Col: start, End: end}
	from, to := 0, len(text)
	const before, after = 40, 160
	if start > before {
		from = start - before
		for from < start && !utf8.RuneStart(text[from]) {
			from++
		}
	}
	if to-end > after {
		to = end + after
		for to > end && to < len(text) && !utf8.RuneStart(text[to]) {
			to--
		}
	}
	pre := text[from:to]
	lead := min(len(pre)-len(strings.TrimLeft(pre, " \t")), start-from)
	m.Preview = pre[lead:]
	m.PreStart = start - from - lead
	m.PreEnd = min(end-from-lead, len(m.Preview))
	if from > 0 {
		m.Preview = "…" + m.Preview
		m.PreStart += len("…")
		m.PreEnd += len("…")
	}
	return m
}

// ListFiles lists the files under root for quick open, relative to it
// and separated by slashes: with `rg --files` when rg is given, which
// leaves out what .gitignore does, else by walking the tree. It stops
// at limit files. The walk also leaves out the folders skipDir names,
// by their slash-separated path relative to root, when it is given.
func ListFiles(ctx context.Context, rg, root string, limit int, skipDir func(rel string) bool) ([]string, bool, error) {
	if rg == "" {
		return walkFiles(ctx, root, limit, skipDir)
	}
	args := append([]string{"--files", "--no-config"}, ignoreGlobs()...)
	cmd := exec.CommandContext(ctx, rg, args...)
	cmd.Dir = root
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, false, err
	}
	if err := cmd.Start(); err != nil {
		return nil, false, err
	}
	var files []string
	truncated := false
	sc := bufio.NewScanner(out)
	for sc.Scan() {
		if len(files) >= limit {
			truncated = true
			break
		}
		files = append(files, filepath.ToSlash(sc.Text()))
	}
	if truncated || sc.Err() != nil {
		_ = cmd.Process.Kill()
	}
	waitErr := cmd.Wait()
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	if err := sc.Err(); err != nil {
		return nil, false, err
	}
	if waitErr != nil && !truncated {
		var exitErr *exec.ExitError
		if !errors.As(waitErr, &exitErr) || exitErr.ExitCode() != 1 {
			return nil, false, fmt.Errorf("listing workspace files: %w: %s", waitErr, strings.TrimSpace(stderr.String()))
		}
	}
	return files, truncated, nil
}

// walkFiles lists files without ripgrep.
func walkFiles(ctx context.Context, root string, limit int, skipDir func(rel string) bool) ([]string, bool, error) {
	var files []string
	truncated := false
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err != nil {
			if d != nil && d.IsDir() && p != root {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			if p != root && workspace.Ignored(d.Name()) {
				return filepath.SkipDir
			}
			if rel, err := filepath.Rel(root, p); p != root && err == nil && skipDir != nil && skipDir(filepath.ToSlash(rel)) {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		if len(files) >= limit {
			truncated = true
			return filepath.SkipAll
		}
		rel, _ := filepath.Rel(root, p)
		files = append(files, filepath.ToSlash(rel))
		return nil
	})
	return files, truncated, err
}

// Scored is a path quick open matched, best first.
type Scored struct {
	Path string
	// Hits are the bytes of Path the query's characters matched.
	Hits []int
}

// Fuzzy ranks paths against query with sahilm/fuzzy, the file's name
// counting before its folders, and returns at most limit of them. An
// empty query lists the first paths.
func Fuzzy(paths []string, query string, limit int) []Scored {
	query = strings.ReplaceAll(query, " ", "")
	if query == "" {
		out := make([]Scored, 0, min(limit, len(paths)))
		for _, p := range paths[:min(limit, len(paths))] {
			out = append(out, Scored{Path: p})
		}
		return out
	}
	ms := fuzzy.FindFrom(query, nameFirst(paths))
	out := make([]Scored, 0, min(limit, len(ms)))
	for _, m := range ms {
		if len(out) == limit {
			break
		}
		out = append(out, Scored{Path: paths[m.Index], Hits: unflip(paths[m.Index], m.MatchedIndexes)})
	}
	return out
}

// nameFirst presents paths as "name dir/", so that the library's
// preference for early and word-start matches favors the file's name.
type nameFirst []string

func (n nameFirst) Len() int { return len(n) }

func (n nameFirst) String(i int) string {
	dir, name := splitPath(n[i])
	if dir == "" {
		return name
	}
	return name + " " + dir
}

func splitPath(p string) (dir, name string) {
	i := strings.LastIndexByte(p, '/')
	return p[:i+1], p[i+1:]
}

// unflip maps the matched indexes of "name dir/" back into the path.
func unflip(p string, idx []int) []int {
	dir, name := splitPath(p)
	out := make([]int, 0, len(idx))
	for _, i := range idx {
		switch {
		case i < len(name):
			out = append(out, len(dir)+i)
		case i > len(name):
			out = append(out, i-len(name)-1)
		}
	}
	return out
}
