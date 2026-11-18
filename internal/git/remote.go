// INPUT: 远端配置。
// OUTPUT: Tracking、Fetch、UpdateSummary、WebURL 等。
// POS: 远端与托管平台链接。
// PROTOCOL: 变更时更新此头部，然后检查本包 doc.go

package git

import (
	"context"
	"net/url"
	"strconv"
	"strings"
)

// Tracking is the branch HEAD's branch tracks, and how many commits HEAD
// has that it has not (ahead) and it has that HEAD has not (behind); ""
// without one.
func Tracking(ctx context.Context, root string) (upstream string, ahead, behind int) {
	out, err := Command(ctx, root, "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{upstream}")
	if err != nil {
		return "", 0, 0
	}
	upstream = strings.TrimSpace(string(out))
	out, err = Command(ctx, root, "rev-list", "--left-right", "--count", "HEAD...@{upstream}")
	if err != nil {
		return upstream, 0, 0
	}
	if f := strings.Fields(string(out)); len(f) == 2 {
		ahead, _ = strconv.Atoi(f[0])
		behind, _ = strconv.Atoi(f[1])
	}
	return upstream, ahead, behind
}

// Refs includes names as well as targets: a push, tag or branch switch
// can change history decorations without changing HEAD.
func Refs(ctx context.Context, root string) (string, error) {
	out, err := Command(ctx, root, "for-each-ref", "--format=%(refname)%00%(objectname)%00%(symref)", "refs/heads", "refs/remotes", "refs/tags")
	return string(out), err
}

// Remotes lists the repository's remotes, origin first.
func Remotes(ctx context.Context, root string) []string {
	out, err := Command(ctx, root, "remote")
	if err != nil {
		return nil
	}
	var remotes []string
	for _, r := range strings.Fields(string(out)) {
		if r == "origin" {
			remotes = append([]string{r}, remotes...)
		} else {
			remotes = append(remotes, r)
		}
	}
	return remotes
}

// WebURL is the web page of a remote's repository, as GitHub, GitLab and
// most hosts serve it, from its fetch URL: git@github.com:o/r.git and
// https://github.com/o/r.git are both https://github.com/o/r. It is ""
// for a remote it cannot tell, as a path on disk.
func WebURL(remote string) string {
	remote = strings.TrimSpace(remote)
	if remote == "" {
		return ""
	}
	var host, path string
	if u, err := url.Parse(remote); err == nil && u.Host != "" {
		if u.Scheme != "https" && u.Scheme != "http" && u.Scheme != "ssh" && u.Scheme != "git" {
			return ""
		}
		host, path = u.Hostname(), u.Path
	} else if at := strings.Index(remote, "@"); at >= 0 && strings.Contains(remote[at:], ":") {
		// scp-like: git@host:owner/repo.git
		rest := remote[at+1:]
		i := strings.Index(rest, ":")
		host, path = rest[:i], rest[i+1:]
	} else {
		return ""
	}
	path = strings.TrimSuffix(strings.Trim(path, "/"), ".git")
	if host == "" || path == "" {
		return ""
	}
	return "https://" + host + "/" + path
}

// RemoteWebURL is the web page of the repository of remote name, or "".
func RemoteWebURL(ctx context.Context, root, name string) string {
	out, err := Command(ctx, root, "remote", "get-url", name)
	if err != nil {
		return ""
	}
	return WebURL(string(out))
}

// AddBranchRemote validates target as a branch name and registers remote at url.
func AddBranchRemote(ctx context.Context, root, target, remote, url string) error {
	if _, err := Command(ctx, root, "check-ref-format", "refs/heads/"+target); err != nil {
		return err
	}
	_, err := Command(ctx, root, "remote", "add", "--", remote, url)
	return err
}

// Fetch updates remote-tracking branches across all remotes.
func Fetch(ctx context.Context, root string) error {
	_, err := Network(ctx, root, "fetch", "--all", "--prune")
	return err
}

// UpdateSummary is what an update brought from old to new: how many
// commits, and how many files they changed.
func UpdateSummary(ctx context.Context, root, old, new string) (commits, files int) {
	if old == "" || new == "" || old == new {
		return 0, 0
	}
	if out, err := Command(ctx, root, "rev-list", "--count", old+".."+new); err == nil {
		commits, _ = strconv.Atoi(strings.TrimSpace(string(out)))
	}
	if out, err := Command(ctx, root, "diff", "--name-only", "-z", old, new, "--"); err == nil {
		files = strings.Count(string(out), "\x00")
	}
	return commits, files
}

// HasRef reports whether ref names a commit in the repository.
func HasRef(ctx context.Context, root, ref string) bool {
	_, err := Command(ctx, root, "rev-parse", "--verify", "--quiet", ref+"^{commit}")
	return err == nil
}
