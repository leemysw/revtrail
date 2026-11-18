// INPUT: 仓库根与分支名。
// OUTPUT: 分支、标签的列出、切换、新建、重命名、删除与恢复；MyBranches（最新提交由本人所作的分支）、UserName。
// POS: 分支操作。
// PROTOCOL: 变更时更新此头部，然后检查本包 doc.go

package git

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Branch is a local or remote-tracking branch.
type Branch struct {
	Name    string // short: main, origin/main
	Remote  bool
	Current bool
	// RemoteName is the remote a remote-tracking branch belongs to, which
	// may itself contain a slash.
	RemoteName string
	// Upstream is the branch a local one tracks, and Track how far apart
	// they are, as git writes it: "ahead 2, behind 1", "gone" or "".
	Upstream, Track string
	Hash            string
	Date            time.Time // of its last commit
	Subject         string
}

// Branches lists the repository's branches: the current one, the other
// local ones, then remote ones, each kind newest first. Remote HEADs are
// left out.
func Branches(ctx context.Context, root string) ([]Branch, error) {
	out, err := Command(ctx, root, "for-each-ref", "--sort=-committerdate",
		"--format=%(refname)%00%(HEAD)%00%(upstream:short)%00%(upstream:track,nobracket)%00%(committerdate:unix)%00%(objectname)%00%(subject)",
		"refs/heads", "refs/remotes")
	if err != nil {
		return nil, err
	}
	remotes := Remotes(ctx, root)
	var local, remote []Branch
	for line := range strings.SplitSeq(string(out), "\n") {
		f := strings.Split(line, "\x00")
		if len(f) < 7 {
			continue
		}
		b := Branch{Current: f[1] == "*", Upstream: f[2], Track: f[3], Hash: f[5], Subject: f[6]}
		if sec, err := strconv.ParseInt(f[4], 10, 64); err == nil {
			b.Date = time.Unix(sec, 0)
		}
		if name, ok := strings.CutPrefix(f[0], "refs/heads/"); ok {
			b.Name = name
			local = append(local, b)
		} else if name, ok := strings.CutPrefix(f[0], "refs/remotes/"); ok && !strings.HasSuffix(name, "/HEAD") {
			b.Name, b.Remote = name, true
			b.RemoteName, _ = splitRemote(remotes, name)
			remote = append(remote, b)
		}
	}
	slices.SortStableFunc(local, func(a, b Branch) int {
		switch {
		case a.Current == b.Current:
			return 0
		case a.Current:
			return -1
		}
		return 1
	})
	return append(local, remote...), nil
}

// LocalName is the local branch a remote one checks out as: origin/feat/x
// is feat/x.
func (b Branch) LocalName() string {
	if !b.Remote {
		return b.Name
	}
	if b.RemoteName != "" {
		return strings.TrimPrefix(b.Name, b.RemoteName+"/")
	}
	_, rest, _ := strings.Cut(b.Name, "/")
	return rest
}

// RemoteBranch is the remote-tracking branch of short name, as
// origin/feat/x, among the repository's remotes.
func RemoteBranch(remotes []string, name string) Branch {
	remote, _ := splitRemote(remotes, name)
	return Branch{Name: name, Remote: true, RemoteName: remote}
}

// splitRemote splits a short remote-tracking name like origin/feat/x by
// the longest configured remote it starts with, so a remote with a slash
// in its name resolves.
func splitRemote(remotes []string, name string) (remote, branch string) {
	for _, r := range remotes {
		if strings.HasPrefix(name, r+"/") && len(r) > len(remote) {
			remote = r
		}
	}
	if remote == "" {
		remote, branch, _ = strings.Cut(name, "/")
		return remote, branch
	}
	return remote, strings.TrimPrefix(name, remote+"/")
}

