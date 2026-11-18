// INPUT: 提交与新提交信息。
// OUTPUT: Reword、CommitMessage、Pushed：不动工作区改写提交信息。
// POS: 改提交信息。
// PROTOCOL: 变更时更新此头部，然后检查本包 doc.go

package git

import (
	"context"
	"errors"
	"strings"
)

// Reword gives commit hash, on the current branch, a new message, as
// IntelliJ's in-memory Edit Commit Message: the commit and those after it
// are written again with the same trees and the branch moves to the last,
// so the working tree and the index are left as they are.
func Reword(ctx context.Context, root, hash, message string) error {
	message = strings.TrimSpace(message)
	if message == "" {
		return errors.New("a commit needs a message")
	}
	if err := checkRevision(hash); err != nil {
		return err
	}
	branch, err := Command(ctx, root, "symbolic-ref", "--quiet", "HEAD")
	if err != nil {
		return errors.New("HEAD is not on a branch")
	}
	head, err := Command(ctx, root, "rev-parse", "--verify", "HEAD")
	if err != nil {
		return err
	}
	if _, err := Command(ctx, root, "merge-base", "--is-ancestor", hash, "HEAD"); err != nil {
		return errors.New("the commit is not on the current branch")
	}
	full, err := Command(ctx, root, "rev-parse", "--verify", "--end-of-options", hash+"^{commit}")
	if err != nil {
		return err
	}
	ident, err := Command(ctx, root, "var", "GIT_COMMITTER_IDENT")
	if err != nil {
		return err
	}
	// The commit itself, then each after it, parents before children.
	out, err := Command(ctx, root, "rev-list", "--reverse", "--topo-order", "--ancestry-path", strings.TrimSpace(string(full))+"..HEAD")
	if err != nil {
		return err
	}
	commits := append([]string{strings.TrimSpace(string(full))}, strings.Fields(string(out))...)
	rewritten := map[string]string{}
	for i, c := range commits {
		raw, err := Command(ctx, root, "cat-file", "commit", c)
		if err != nil {
			return err
		}
		msg := ""
		if i == 0 {
			msg = message + "\n"
		}
		obj := rewriteCommit(string(raw), rewritten, "committer "+strings.TrimSpace(string(ident)), msg)
		id, err := input(ctx, root, obj, "hash-object", "-t", "commit", "-w", "--stdin")
		if err != nil {
			return err
		}
		rewritten[c] = strings.TrimSpace(string(id))
	}
	last := rewritten[commits[len(commits)-1]]
	_, err = Command(ctx, root, "update-ref", "-m", "reword: "+firstLine(message), strings.TrimSpace(string(branch)), last, strings.TrimSpace(string(head)))
	return err
}

// rewriteCommit is the raw commit object raw with its parents mapped
// through rewritten, committer as its committer line, and message as its
// message when not "". A signature no longer holds, so it is dropped, and
// so is an encoding the new message is not in.
func rewriteCommit(raw string, rewritten map[string]string, committer, message string) string {
	header, body, _ := strings.Cut(raw, "\n\n")
	var b strings.Builder
	skipping := false
	for line := range strings.SplitSeq(header, "\n") {
		if skipping && strings.HasPrefix(line, " ") {
			continue
		}
		skipping = false
		key, value, _ := strings.Cut(line, " ")
		switch {
		case key == "gpgsig" || key == "gpgsig-sha256":
			skipping = true
			continue
		case key == "encoding" && message != "":
			continue
		case key == "committer":
			line = committer
		case key == "parent":
			if p, ok := rewritten[value]; ok {
				line = "parent " + p
			}
		}
		b.WriteString(line + "\n")
	}
	if message == "" {
		message = body
	}
	return b.String() + "\n" + message
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(s, "\n")
	return line
}

// CommitMessage is commit hash's whole message.
func CommitMessage(ctx context.Context, root, hash string) (string, error) {
	if err := checkRevision(hash); err != nil {
		return "", err
	}
	out, err := Command(ctx, root, "log", "-1", "--format=%B", "--end-of-options", hash)
	return strings.TrimSpace(string(out)), err
}

// Pushed reports whether a remote-tracking branch holds commit hash.
func Pushed(ctx context.Context, root, hash string) bool {
	out, err := Command(ctx, root, "branch", "--remotes", "--contains", hash)
	return err == nil && strings.TrimSpace(string(out)) != ""
}
