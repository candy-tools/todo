package tui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/candy-tools/todo/internal/todo"
)

func TestTreeFlatten(t *testing.T) {
	d := parseDoc(t, "# Work\n\n- [ ] a\n  - [ ] a1\n- [ ] b\n")
	tr := newTree(d, newViewState())
	// Four real rows plus the trailing placeholder.
	if len(tr.rows) != 5 {
		t.Fatalf("got %d rows, want 5 (incl. placeholder)", len(tr.rows))
	}
	if tr.rows[4].kind != rowNewCategory {
		t.Errorf("the last row should be the placeholder")
	}
	want := []string{"Work", "a", "a1", "b"}
	for i, w := range want {
		if tr.rows[i].item.Title != w {
			t.Errorf("row %d = %q, want %q", i, tr.rows[i].item.Title, w)
		}
	}
	// Depths: Work 0, a 1, a1 2, b 1.
	wantDepth := []int{0, 1, 2, 1}
	for i, d := range wantDepth {
		if tr.rows[i].depth != d {
			t.Errorf("row %d depth = %d, want %d", i, tr.rows[i].depth, d)
		}
	}
}

func TestTreePlaceholderAlwaysPresent(t *testing.T) {
	tr := newTree(parseDoc(t, "# Work\n"), newViewState()) // rows: Work, placeholder
	if len(tr.rows) != 2 || tr.rows[1].kind != rowNewCategory {
		t.Fatalf("expected a trailing placeholder row, got %d rows", len(tr.rows))
	}
	tr.cursor = 1
	if !tr.onPlaceholder() {
		t.Errorf("cursor on the last row should report onPlaceholder")
	}
	tr.cursor = 0
	if tr.onPlaceholder() {
		t.Errorf("cursor on Work should not report onPlaceholder")
	}
}

func TestTreeCollapseJumpsToParentOnLeaf(t *testing.T) {
	d := parseDoc(t, "# Work\n\n- [ ] a\n")
	tr := newTree(d, newViewState())
	tr.cursor = 1 // task a (a leaf)
	tr.collapse()
	if tr.selected().Title != "Work" {
		t.Errorf("collapse on a leaf should jump to the parent, got %q", tr.selected().Title)
	}
}

func TestTreeCursorClampsAfterRebuild(t *testing.T) {
	d := parseDoc(t, "# Work\n\n- [ ] a\n- [ ] b\n")
	tr := newTree(d, newViewState()) // rows: Work, a, b, placeholder
	tr.cursor = 2                    // task b
	tr.collapsed[d.Roots[0].ID] = true
	tr.rebuild() // now: Work, placeholder
	if tr.cursor != len(tr.rows)-1 {
		t.Errorf("cursor should clamp into range after rows shrink, got %d of %d", tr.cursor, len(tr.rows))
	}
}

func TestTreeRowRendering(t *testing.T) {
	d := parseDoc(t, "# Work\n\n- [ ] open\n- [x] done\n")
	tr := newTree(d, newViewState())
	// Unselected done task shows a filled circle; open task shows an empty circle.
	if got := tr.rowString(treeRow{item: find(d, "done")}, false); !strings.Contains(got, "●") {
		t.Errorf("done row should contain ●, got %q", got)
	}
	if got := tr.rowString(treeRow{item: find(d, "open")}, false); !strings.Contains(got, "○") {
		t.Errorf("open row should contain ○, got %q", got)
	}
	// A selected row is marked with the ❯ gutter.
	if got := tr.rowString(treeRow{item: find(d, "open")}, true); !strings.Contains(got, "❯") {
		t.Errorf("selected row should contain the ❯ gutter, got %q", got)
	}
}

func TestTreePageJumps(t *testing.T) {
	// One category with 30 tasks: rows are Work (0), t0..t29 (1..30), placeholder (31).
	var b strings.Builder
	b.WriteString("# Work\n\n")
	for i := 0; i < 30; i++ {
		_, _ = fmt.Fprintf(&b, "- [ ] t%d\n", i)
	}
	tr := newTree(parseDoc(t, b.String()), newViewState())
	last := len(tr.rows) - 1

	// pageDown advances a fixed navStep rows.
	tr.cursor = 0
	tr.pageDown()
	if tr.cursor != navStep {
		t.Errorf("pageDown from 0 = %d, want %d", tr.cursor, navStep)
	}
	// pageUp goes back the same stride.
	tr.pageUp()
	if tr.cursor != 0 {
		t.Errorf("pageUp back to the top = %d, want 0", tr.cursor)
	}
	// pageUp clamps at the top instead of going negative.
	tr.pageUp()
	if tr.cursor != 0 {
		t.Errorf("pageUp past the top should clamp to 0, got %d", tr.cursor)
	}
	// pageDown clamps at the last row.
	tr.cursor = last - 2
	tr.pageDown()
	if tr.cursor != last {
		t.Errorf("pageDown past the end should clamp to %d, got %d", last, tr.cursor)
	}
}

