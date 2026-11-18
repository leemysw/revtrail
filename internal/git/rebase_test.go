package git

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

func TestInteractiveRebase(t *testing.T) {
	ctx := context.Background()
	root, run := newRepo(t)
	run("config", "user.name", "Revtrail")
	run("config", "user.email", "revtrail@example.com")
	run("config", "commit.gpgsign", "false")
	commit := func(file, subject string) {
		write(t, root, file, subject+"\n")
		run("add", file)
		run("commit", "-qm", subject)
	}
	commit("a.txt", "add a")
	commit("b.txt", "add b")
	commit("c.txt", "add c")
	commit("b.txt", "fixup! add b")
	commit("d.txt", "add d")

	plan, err := RebasePlan(ctx, root, "HEAD~3")
	if err != nil {
		t.Fatal(err)
	}
	plan = Autosquash(plan)
	var got []string
	for _, s := range plan {
		got = append(got, string(s.Action)+" "+s.Commit.Subject)
	}
	if want := "[pick add b fixup fixup! add b pick add c pick add d]"; fmt.Sprint(got) != want {
		t.Fatalf("plan = %v, want %s", got, want)
	}

	plan[2].Action, plan[2].Message = RebaseReword, "add c, reworded\n\nit's quoted"
	plan[3].Action = RebaseDrop
	if err := InteractiveRebase(ctx, root, "HEAD~3", plan, KeepChanges); err != nil {
		t.Fatal(err)
	}
	if got := run("log", "--format=%B%x00"); got != "add c, reworded\n\nit's quoted\n\x00\nadd b\n\x00\nadd a\n\x00\n" {
		t.Fatalf("log = %q", got)
	}
	if got := run("show", "HEAD~1:b.txt"); got != "fixup! add b\n" {
		t.Fatalf("b.txt = %q, want the fixup's content", got)
	}

	plan, err = RebasePlan(ctx, root, "HEAD~2")
	if err != nil || len(plan) != 3 || plan[0].Commit.Subject != "add a" {
		t.Fatalf("root plan = %+v (%v)", plan, err)
	}
	plan[0].Action = RebaseEdit
	if err := InteractiveRebase(ctx, root, "HEAD~2", plan, KeepChanges); !errors.Is(err, ErrPaused) {
		t.Fatalf("edit stop = %v, want ErrPaused", err)
	}
	if err := ContinueOperation(ctx, root); err != nil || OperationState(ctx, root) != OpNormal {
		t.Fatalf("continue = %v, state %v", err, OperationState(ctx, root))
	}
	if got := run("rev-list", "--count", "HEAD"); got != "3\n" {
		t.Fatalf("commits after edit = %s", got)
	}
}