func checkBranchName(ctx context.Context, root, name string) error {
	if name == "" {
		return errors.New("a branch needs a name")
	}
	if _, err := Command(ctx, root, "check-ref-format", "--branch", name); err != nil {
		return errors.New("“" + name + "” is not a valid branch name")
	}
	return nil
}

// UserName is the configured user.name, "" without one.
func UserName(ctx context.Context, root string) string {
	out, _ := Command(ctx, root, "config", "user.name")
	return strings.TrimSpace(string(out))
}

// MyBranches are the local and remote branches, by full ref name, whose
// latest commit the configured user.email authored: IntelliJ's Show My
// Branches.
func MyBranches(ctx context.Context, root string) (map[string]bool, error) {
	email, err := Command(ctx, root, "config", "user.email")
	me := strings.ToLower(strings.TrimSpace(string(email)))
	if err != nil || me == "" {
		return map[string]bool{}, nil
	}
	out, err := Command(ctx, root, "for-each-ref", "--format=%(refname)%00%(authoremail)", "refs/heads", "refs/remotes")
	if err != nil {
		return nil, err
	}
	mine := map[string]bool{}
	for line := range strings.SplitSeq(strings.TrimSpace(string(out)), "\n") {
		ref, author, _ := strings.Cut(line, "\x00")
		if strings.ToLower(strings.Trim(author, "<>")) == me {
			mine[ref] = true
		}
	}
	return mine, nil
}

func checkRevision(rev string) error {
	if rev == "" || strings.HasPrefix(rev, "-") {
		return fmt.Errorf("invalid revision: %q", rev)
	}
	return nil
}

// Switch checks out branch b. A remote branch checks out through the
// local branch of its name, created to track it when there is none.
func Switch(ctx context.Context, root string, b Branch, mode LocalChanges) error {
	name := b.LocalName()
	args := []string{"switch", name}
	if b.Remote {
		if _, err := Command(ctx, root, "rev-parse", "--verify", "--quiet", "refs/heads/"+name); err != nil {
			args = []string{"switch", "--track", "-c", name, b.Name}
		}
	}
	return checkout(ctx, root, mode, args...)
}

// Detach checks out the commit ref names, with HEAD detached.
func Detach(ctx context.Context, root, ref string, mode LocalChanges) error {
	hash, err := Command(ctx, root, "rev-parse", "--verify", "--end-of-options", ref+"^{commit}")
	if err != nil {
		return err
	}
	return checkout(ctx, root, mode, "switch", "--detach", strings.TrimSpace(string(hash)))
}

// CreateBranch makes branch name at start, HEAD when "", and switches to
// it when checkout is set.
func CreateBranch(ctx context.Context, root, name, start string, checkout bool) error {
	name = strings.TrimSpace(name)
	if err := checkBranchName(ctx, root, name); err != nil {
		return err
	}
	args := []string{"branch", "--", name}
	if checkout {
		args = []string{"switch", "-c", name}
	}
	if start != "" {
		args = append(args, start)
	}
	_, err := Command(ctx, root, args...)
	return err
}

// RenameBranch renames oldName to newName.
func RenameBranch(ctx context.Context, root, oldName, newName string) error {
	newName = strings.TrimSpace(newName)
	if err := checkBranchName(ctx, root, newName); err != nil {
		return err
	}
	_, err := Command(ctx, root, "branch", "-m", "--", oldName, newName)
	return err
}

// ErrBranchUnmerged is DeleteBranch's error, without force, for a branch
// whose commits no other branch holds.
var ErrBranchUnmerged = errors.New("branch is not fully merged")

// DeleteBranch deletes a local branch and returns the commit it named,
// for restoring it.
func DeleteBranch(ctx context.Context, root, name string, force bool) (string, error) {
	hash, err := Command(ctx, root, "rev-parse", "--verify", "--quiet", "refs/heads/"+name)
	if err != nil {
		return "", err
	}
	flag := "-d"
	if force {
		flag = "-D"
	}
	_, err = Command(ctx, root, "branch", flag, "--", name)
	if err != nil && !force && strings.Contains(err.Error(), "not fully merged") {
		return "", fmt.Errorf("%w: %v", ErrBranchUnmerged, err)
	}
	return strings.TrimSpace(string(hash)), err
}

