// INPUT: 文件路径与修订。
// OUTPUT: Blame、FileHistory、CompareRefs、ApplyPatchText、DiffFiles。
// POS: 查看与比较类操作。
// PROTOCOL: 变更时更新此头部，然后检查本包 doc.go

package git

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// BlameLine describes the origin commit for a single 1-indexed line in a file.
type BlameLine struct {
	Hash    string
	Short   string
	Author  string
	Time    time.Time
	Summary string
	Line    int
}

// Blame annotates each line of relPath with the commit that last modified it.
func Blame(ctx context.Context, root, relPath string) ([]BlameLine, error) {
	out, err := Command(ctx, root, "blame", "--line-porcelain", "--", relPath)
	if err != nil {
		return nil, err
	}
	var result []BlameLine
	meta := make(map[string]BlameLine)
	var cur BlameLine
	for line := range strings.SplitSeq(string(out), "\n") {
		if strings.HasPrefix(line, "\t") {
			if cur.Hash != "" {
				meta[cur.Hash] = cur
				result = append(result, cur)
			}
			cur = BlameLine{}
			continue
		}
		if cur.Hash == "" {
			fields := strings.Fields(line)
			if len(fields) >= 3 && len(fields[0]) == 40 {
				lineNo, _ := strconv.Atoi(fields[2])
				if prev, ok := meta[fields[0]]; ok {
					cur = prev
				} else {
					cur = BlameLine{Hash: fields[0], Short: fields[0][:7]}
				}
				cur.Line = lineNo
			}
			continue
		}
		switch {
		case strings.HasPrefix(line, "author "):
			cur.Author = strings.TrimPrefix(line, "author ")
		case strings.HasPrefix(line, "author-time "):
			sec, _ := strconv.ParseInt(strings.TrimPrefix(line, "author-time "), 10, 64)
			cur.Time = time.Unix(sec, 0)
		case strings.HasPrefix(line, "summary "):
			cur.Summary = strings.TrimPrefix(line, "summary ")
		}
	}
	return result, nil
}

// FileHistory returns paginated commits touching relPath, following renames.
func FileHistory(ctx context.Context, root, relPath string, skip, n int) ([]Commit, error) {
	out, err := Command(ctx, root, "log", "-z", logFormat, "--follow", "--skip="+strconv.Itoa(skip), "-n", strconv.Itoa(n), "--", relPath)
	if err != nil {
		return nil, err
	}
	return ParseLog(out), nil
}

// CompareResult holds commit and file differences between two refs/revisions.
type CompareResult struct {
	Base, Target string
	Forward      []Commit   // in target, not in base (base..target)
	Backward     []Commit   // in base, not in target (target..base)
	Files        []FileStat // tree diff base..target
}

// CompareRefs reads the commits each of base and target has that the
// other has not, and the files they differ in. An empty target is the
// working tree, which has no commits of its own.
func CompareRefs(ctx context.Context, root, base, target string) (CompareResult, error) {
	res := CompareResult{Base: base, Target: target}
	if err := checkRevisions(base, target); err != nil {
		return res, err
	}
	if target != "" {
		var err error
		if res.Forward, err = logPage(ctx, root, 0, 200, nil, []string{base + ".." + target}); err != nil {
			return res, err
		}
		if res.Backward, err = logPage(ctx, root, 0, 200, nil, []string{target + ".." + base}); err != nil {
			return res, err
		}
	}
	out, err := Command(ctx, root, compareArgs([]string{"diff", "--numstat", "-z", "-M", "--no-color", "--no-ext-diff"}, base, target)...)
	res.Files = ParseNumstat(out)
	return res, err
}

// CompareFileDiff is the whole-file diff of f between base and target, or
// the working tree when target is empty.
func CompareFileDiff(ctx context.Context, root, base, target string, f FileStat) ([]byte, error) {
	if err := checkRevisions(base, target); err != nil {
		return nil, err
	}
	args := append(compareArgs([]string{"diff", "--no-ext-diff", "--no-color", "-M", "--unified=2147483647"}, base, target), "--", f.Path)
	if f.OldPath != "" {
		args = append(args, f.OldPath)
	}
	return Command(ctx, root, args...)
}

func checkRevisions(base, target string) error {
	err := checkRevision(base)
	if target != "" {
		err = errors.Join(err, checkRevision(target))
	}
	return err
}

func compareArgs(args []string, base, target string) []string {
	if target == "" {
		return append(args, base)
	}
	return append(args, base, target)
}

// ApplyPatchText applies a unified diff string to the working tree cleanly without modifying history.
func ApplyPatchText(ctx context.Context, root, patchText string) error {
	if strings.TrimSpace(patchText) == "" {
		return fmt.Errorf("patch is empty")
	}
	_, err := input(ctx, root, patchText, "apply", "--whitespace=nowarn", "-")
	return err
}

// DiffFiles returns the patch from file a to file b, which need not be
// in a repository, as git diff --no-index: empty when they are the same.
func DiffFiles(ctx context.Context, dir, a, b string) ([]byte, error) {
	out, err := Command(ctx, dir, "diff", "--no-index", "--no-ext-diff", "--no-color", "--unified=2147483647", "--", a, b)
	var exit *exec.ExitError
	if errors.As(err, &exit) && exit.ExitCode() == 1 {
		return out, nil // they differ
	}
	return out, err
}
