// INPUT: 临时索引。
// OUTPUT: commitIndex.check：提交前检查。
// POS: 按选择提交的校验步骤。
// PROTOCOL: 变更时更新此头部，然后检查本包 doc.go

package git

import (
	"bytes"
	"context"
	"fmt"
	"go/format"
	"strings"
)

func (index commitIndex) check(ctx context.Context, whitespace, goFormat bool) error {
	if whitespace {
		if _, err := index.run(ctx, "", "diff", "--cached", "--check"); err != nil {
			return err
		}
	}
	if !goFormat {
		return nil
	}
	raw, err := index.run(ctx, "", "diff", "--cached", "--raw", "--no-abbrev", "--no-renames", "--diff-filter=AMT", "-z")
	if err != nil {
		return err
	}
	fields := strings.Split(strings.TrimSuffix(string(raw), "\x00"), "\x00")
	for i := 0; i+1 < len(fields); i += 2 {
		path := fields[i+1]
		if !strings.HasSuffix(path, ".go") {
			continue
		}
		meta := strings.Fields(fields[i])
		if meta[1] != "100644" && meta[1] != "100755" {
			continue
		}
		source, err := index.run(ctx, "", "cat-file", "blob", meta[3])
		if err != nil {
			return err
		}
		formatted, err := format.Source(source)
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		if !bytes.Equal(source, formatted) {
			return fmt.Errorf("gofmt: %s", path)
		}
	}
	return nil
}
