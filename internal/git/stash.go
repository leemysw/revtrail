// INPUT: 仓库根与储藏引用。
// OUTPUT: 储藏的列出（含提交哈希）、文件（含未跟踪文件）、保存、查看、应用、建分支、删除与清空。
// POS: 储藏。
// PROTOCOL: 变更时更新此头部，然后检查本包 doc.go

package git

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// StashEntry represents a saved entry in the Git stash list.
type StashEntry struct {
	Ref     string
	Hash    string // the stash commit, which stays when later stashes renumber Ref
	Branch  string
	Message string
	Date    time.Time
}

// Stashes lists the repository's saved stash entries, newest first.
func Stashes(ctx context.Context, root string) ([]StashEntry, error) {
	out, err := Command(ctx, root, "stash", "list", "--format=%gd%x00%at%x00%H%x00%gs")
	if err != nil {
		return nil, err
	}
	var entries []StashEntry
	for line := range strings.SplitSeq(strings.TrimRight(string(out), "\r\n"), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\x00", 4)
		if len(parts) < 4 {
			continue
		}
		sec, _ := strconv.ParseInt(parts[1], 10, 64)
		branch, msg := parseStashSubject(parts[3])
		entries = append(entries, StashEntry{
			Ref:     parts[0],
			Hash:    parts[2],
			Branch:  branch,
			Message: msg,
			Date:    time.Unix(sec, 0),
		})
	}
	return entries, nil
}

func parseStashSubject(s string) (branch, msg string) {
	for _, prefix := range []string{"WIP on ", "On "} {
		if rest, ok := strings.CutPrefix(s, prefix); ok {
			if br, body, found := strings.Cut(rest, ": "); found {
				return br, body
			}
		}
	}
	return "", s
}

func validateStashRef(ref string) (string, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "stash@{0}", nil
	}
	if strings.HasPrefix(ref, "-") || !strings.HasPrefix(ref, "stash@{") {
		return "", fmt.Errorf("invalid stash reference: %q", ref)
	}
	return ref, nil
}

// StashSave saves local modifications onto the stash stack.
func StashSave(ctx context.Context, root, message string, includeUntracked, keepIndex bool) error {
	args := []string{"stash", "push"}
	if includeUntracked {
		args = append(args, "-u")
	}
	if keepIndex {
		args = append(args, "-k")
	}
	if msg := strings.TrimSpace(message); msg != "" {
		args = append(args, "-m", msg)
	}
	cmd := command(ctx, root, args...)
	// GIT_LITERAL_PATHSPECS=1 disables Git's internal :(exclude) pathspec used by stash -u cleanup.
	cmd.Env = append(cmd.Env, "GIT_LITERAL_PATHSPECS=0")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	if strings.Contains(string(out), "No local changes to save") {
		return errors.New("no local changes to save")
	}
	return nil
}

// StashShow returns the stat summary and unified patch for a stash entry.
func StashShow(ctx context.Context, root, ref string) (string, error) {
	v, err := validateStashRef(ref)
	if err != nil {
		return "", err
	}
	out, err := Command(ctx, root, "stash", "show", "-p", "--stat", v)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// StashFiles are what a stash entry holds: the files it changed against
// the commit it was made on, and the untracked files it saved (with
// --include-untracked), which its third parent keeps.
func StashFiles(ctx context.Context, root, hash string) (changed, untracked []FileStat, err error) {
	d, err := Show(ctx, root, hash)
	if err != nil {
		return nil, nil, err
	}
	out, err := Command(ctx, root, "rev-parse", "--verify", "--quiet", hash+"^3")
	if err != nil || strings.TrimSpace(string(out)) == "" {
		return d.Files, nil, nil // no untracked files saved
	}
	out, err = Command(ctx, root, "ls-tree", "-r", "-z", "--name-only", hash+"^3")
	if err != nil {
		return nil, nil, err
	}
	for p := range strings.SplitSeq(strings.TrimRight(string(out), "\x00"), "\x00") {
		if p != "" {
			untracked = append(untracked, FileStat{Path: p, Added: -1, Deleted: -1, Status: 'A'})
		}
	}
	return d.Files, untracked, nil
}

// StashApply applies (or pops) the specified stash entry onto the working tree.
func StashApply(ctx context.Context, root, ref string, pop, index bool) error {
	v, err := validateStashRef(ref)
	if err != nil {
		return err
	}
	sub := "apply"
	if pop {
		sub = "pop"
	}
	args := []string{"stash", sub}
	if index {
		args = append(args, "--index")
	}
	return local(ctx, root, append(args, v)...)
}

// StashBranch creates and checks out a new branch starting at the commit from which ref was created, then pops ref.
func StashBranch(ctx context.Context, root, branchName, ref string) error {
	branchName = strings.TrimSpace(branchName)
	if err := checkBranchName(ctx, root, branchName); err != nil {
		return err
	}
	v, err := validateStashRef(ref)
	if err != nil {
		return err
	}
	_, err = Command(ctx, root, "stash", "branch", "--", branchName, v)
	return err
}

// StashDrop removes a single stash entry.
func StashDrop(ctx context.Context, root, ref string) error {
	v, err := validateStashRef(ref)
	if err != nil {
		return err
	}
	_, err = Command(ctx, root, "stash", "drop", v)
	return err
}

// StashClear removes all stash entries.
func StashClear(ctx context.Context, root string) error {
	_, err := Command(ctx, root, "stash", "clear")
	return err
}
