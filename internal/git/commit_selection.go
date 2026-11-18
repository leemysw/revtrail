// INPUT: 选中的文件与块。
// OUTPUT: commitIndex、CommitSelected、WorkingFileDiff、ChangeDiff：在临时索引上按选择提交。
// POS: 部分提交的核心，不动用户的真实索引。
// PROTOCOL: 变更时更新此头部，然后检查本包 doc.go

package git

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
)

type commitIndex struct {
	root, file, base string
}

func (i commitIndex) command(ctx context.Context, args ...string) *exec.Cmd {
	cmd := command(ctx, i.root, args...)
	cmd.Env = append(cmd.Env, "GIT_INDEX_FILE="+i.file)
	return cmd
}

func (i commitIndex) run(ctx context.Context, input string, args ...string) ([]byte, error) {
	cmd := i.command(ctx, args...)
	cmd.Stdin = strings.NewReader(input)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("git %s: %w: %s", args[0], err, strings.TrimSpace(string(out)))
	}
	return out, nil
}

func commitPaths(changes []Change) []string {
	var paths []string
	for _, c := range changes {
		paths = append(paths, c.Path)
		if c.OldPath != "" && (c.Index == 'R' || c.Work == 'R') {
			paths = append(paths, c.OldPath)
		}
	}
	slices.Sort(paths)
	return slices.Compact(paths)
}

func nulPaths(paths []string) string {
	if len(paths) == 0 {
		return ""
	}
	return strings.Join(paths, "\x00") + "\x00"
}

