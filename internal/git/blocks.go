// INPUT: 工作区 diff 与选中的块。
// OUTPUT: BlockSelection、PatchDigest、RollbackBlock：按块提交与回滚。
// POS: 块级提交的补丁拼装。
// PROTOCOL: 变更时更新此头部，然后检查本包 doc.go

package git

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui/editor"
)

// BlockSelection is tied to the reviewed patch. Changed lines must be reviewed
// again before the same block numbers can be used.
type BlockSelection struct {
	Digest string
	Mask   string // one '1' or '0' per consecutive run of changed lines
}

func PatchDigest(raw []byte) string { return fmt.Sprintf("%x", sha256.Sum256(raw)) }

// blockText reconstructs one version of a full-context working diff. Retain
// the new lines of checked blocks and the old lines of unchecked blocks.
func blockText(p editor.FilePatch, mask string) (string, error) {
	var b strings.Builder
	n := 0
	for _, h := range p.Hunks {
		inBlock, written, newline := false, false, false
		for _, line := range h.Lines {
			if line[0] == '\\' {
				if written {
					newline = false
				}
				continue
			}
			changed := line[0] != ' '
			if changed && !inBlock {
				n++
			}
			inBlock = changed
			if n > len(mask) {
				return "", fmt.Errorf("diff block selection changed")
			}
			written = !changed || (line[0] == '+') == (mask[n-1] == '1')
			if written {
				if newline {
					b.WriteByte('\n')
				}
				b.WriteString(line[1:])
				newline = true
			}
		}
		if newline {
			b.WriteByte('\n')
		}
	}
	if len(mask) != n {
		return "", fmt.Errorf("diff block selection changed")
	}
	return b.String(), nil
}

func (i commitIndex) filePatch(ctx context.Context, paths ...string) ([]byte, error) {
	args := []string{"diff", "--cached", "-M", "--no-ext-diff", "--no-textconv", "--no-color", "--unified=2147483647", i.base, "--"}
	return i.run(ctx, "", append(args, paths...)...)
}

func (i commitIndex) includeBlocks(ctx context.Context, ch Change) error {
	raw, err := i.filePatch(ctx, commitPaths([]Change{ch})...)
	if err != nil {
		return err
	}
	if PatchDigest(raw) != ch.Blocks.Digest {
		return fmt.Errorf("%s: diff changed; reopen it and review the blocks", ch.Path)
	}
	patches, err := editor.ParsePatch(string(raw))
	if err != nil {
		return err
	}
	if len(patches) != 1 {
		return fmt.Errorf("%s: expected one file diff", ch.Path)
	}
	content, err := blockText(patches[0], ch.Blocks.Mask)
	if err != nil {
		return err
	}
	entry, err := i.run(ctx, "", "ls-files", "--stage", "-z", "--", ch.Path)
	if err != nil {
		return err
	}
	mode, _, _ := strings.Cut(string(entry), " ")
	if mode != "100644" && mode != "100755" {
		return fmt.Errorf("%s: partial commits require a regular file", ch.Path)
	}
	oid, err := i.run(ctx, content, "hash-object", "-w", "--stdin")
	if err != nil {
		return err
	}
	_, err = i.run(ctx, "", "update-index", "--cacheinfo", mode, strings.TrimSpace(string(oid)), ch.Path)
	return err
}

// RollbackBlock reverses just the chosen change, with the rest of the file
// as context. Git checks that context before updating the working file.
func RollbackBlock(ctx context.Context, root, name, digest string, index int) error {
	raw, err := WorkingFileDiff(ctx, root, name, "")
	if err != nil {
		return err
	}
	if PatchDigest(raw) != digest {
		return fmt.Errorf("%s: diff changed; reopen it before rolling back", name)
	}
	files, err := editor.ParsePatch(string(raw))
	if err != nil {
		return err
	}
	if len(files) != 1 || files[0].OldName != name || files[0].NewName != name || len(files[0].Hunks) != 1 {
		return fmt.Errorf("%s: expected a modified text file", name)
	}
	h := files[0].Hunks[0]
	var lines []string
	block, oldCount, inBlock, keep := -1, 0, false, false
	for _, line := range h.Lines {
		if line[0] == '\\' {
			if keep {
				lines = append(lines, line)
			}
			continue
		}
		changed := line[0] != ' '
		if changed && !inBlock {
			block++
		}
		inBlock = changed
		keep = !(changed && block != index && line[0] == '-')
		if !keep {
			continue
		}
		if changed && block != index {
			line = " " + line[1:]
		}
		if line[0] != '+' {
			oldCount++
		}
		lines = append(lines, line)
	}
	if index < 0 || index > block {
		return fmt.Errorf("invalid diff block")
	}
	oldStart := 1
	if oldCount == 0 {
		oldStart = 0
	}
	header := fmt.Sprintf("--- %s\n+++ %s\n@@ -%d,%d +%d,%d @@\n", strconv.Quote("a/"+name), strconv.Quote("b/"+name), oldStart, oldCount, h.NewStart, h.NewLines)
	cmd := command(ctx, root, "apply", "--reverse", "--whitespace=nowarn", "-")
	cmd.Stdin = strings.NewReader(header + strings.Join(lines, "\n") + "\n")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}
