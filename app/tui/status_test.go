package tui

import (
	"strings"
	"testing"

	"github.com/candy-tools/todo/internal/todo"
)

func TestSetInProgress(t *testing.T) {
	m, path := newTestModel(t, "# Work\n\n- [ ] a\n")
	m = press(m, "down", "p")
	if got := find(m.doc, "a").Status; got != todo.InProgress {
		t.Errorf("p should set the task in progress, got %v", got)
	}
	if !strings.Contains(readFile(t, path), "- [/] a") {
		t.Errorf("in-progress not persisted:\n%s", readFile(t, path))
	}
	// p again returns it to open.
	m = press(m, "p")
	if got := find(m.doc, "a").Status; got != todo.Open {
		t.Errorf("p on an in-progress task should return it to open, got %v", got)
	}
}

func TestSetDeferred(t *testing.T) {
	m, path := newTestModel(t, "# Work\n\n- [ ] a\n")
	m = press(m, "down", ">")
	if got := find(m.doc, "a").Status; got != todo.Deferred {
		t.Errorf("> should defer the task, got %v", got)
	}
	if !strings.Contains(readFile(t, path), "- [>] a") {
		t.Errorf("deferred not persisted:\n%s", readFile(t, path))
	}
	// > again returns it to open.
	m = press(m, ">")
	if got := find(m.doc, "a").Status; got != todo.Open {
		t.Errorf("> on a deferred task should return it to open, got %v", got)
	}
}

func TestStatusFlagSwitchesStates(t *testing.T) {
	// p then > moves straight from in-progress to deferred (not back to open).
	m, _ := newTestModel(t, "# Work\n\n- [ ] a\n")
	m = press(m, "down", "p", ">")
	if got := find(m.doc, "a").Status; got != todo.Deferred {
		t.Errorf("> on an in-progress task should defer it, got %v", got)
	}
}

func TestStatusFlagsDoNotCascade(t *testing.T) {
	m, _ := newTestModel(t, "# Work\n\n- [ ] parent\n  - [ ] c1\n")
	m = press(m, "down", "p") // parent in progress
	if got := find(m.doc, "parent").Status; got != todo.InProgress {
		t.Fatalf("parent should be in progress, got %v", got)
	}
	if got := find(m.doc, "c1").Status; got != todo.Open {
		t.Errorf("a status flag must not cascade to children, c1 = %v", got)
	}
}

func TestStatusFlagIsTaskOnly(t *testing.T) {
	m, _ := newTestModel(t, "# Work\n\n- [ ] a\n") // Work category selected
	m = press(m, "p")
	if m.status == "" {
		t.Errorf("p on a category should report that only tasks can be flagged")
	}
	if got := find(m.doc, "a").Status; got != todo.Open {
		t.Errorf("flagging a category must not touch its tasks, a = %v", got)
	}
}

func TestTreeRowStatusGlyphs(t *testing.T) {
	d := todo.Parse("# Work\n\n- [/] prog\n- [>] defer\n")
	tr := newTree(d)
	if got := tr.rowString(treeRow{item: find(d, "prog")}, false); !strings.Contains(got, "◐") {
		t.Errorf("in-progress row should contain ◐, got %q", got)
	}
	if got := tr.rowString(treeRow{item: find(d, "defer")}, false); !strings.Contains(got, "⏸") {
		t.Errorf("deferred row should contain ⏸, got %q", got)
	}
}

func TestItemMetaStatus(t *testing.T) {
	d := todo.Parse("# Work\n\n- [/] prog\n- [>] defer\n")
	if got := itemMeta(find(d, "prog")); !strings.Contains(got, "in progress") {
		t.Errorf("itemMeta for an in-progress task should say 'in progress', got %q", got)
	}
	if got := itemMeta(find(d, "defer")); !strings.Contains(got, "deferred") {
		t.Errorf("itemMeta for a deferred task should say 'deferred', got %q", got)
	}
}

func TestFooterShowsStatusHints(t *testing.T) {
	m, _ := newTestModel(t, "# Work\n\n- [ ] a\n")
	got := m.footer(200)
	for _, want := range []string{"Prog", "Defer"} {
		if !strings.Contains(got, want) {
			t.Errorf("footer should hint %q, got %q", want, got)
		}
	}
}
