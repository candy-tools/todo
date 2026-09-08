package todo_test

import (
	"testing"

	"github.com/candy-tools/todo/internal/todo"
)

func TestRenderCanonical(t *testing.T) {
	// The sample is already in canonical form, so it must round-trip verbatim.
	got := todo.Parse(sample).Render()
	if got != sample {
		t.Errorf("render mismatch:\n got:\n%q\nwant:\n%q", got, sample)
	}
}

func TestRenderStable(t *testing.T) {
	// Parse → render → parse → render must be a fixed point regardless of the
	// input's original spacing.
	messy := "#    Work\n- [ ]   Ship\n\n\n  extra    desc\n\n\n- [x] Done thing\n"
	once := todo.Parse(messy).Render()
	twice := todo.Parse(once).Render()
	if once != twice {
		t.Errorf("render not stable:\n once:\n%q\ntwice:\n%q", once, twice)
	}
}

func TestRenderRoundTripTree(t *testing.T) {
	// The tree after a render/re-parse must be identical to the original tree.
	d1 := todo.Parse(sample)
	d2 := todo.Parse(d1.Render())
	if treeString(d1) != treeString(d2) {
		t.Errorf("tree changed across a render round-trip:\n%s\n---\n%s", treeString(d1), treeString(d2))
	}
}

func TestRenderPreservesPreamble(t *testing.T) {
	src := "Intro line\n\n# Work\n\n- [ ] a\n"
	got := todo.Parse(src).Render()
	if got != src {
		t.Errorf("preamble round-trip failed:\n got:\n%q\nwant:\n%q", got, src)
	}
}

func TestRenderStatusMarkersRoundTrip(t *testing.T) {
	// All four checkbox markers must round-trip verbatim through parse → render.
	src := "# W\n\n- [ ] open\n- [/] prog\n- [>] defer\n- [x] done\n"
	if got := todo.Parse(src).Render(); got != src {
		t.Errorf("status markers should round-trip:\n got:\n%q\nwant:\n%q", got, src)
	}
}

func TestRenderEmpty(t *testing.T) {
	if got := (&todo.Document{}).Render(); got != "" {
		t.Errorf("empty document should render to empty string, got %q", got)
	}
}

func TestRenderMultilineDescriptionWithBlank(t *testing.T) {
	task := todo.NewTask("t", "para one\n\npara two", false)
	d := &todo.Document{}
	d.AppendRoot(task)
	want := "- [ ] t\n  para one\n\n  para two\n"
	if got := d.Render(); got != want {
		t.Errorf("render = %q, want %q", got, want)
	}
	// And it must survive a re-parse.
	if todo.Parse(want).Roots[0].Description != "para one\n\npara two" {
		t.Errorf("blank-line-in-description did not round-trip")
	}
}
