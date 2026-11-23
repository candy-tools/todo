package todo

import (
	"context"
	"errors"
	"testing"
)

func TestStoreSprintWritesPersist(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	ids := addSample(t, s)

	a := mustID(t)(s.AddSprint(ctx, "Alpha", "goal"))
	b := mustID(t)(s.AddSprint(ctx, "Beta", ""))
	// addSample made the first sprint, for its working task.
	if doc := load(t, s); len(doc.Sprints) != 3 || doc.Sprints[1].Goal != "goal" {
		t.Fatalf("sprints after add = %+v", doc.Sprints)
	}

	name := "Gamma"
	mustOK(t)(s.EditSprint(ctx, b, SprintChanges{Name: &name}))
	if sp, _ := load(t, s).SprintByID(b); sp.Name != "Gamma" {
		t.Errorf("name = %q", sp.Name)
	}

	mustOK(t)(s.Assign(ctx, ids.t1, a))
	if it := item(t, load(t, s), ids.t1); it.Sprint != a || it.Status != New {
		t.Errorf("assigned task = %q %v", it.Sprint, it.Status)
	}
	mustOK(t)(s.Unassign(ctx, ids.t1))
	if it := item(t, load(t, s), ids.t1); it.Sprint != "" || it.Status != Open {
		t.Errorf("unassigned task = %q %v", it.Sprint, it.Status)
	}

	mustOK(t)(s.Assign(ctx, ids.t2, a))
	mustOK(t)(s.SetState(ctx, ids.t2, Done, StateOptions{}))
	removed, _, err := s.PruneSprint(ctx, a)
	if err != nil || removed != 1 {
		t.Fatalf("PruneSprint = %d, %v", removed, err)
	}
	if _, ok := load(t, s).ByID(ids.t2); ok {
		t.Error("pruned task still there")
	}

	mustOK(t)(s.Assign(ctx, ids.t1, a))
	if _, err := s.DeleteSprint(ctx, a); !errors.Is(err, ErrSprintNotDone) {
		t.Fatalf("DeleteSprint of a sprint not done = %v, want ErrSprintNotDone", err)
	}
	mustOK(t)(s.SetState(ctx, ids.t1, Done, StateOptions{Cascade: true}))
	if got := load(t, s).SprintStatus(a); got != SprintDone {
		t.Errorf("sprint status = %v, want done", got)
	}
	mustOK(t)(s.DeleteSprint(ctx, a))
	doc := load(t, s)
	if _, ok := doc.SprintByID(a); ok {
		t.Error("sprint not deleted")
	}
	if _, ok := doc.ByID(ids.t1); ok {
		t.Error("the sprint's task was not deleted with it")
	}
}

func TestStoreSprintMissingID(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	ids := addSample(t, s)
	const missing = "zzzzzzz"
	name := "x"
	_, _, pruneErr := s.PruneSprint(ctx, missing)
	for name, err := range map[string]error{
		"edit":   errOf(s.EditSprint(ctx, missing, SprintChanges{Name: &name})),
		"delete": errOf(s.DeleteSprint(ctx, missing)),
		"assign": errOf(s.Assign(ctx, ids.t1, missing)),
		"prune":  pruneErr,
	} {
		if !errors.Is(err, ErrNotFound) || !errors.Is(err, ErrSprintNotFound) {
			t.Errorf("%s: err = %v, want ErrSprintNotFound", name, err)
		}
	}
	sp := mustID(t)(s.AddSprint(ctx, "S", ""))
	if err := errOf(s.Assign(ctx, missing, sp)); !errors.Is(err, ErrNotFound) || errors.Is(err, ErrSprintNotFound) {
		t.Errorf("assign missing task: err = %v", err)
	}
	if err := errOf(s.Unassign(ctx, missing)); !errors.Is(err, ErrNotFound) {
		t.Errorf("unassign missing task: err = %v", err)
	}
}
