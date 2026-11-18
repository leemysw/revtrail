// INPUT: 日志筛选条件。
// OUTPUT: LogFilter（分支、文本、作者、日期、路径、排序）、FilteredLog、ResolveCommit。
// POS: 日志窗口的筛选查询。
// PROTOCOL: 变更时更新此头部，然后检查本包 doc.go

package git

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// LogFilter is what the Git log window shows: Ref is a branch, HEAD, a
// range as old..new, or "" for every ref.
type LogFilter struct {
	Ref, Text, Author, Since string
	Paths                    string // paths relative to the root, one per line
	CaseSensitive, Regexp    bool
	DateOrder                bool // IntelliSort off: newest first rather than by topology
}

// paths are the filter's paths.
func (f LogFilter) paths() []string {
	var paths []string
	for p := range strings.SplitSeq(f.Paths, "\n") {
		if p = strings.TrimSpace(p); p != "" {
			paths = append(paths, p)
		}
	}
	return paths
}

// ResolveCommit is the commit a hash, branch or tag names.
func ResolveCommit(ctx context.Context, root, rev string) (string, error) {
	if err := checkRevision(rev); err != nil {
		return "", err
	}
	out, err := Command(ctx, root, "rev-parse", "--verify", "--quiet", rev+"^{commit}")
	if err != nil {
		return "", fmt.Errorf("unknown revision: %s", rev)
	}
	return strings.TrimSpace(string(out)), nil
}

func FilteredLog(ctx context.Context, root string, f LogFilter, skip, n int) ([]Commit, error) {
	refs := []string{"--all"}
	if f.Ref != "" {
		refs = []string{f.Ref}
	}
	var flags []string
	if f.Regexp {
		flags = append(flags, "--extended-regexp")
	} else {
		flags = append(flags, "--fixed-strings")
	}
	if !f.CaseSensitive {
		flags = append(flags, "--regexp-ignore-case")
	}
	if f.Author != "" {
		flags = append(flags, "--author="+f.Author)
	}
	if f.Since != "" {
		flags = append(flags, "--since="+f.Since)
	}
	if f.DateOrder {
		flags = append(flags, "--date-order")
	}
	query := strings.TrimSpace(f.Text)
	if !f.Regexp && len(query) >= 4 && len(query) <= 64 && strings.Trim(query, "0123456789abcdefABCDEF") == "" {
		out, err := Command(ctx, root, "rev-parse", "--verify", "--quiet", query+"^{commit}")
		if err == nil {
			if skip != 0 {
				return nil, nil
			}
			hash := strings.TrimSpace(string(out))
			if f.Ref != "" && !strings.Contains(f.Ref, "..") {
				if _, err = Command(ctx, root, "merge-base", "--is-ancestor", hash, f.Ref); err != nil {
					var e *exec.ExitError
					if errors.As(err, &e) && e.ExitCode() == 1 {
						return nil, nil
					}
					return nil, err
				}
			}
			return logPage(ctx, root, 0, 1, append(flags, "--no-walk"), []string{hash}, f.paths()...)
		}
	}
	if query != "" {
		flags = append(flags, "--grep="+query)
	}
	return logPage(ctx, root, skip, n, flags, refs, f.paths()...)
}
