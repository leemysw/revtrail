// INPUT: 提交范围与变基步骤。
// OUTPUT: RebasePlan、InteractiveRebase、Autosquash。
// POS: 交互式变基。
// PROTOCOL: 变更时更新此头部，然后检查本包 doc.go

package git

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// RebaseAction is what an interactive rebase does with one commit.
type RebaseAction string

const (
	RebasePick   RebaseAction = "pick"
	RebaseReword RebaseAction = "reword"
	RebaseEdit   RebaseAction = "edit"
	RebaseSquash RebaseAction = "squash"
	RebaseFixup  RebaseAction = "fixup"
	RebaseDrop   RebaseAction = "drop"
)

// RebaseStep is one line of an interactive rebase's plan. Message is the
// commit's message, a new one for a reword.
type RebaseStep struct {
	Action  RebaseAction
	Commit  Commit
	Message string
}

// RebasePlan is each commit from from to HEAD, oldest first, picked with
// its message.
func RebasePlan(ctx context.Context, root, from string) ([]RebaseStep, error) {
	if err := checkRevision(from); err != nil {
		return nil, err
	}
	if _, err := Command(ctx, root, "merge-base", "--is-ancestor", from, "HEAD"); err != nil {
		return nil, errors.New("the commit is not on the current branch")
	}
	span := []string{"HEAD", "--not", from + "^@"}
	commits, err := logPage(ctx, root, 0, 1000, nil, span)
	if err != nil {
		return nil, err
	}
	out, err := Command(ctx, root, append([]string{"log", "--format=%H%x00%B%x1e"}, span...)...)
	if err != nil {
		return nil, err
	}
	messages := map[string]string{}
	for rec := range strings.SplitSeq(string(out), "\x1e") {
		hash, msg, _ := strings.Cut(strings.TrimLeft(rec, "\n"), "\x00")
		messages[hash] = strings.TrimSpace(msg)
	}
	steps := make([]RebaseStep, 0, len(commits))
	for _, c := range slices.Backward(commits) {
		if len(c.Parents) > 1 {
			return nil, errors.New("the commits to rebase include a merge")
		}
		steps = append(steps, RebaseStep{Action: RebasePick, Commit: c, Message: messages[c.Hash]})
	}
	return steps, nil
}

// Autosquash is the plan IntelliJ's Interactively Rebase from Here opens
// with: steps with the fixup! and squash! commits moved after the commit
// they name, as --autosquash places them.
func Autosquash(steps []RebaseStep) []RebaseStep {
	var plan []RebaseStep
	for _, s := range steps {
		action, target := RebasePick, ""
		for prefix, a := range map[string]RebaseAction{"fixup! ": RebaseFixup, "squash! ": RebaseSquash} {
			if t, ok := strings.CutPrefix(s.Commit.Subject, prefix); ok {
				action, target = a, t
			}
		}
		at := -1
		if target != "" {
			for i, p := range plan {
				if p.Commit.Subject == target || strings.HasPrefix(p.Commit.Hash, target) {
					at = i
				}
			}
		}
		if at < 0 {
			plan = append(plan, s)
			continue
		}
		// After the target and the fixups already behind it.
		for at+1 < len(plan) && (plan[at+1].Action == RebaseFixup || plan[at+1].Action == RebaseSquash) {
			at++
		}
		s.Action = action
		plan = slices.Insert(plan, at+1, s)
	}
	return plan
}

// InteractiveRebase rebases the current branch onto from's parent with
// steps as its plan. A reword sets its message itself, so no editor
// opens; an edit, or a conflict, stops the rebase with ErrPaused or
// ErrConflict, to go on with ContinueOperation.
func InteractiveRebase(ctx context.Context, root, from string, steps []RebaseStep, mode LocalChanges) error {
	if err := checkRevision(from); err != nil {
		return err
	}
	if len(steps) == 0 || steps[0].Action == RebaseSquash || steps[0].Action == RebaseFixup {
		return errors.New("the first commit cannot be squashed into one before it")
	}
	var todo strings.Builder
	for _, s := range steps {
		action := s.Action
		if action == RebaseReword {
			action = RebasePick
		}
		todo.WriteString(string(action) + " " + s.Commit.Hash + " " + s.Commit.Subject + "\n")
		if s.Action == RebaseReword {
			todo.WriteString("exec " + amendMessage(s.Message) + "\n")
		}
	}
	dir, err := os.MkdirTemp("", "revtrail-rebase")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	file := filepath.Join(dir, "git-rebase-todo")
	if err := os.WriteFile(file, []byte(todo.String()), 0o600); err != nil {
		return err
	}
	onto := from + "^"
	if _, err := Command(ctx, root, "rev-parse", "--verify", "--quiet", onto); err != nil {
		onto = "--root"
	}
	args := autostash(mode, "-c", "sequence.editor=cp "+shellQuote(file), "rebase", "--interactive", "--no-autosquash")
	return operation(ctx, root, 5*time.Minute, append(args, onto)...)
}

// amendMessage is a shell command, on one line, that gives HEAD message.
func amendMessage(message string) string {
	var b strings.Builder
	b.WriteString("printf '%s\\n'")
	for line := range strings.SplitSeq(strings.TrimSpace(message), "\n") {
		b.WriteString(" " + shellQuote(line))
	}
	b.WriteString(" | git commit --amend --only --no-verify --quiet --cleanup=strip -F -")
	return b.String()
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