// RestoreBranch recreates a deleted branch at hash, with the upstream it
// tracked, as after DeleteBranch.
func RestoreBranch(ctx context.Context, root, name, hash, upstream string) error {
	if _, err := Command(ctx, root, "branch", "--", name, hash); err != nil {
		return err
	}
	if upstream == "" {
		return nil
	}
	_, err := Command(ctx, root, "branch", "--set-upstream-to", upstream, "--", name)
	return err
}

// DeleteRemoteBranch deletes a branch from its remote, given as its short
// remote-tracking name: origin/feat/x.
func DeleteRemoteBranch(ctx context.Context, root, remoteBranch string) error {
	remote, branch := splitRemote(Remotes(ctx, root), remoteBranch)
	if remote == "" || branch == "" {
		return fmt.Errorf("cannot resolve remote for %q", remoteBranch)
	}
	_, err := Network(ctx, root, "push", "--", remote, ":refs/heads/"+branch)
	return err
}

// UpdateBranch brings b up to its upstream: the current branch as Pull
// does with mode, another one by fast-forward without checking it out.
func UpdateBranch(ctx context.Context, root string, b Branch, mode string) error {
	if b.Current {
		return Pull(ctx, root, mode)
	}
	if b.Upstream == "" {
		return errors.New("branch tracks no remote branch")
	}
	remote, branch := splitRemote(Remotes(ctx, root), b.Upstream)
	if remote == "" || branch == "" {
		return fmt.Errorf("cannot resolve upstream %q", b.Upstream)
	}
	_, err := Network(ctx, root, "fetch", "--", remote, "refs/heads/"+branch+":refs/heads/"+b.Name)
	return err
}

// BranchExtras returns recently checked-out branch names from the reflog and all tags.
func BranchExtras(ctx context.Context, root string) (recent, tags []string) {
	out, _ := Command(ctx, root, "reflog", "show", "-n", "200", "--format=%gs", "HEAD")
	for line := range strings.SplitSeq(string(out), "\n") {
		if msg, ok := strings.CutPrefix(line, "checkout: moving from "); ok {
			if from, to, ok := strings.Cut(msg, " to "); ok {
				for _, n := range []string{to, from} {
					if !slices.Contains(recent, n) {
						recent = append(recent, n)
					}
				}
			}
		}
	}
	out, _ = Command(ctx, root, "tag", "--list")
	return recent, strings.Fields(string(out))
}

// CreateTag creates a lightweight tag, or an annotated one with message,
// at rev, HEAD when "".
func CreateTag(ctx context.Context, root, name, rev, message string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("a tag needs a name")
	}
	if _, err := Command(ctx, root, "check-ref-format", "refs/tags/"+name); err != nil {
		return errors.New("“" + name + "” is not a valid tag name")
	}
	args := []string{"tag"}
	if strings.TrimSpace(message) != "" {
		args = append(args, "-a", "-m", message)
	}
	args = append(args, "--", name)
	if rev != "" {
		if err := checkRevision(rev); err != nil {
			return err
		}
		args = append(args, rev)
	}
	_, err := Command(ctx, root, args...)
	return err
}

// DeleteTag deletes a local tag.
func DeleteTag(ctx context.Context, root, name string) error {
	_, err := Command(ctx, root, "tag", "-d", "--", name)
	return err
}

// PushTag pushes a local tag to the first remote, origin when there is one.
func PushTag(ctx context.Context, root, name string) error {
	remotes := Remotes(ctx, root)
	if len(remotes) == 0 {
		return errors.New("no remote to push to")
	}
	_, err := Network(ctx, root, "push", "--", remotes[0], "refs/tags/"+name)
	return err
}