func TestTreeScrollDecoupledFromCursor(t *testing.T) {
	// One category with 30 tasks: rows Work(0), t0..t29 (1..30), placeholder(31).
	var b strings.Builder
	b.WriteString("# Work\n\n")
	for i := 0; i < 30; i++ {
		_, _ = fmt.Fprintf(&b, "- [ ] t%d\n", i)
	}
	tr := newTree(parseDoc(t, b.String()), newViewState())
	tr.viewHeight = 10

	// Scroll down well past the first page.
	for i := 0; i < 15; i++ {
		tr.moveDown()
	}
	if tr.cursor != 15 {
		t.Fatalf("cursor after 15×moveDown = %d, want 15", tr.cursor)
	}
	// The window scrolled just enough to keep the cursor on the last visible row.
	if want := tr.cursor - tr.viewHeight + 1; tr.offset != want {
		t.Fatalf("offset after scrolling down = %d, want %d", tr.offset, want)
	}

	// Regression: moving the cursor up within the window must move the pointer,
	// not the page. The view used to re-derive the window from the cursor, pinning
	// the cursor to the bottom row so the whole page slid up on a single step up.
	page := tr.offset
	tr.moveUp()
	if tr.cursor != 14 {
		t.Errorf("cursor after moveUp = %d, want 14", tr.cursor)
	}
	if tr.offset != page {
		t.Errorf("scroll offset must stay put while the cursor moves within the window: got %d, want %d", tr.offset, page)
	}

	// Stepping up to the top of the window still doesn't scroll…
	for tr.cursor > tr.offset {
		tr.moveUp()
	}
	if tr.offset != page {
		t.Errorf("offset moved while the cursor was still inside the window: got %d, want %d", tr.offset, page)
	}
	// …but the next step, which leaves the window, scrolls it up by exactly one.
	tr.moveUp()
	if tr.offset != page-1 {
		t.Errorf("offset should scroll up by one when the cursor leaves the top: got %d, want %d", tr.offset, page-1)
	}
}

func TestTreeEmptyView(t *testing.T) {
	tr := newTree(&todo.Document{}, newViewState())
	if !strings.Contains(tr.view(40, 10), "new category") {
		t.Errorf("an empty tree should still show the + new category placeholder")
	}
}

func TestTreeDescriptionMarker(t *testing.T) {
	d := parseDoc(t, "# Work\n\n- [ ] a\n  notes here\n- [ ] b\n")
	tr := newTree(d, newViewState())
	if got := tr.rowString(treeRow{item: find(d, "a")}, false); !strings.Contains(got, "≡") {
		t.Errorf("a task with a description should carry the ≡ marker, got %q", got)
	}
	if got := tr.rowString(treeRow{item: find(d, "b")}, false); strings.Contains(got, "≡") {
		t.Errorf("a task without a description must not carry the marker, got %q", got)
	}
}

// TestTaskRowTrailer pins the exact text after a task's title (state, subtask
// progress, the no-plan warning and the ≡ marker) on selected and unselected
// rows, and its absence on a subtask row.
func TestTaskRowTrailer(t *testing.T) {
	d := parseDoc(t, "# Work\n\n- [ ] a\n  notes here\n  - [x] s1\n  - [ ] s2\n")
	find(d, "a").Status = todo.Ready
	tr := newTree(d, newViewState())
	cases := []struct {
		row      treeRow
		selected bool
		want     string
	}{
		{treeRow{item: find(d, "a")}, false, "    ◉ a ready (1/2) ⚠ no plan ≡"},
		{treeRow{item: find(d, "a")}, true, "❯   ◉ a ready (1/2) ⚠ no plan ≡"},
		{treeRow{item: find(d, "s1"), depth: 1}, false, "      ● s1"},
		{treeRow{item: find(d, "s2"), depth: 1}, true, "❯     ○ s2"},
	}
	for _, c := range cases {
		if got := tr.rowString(c.row, c.selected); got != c.want {
			t.Errorf("rowString(%q, selected %v) = %q, want %q", c.row.item.Title, c.selected, got, c.want)
		}
	}
}