// Build the proposed tree from HEAD and the selected working files, including new files.
func withCommitSelection(ctx context.Context, root string, changes []Change, use func(commitIndex) error) error {
	dir, err := os.MkdirTemp("", "revtrail-commit-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	i := commitIndex{root: root, file: filepath.Join(dir, "index")}
	head, err := Command(ctx, root, "rev-parse", "--verify", "--quiet", "HEAD")
	i.base = strings.TrimSpace(string(head))
	if err != nil {
		var exit *exec.ExitError
		if !errors.As(err, &exit) || exit.ExitCode() != 1 {
			return err
		}
		empty, err := Command(ctx, root, "hash-object", "-t", "tree", "--stdin")
		if err != nil {
			return err
		}
		i.base = strings.TrimSpace(string(empty))
	}
	if _, err := i.run(ctx, "", "read-tree", i.base); err != nil {
		return err
	}
	var removed, files []string
	for _, ch := range changes {
		if ch.OldPath != "" && (ch.Index == 'R' || ch.Work == 'R') {
			removed = append(removed, ch.OldPath)
		}
		files = append(files, ch.Path)
	}
	if len(removed) > 0 {
		if _, err := i.run(ctx, nulPaths(removed), "update-index", "--force-remove", "-z", "--stdin"); err != nil {
			return err
		}
	}
	if len(files) > 0 {
		if _, err := i.run(ctx, nulPaths(files), "update-index", "--add", "--remove", "--replace", "-z", "--stdin"); err != nil {
			return err
		}
	}
	for _, ch := range changes {
		if ch.Blocks.Mask != "" {
			if err := i.includeBlocks(ctx, ch); err != nil {
				return err
			}
		}
	}
	return use(i)
}

// Hold Git's index lock while preparing the two trees: the exact checked
// snapshot for the commit and the user's index with only those paths updated.
func CommitSelected(ctx context.Context, root string, changes []Change, whitespace, goFormat bool, args []string) error {
	i, err := lockIndexCopy(ctx, root)
	if err != nil {
		return err
	}
	lock := i.file
	path := strings.TrimSuffix(lock, ".lock")
	removeLock := true
	defer func() {
		if removeLock {
			os.Remove(lock)
		}
	}()
	unmerged, err := i.run(ctx, "", "ls-files", "--unmerged", "-z")
	if err != nil {
		return err
	}
	if len(unmerged) > 0 {
		return fmt.Errorf("resolve merge conflicts before committing")
	}
	paths := commitPaths(changes)
	// Git requires all staged resolutions when completing a merge or a cherry-pick.
	for _, ref := range []string{"MERGE_HEAD", "CHERRY_PICK_HEAD", "REVERT_HEAD"} {
		if _, err := i.run(ctx, "", "rev-parse", "--verify", "--quiet", ref); err != nil {
			continue
		}
		for _, ch := range changes {
			if ch.Blocks.Mask != "" {
				return fmt.Errorf("include whole files to finish %s", ref)
			}
		}
		staged, err := i.run(ctx, "", "diff", "--cached", "--no-renames", "--name-only", "-z")
		if err != nil {
			return err
		}
		for name := range strings.SplitSeq(strings.TrimRight(string(staged), "\x00"), "\x00") {
			if name != "" {
				if _, ok := slices.BinarySearch(paths, name); !ok {
					return fmt.Errorf("include %s to finish %s", name, ref)
				}
			}
		}
		break
	}
	err = withCommitSelection(ctx, root, changes, func(candidate commitIndex) error {
		i.base = candidate.base
		// When the complete working file was already staged, leave its uncommitted
		// blocks staged. Never overwrite an independently edited index version.
		var resetPaths []string
		for _, ch := range changes {
			if ch.Blocks.Mask != "" {
				indexed, err := i.filePatch(ctx, commitPaths([]Change{ch})...)
				if err != nil {
					return err
				}
				if len(indexed) != 0 {
					if PatchDigest(indexed) == ch.Blocks.Digest {
						continue
					}
					proposed, err := candidate.filePatch(ctx, commitPaths([]Change{ch})...)
					if err != nil {
						return err
					}
					if !bytes.Equal(indexed, proposed) {
						return fmt.Errorf("%s has a different staged version; review or unstage it before committing individual changes", ch.Path)
					}
				}
			}
			resetPaths = append(resetPaths, commitPaths([]Change{ch})...)
		}
		if err := candidate.check(ctx, whitespace, goFormat); err != nil {
			return err
		}
		_, err := candidate.run(ctx, "", args...)
		if err != nil {
			return err
		}
		removeLock = false
		if len(resetPaths) > 0 {
			// A pre-commit hook can format and re-add files. Reconcile against the commit it actually created.
			if _, err := i.run(ctx, nulPaths(resetPaths), "reset", "-q", "HEAD", "--pathspec-from-file=-", "--pathspec-file-nul"); err != nil {
				return fmt.Errorf("commit created; could not reconcile index %s: %w", lock, err)
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	if err := os.Rename(lock, path); err != nil {
		return fmt.Errorf("commit created; updated index retained at %s: %w", lock, err)
	}
	return nil
}

// WorkingFileDiff uses the same working-copy snapshot as the Commit panel, even
// for untracked files or files removed from the index and recreated on disk.
func WorkingFileDiff(ctx context.Context, root, path, oldPath string) ([]byte, error) {
	ch := Change{Path: path, OldPath: oldPath}
	if oldPath != "" {
		ch.Index = 'R'
	}
	var patch []byte
	err := withCommitSelection(ctx, root, []Change{ch}, func(i commitIndex) error {
		var err error
		patch, err = i.filePatch(ctx)
		return err
	})
	return patch, err
}

// ChangeDiff returns the full-context diff of path against HEAD (staged when staged is true, working copy otherwise).
func ChangeDiff(ctx context.Context, root, path, oldPath string, staged bool) ([]byte, error) {
	if !staged {
		return WorkingFileDiff(ctx, root, path, oldPath)
	}
	args := []string{"diff", "--cached", "--no-ext-diff", "--no-color", "--unified=2147483647", "--", path}
	if oldPath != "" {
		args = append(args, oldPath)
	}
	return Command(ctx, root, args...)
}

func lockIndexCopy(ctx context.Context, root string) (commitIndex, error) {
	location, err := Command(ctx, root, "rev-parse", "--path-format=absolute", "--git-path", "index")
	if err != nil {
		return commitIndex{}, err
	}
	path := strings.TrimSuffix(string(location), "\n")
	i := commitIndex{root: root, file: path + ".lock"}
	f, err := os.OpenFile(i.file, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0666)
	if err != nil {
		return i, err
	}
	src, err := os.Open(path)
	missing := os.IsNotExist(err)
	if err == nil {
		_, err = io.Copy(f, src)
		src.Close()
	}
	closeErr := f.Close()
	if missing && closeErr == nil {
		_, err = i.run(ctx, "", "read-tree", "--empty")
	}
	if closeErr != nil {
		err = closeErr
	}
	if err != nil {
		os.Remove(i.file)
	}
	return i, err
}
