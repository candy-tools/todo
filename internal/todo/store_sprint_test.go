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
	if doc := load(t, s); len(doc.Sprints) != 2 || doc.Sprints[0].Goal != "goal" {
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

	mustOK(t)(s.CloseSprint(ctx, a))
	if sp, _ := load(t, s).SprintByID(a); sp.Status != SprintDone {
		t.Error("sprint not closed")
	}
	mustOK(t)(s.ReopenSprint(ctx, a))
	if sp, _ := load(t, s).SprintByID(a); sp.Status != SprintOpen {
		t.Error("sprint not reopened")
	}
	mustOK(t)(s.DeleteSprint(ctx, a))
	if _, ok := load(t, s).SprintByID(a); ok {
		t.Error("sprint not deleted")
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
		"close":  errOf(s.CloseSprint(ctx, missing)),
		"reopen": errOf(s.ReopenSprint(ctx, missing)),
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
