// INPUT: 本地分支与远端。
// OUTPUT: PushPlan：待推提交与推送。
// POS: 推送。
// PROTOCOL: 变更时更新此头部，然后检查本包 doc.go

package git

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
)

type PushPlan struct {
	Root, Branch, Head, Remote, Target, Expected string
	Remotes                                      []string
	pushURLs                                     string
	Commits                                      []Commit
	Tracked, Behind, More                        bool
}

// ReadPushPlan resolves where local branch, the current one when "",
// pushes to, independently of the pull upstream, and what it would push.
func ReadPushPlan(ctx context.Context, root, branch, remote, target string, limit int) (PushPlan, error) {
	p := PushPlan{Root: root, Remotes: Remotes(ctx, root), Branch: branch}
	if branch == "" {
		b, err := Command(ctx, root, "symbolic-ref", "--quiet", "--short", "HEAD")
		if err != nil {
			return p, errors.New("detached HEAD")
		}
		p.Branch = strings.TrimSpace(string(b))
	}
	p.Head = branchTip(ctx, root, p.Branch)
	if p.Head == "" {
		if branch != "" {
			return p, errors.New("no branch " + branch)
		}
		return p, nil
	}
	out, err := Command(ctx, root, "for-each-ref", "--format=%(upstream:remotename)%00%(upstream:remoteref)", "refs/heads/"+p.Branch)
	if err != nil {
		return p, err
	}
	up, ref, _ := strings.Cut(strings.TrimSpace(string(out)), "\x00")
	p.Tracked = up != ""
	if remote == "" {
		for _, key := range []string{"branch." + p.Branch + ".pushRemote", "remote.pushDefault"} {
			if v, e := Command(ctx, root, "config", "--get", key); e == nil {
				remote = strings.TrimSpace(string(v))
				break
			}
		}
		if remote == "" && slices.Contains(p.Remotes, up) {
			remote = up
		}
		if remote == "" && len(p.Remotes) > 0 {
			remote = p.Remotes[0]
		}
	}
	if target == "" {
		target = p.Branch
		if remote == up && strings.HasPrefix(ref, "refs/heads/") {
			target = strings.TrimPrefix(ref, "refs/heads/")
		}
	}
	p.Remote, p.Target = remote, target
	if _, err := Command(ctx, root, "check-ref-format", "refs/heads/"+target); err != nil {
		return p, err
	}
	refs := []string{p.Head}
	if remote != "" {
		if !slices.Contains(p.Remotes, remote) {
			return p, fmt.Errorf("unknown remote: %s", remote)
		}
		urls, err := Command(ctx, root, "remote", "get-url", "--push", "--all", remote)
		if err != nil {
			return p, err
		}
		p.pushURLs = string(urls)
		remoteRef := "refs/remotes/" + remote + "/" + target
		if v, e := Command(ctx, root, "rev-parse", "--verify", "--quiet", remoteRef); e == nil {
			p.Expected = strings.TrimSpace(string(v))
			refs = append(refs, "^"+p.Expected)
			_, e = Command(ctx, root, "merge-base", "--is-ancestor", p.Expected, p.Head)
			p.Behind = e != nil
		} else {
			refs = append(refs, "--not", "--remotes="+remote)
		}
	}
	p.Commits, err = logPage(ctx, root, 0, limit+1, nil, refs)
	if len(p.Commits) > limit {
		p.Commits = p.Commits[:limit]
		p.More = true
	}
	return p, err
}

func (p PushPlan) Push(ctx context.Context, tags string, hooks, force bool) error {
	if branchTip(ctx, p.Root, p.Branch) != p.Head {
		return errors.New("the branch moved; reopen the push preview")
	}
	urls, err := Command(ctx, p.Root, "remote", "get-url", "--push", "--all", p.Remote)
	if err != nil || string(urls) != p.pushURLs {
		return errors.New("push destination changed; reopen the push preview")
	}
	ref := "refs/heads/" + p.Target
	args := []string{"push"}
	if !hooks {
		args = append(args, "--no-verify")
	}
	switch tags {
	case "all":
		args = append(args, "--tags")
	case "follow":
		args = append(args, "--follow-tags")
	default:
		args = append(args, "--no-follow-tags")
	}
	if force {
		args = append(args, "--force-with-lease="+ref+":"+p.Expected)
	}
	args = append(args, "--", p.Remote, p.Head+":"+ref)
	if _, err := Network(ctx, p.Root, args...); err != nil {
		return err
	}
	if !p.Tracked {
		_, err = Command(ctx, p.Root, "branch", "--set-upstream-to", p.Remote+"/"+p.Target, "--", p.Branch)
	}
	return err
}

func branchTip(ctx context.Context, root, branch string) string {
	out, err := Command(ctx, root, "rev-parse", "--verify", "--quiet", "refs/heads/"+branch)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
