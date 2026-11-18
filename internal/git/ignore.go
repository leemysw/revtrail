// INPUT: 仓库的 git check-ignore 输出。
// OUTPUT: Ignored：被忽略路径的查询。
// POS: 项目树与搜索的忽略规则来源。
// PROTOCOL: 变更时更新此头部，然后检查本包 doc.go

package git

import (
	"bytes"
	"context"
	"os/exec"
	"strings"
	"time"
)

// Ignored is what .gitignore and the other exclude files leave out of a
// directory of a repository: folders whole, and files one by one.
type Ignored struct {
	dirs, files map[string]bool // slash-separated, relative to the directory
}

// ReadIgnored lists what git ignores under dir, relative to it. Git
// reports a folder holding only ignored files as ignored itself; asking
// check-ignore about each answer keeps those out. Outside a repository,
// or without git, it ignores nothing.
func ReadIgnored(ctx context.Context, dir string) (*Ignored, error) {
	ig := &Ignored{dirs: map[string]bool{}, files: map[string]bool{}}
	out, err := Command(ctx, dir, "ls-files", "--others", "--ignored", "--exclude-standard", "--directory", "-z")
	if err != nil {
		return ig, err
	}
	if len(bytes.Trim(out, "\x00")) == 0 {
		return ig, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	cmd := command(ctx, dir, "check-ignore", "--stdin", "-z")
	// Not GIT_LITERAL_PATHSPECS, which check-ignore refuses.
	cmd.Env = append(cmd.Env, "GIT_LITERAL_PATHSPECS=0")
	cmd.Stdin = bytes.NewReader(out)
	confirmed, err := cmd.Output()
	if err != nil && len(confirmed) == 0 {
		// check-ignore exits 1 when nothing is ignored.
		if ee, ok := err.(*exec.ExitError); ok && ee.ExitCode() == 1 {
			return ig, nil
		}
		return ig, err
	}
	for _, p := range strings.Split(string(confirmed), "\x00") {
		switch {
		case p == "" || p == "./":
		case strings.HasSuffix(p, "/"):
			ig.dirs[strings.TrimSuffix(p, "/")] = true
		default:
			ig.files[p] = true
		}
	}
	return ig, nil
}

// Dir reports whether git ignores the folder rel, or one it is in.
func (ig *Ignored) Dir(rel string) bool {
	if ig == nil {
		return false
	}
	for p := rel; p != "" && p != "."; {
		if ig.dirs[p] {
			return true
		}
		i := strings.LastIndexByte(p, '/')
		if i < 0 {
			break
		}
		p = p[:i]
	}
	return false
}

// Has reports whether git ignores rel, a file or a folder, itself or
// through a folder it is in.
func (ig *Ignored) Has(rel string) bool {
	if ig == nil {
		return false
	}
	return ig.files[rel] || ig.Dir(rel)
}
