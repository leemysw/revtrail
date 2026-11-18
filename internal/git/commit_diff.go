// INPUT: 提交与临时索引。
// OUTPUT: CommitDiff：提交将带上的差异。
// POS: 提交预览。
// PROTOCOL: 变更时更新此头部，然后检查本包 doc.go

package git

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"
)

// CommitDiff is the proposed commit, including the previous commit's changes when amending.
func CommitDiff(ctx context.Context, root string, changes []Change, amend bool) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	var patch string
	err := withCommitSelection(ctx, root, changes, func(i commitIndex) error {
		var err error
		patch, err = i.diff(ctx, amend)
		return err
	})
	return patch, err
}

func (i commitIndex) diff(ctx context.Context, amend bool) (string, error) {
	args := []string{"diff", "--cached", "--no-ext-diff", "--no-textconv", "--no-color", "--stat", "--patch"}
	base := i.base
	if amend {
		line, err := Command(ctx, i.root, "rev-list", "--parents", "-1", i.base)
		if err != nil {
			return "", err
		}
		refs := strings.Fields(string(line))
		if len(refs) > 1 {
			base = refs[1]
		} else {
			empty, err := Command(ctx, i.root, "hash-object", "-t", "tree", "--stdin")
			if err != nil {
				return "", err
			}
			base = strings.TrimSpace(string(empty))
		}
	}
	cmd := i.command(ctx, append(args, base, "--")...)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", err
	}
	if err := cmd.Start(); err != nil {
		return "", err
	}
	const limit = 48 << 10
	data, readErr := io.ReadAll(io.LimitReader(stdout, limit+1))
	stdout.Close()
	err = cmd.Wait()
	if readErr != nil {
		return "", readErr
	}
	if len(data) > limit {
		return strings.ToValidUTF8(string(data[:limit]), "�") + "\n[diff truncated]", nil
	}
	if err != nil {
		return "", fmt.Errorf("git diff: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return string(data), nil
}
