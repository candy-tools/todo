package tui

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/candy-tools/todo/internal/todo"
	"github.com/candy-tools/todo/internal/todo/todotest"
)

// externalAddTask adds a task through a second store handle, standing in for a
// CLI command or an agent writing while the TUI runs.
func externalAddTask(t *testing.T, path, title string, parentID, afterID uint) uint {
	t.Helper()
	id, err := openSecond(t, path).AddTask(context.Background(), title, "", todo.Open, parentID, afterID)
	if err != nil {
		t.Fatalf("external add: %v", err)
	}
	return id
}

// pollOnce runs one revision probe and feeds its message, which must not be
// nil, to the model.
func pollOnce(t *testing.T, m model) model {
	t.Helper()
	msg := reloadCheck(m.store, m.lastRev)
	if msg == nil {
		t.Fatal("expected the poll to report a change")
	}
	return send(m, msg)
}

// --- reloadCheck: the revision probe ---

func TestReloadCheckUnchangedIsNil(t *testing.T) {
	m, _ := newTestModel(t, "# Work\n\n- [ ] a\n")
	if msg := reloadCheck(m.store, m.lastRev); msg != nil {
		t.Errorf("an unchanged store must yield no reload, got %#v", msg)
	}
}

func TestReloadCheckDetectsChange(t *testing.T) {
	m, path := newTestModel(t, "# Work\n\n- [ ] a\n")
	work, a := find(m.doc, "Work"), find(m.doc, "a")
	externalAddTask(t, path, "b", work.ID, a.ID)
	msg := reloadCheck(m.store, m.lastRev)
	r, ok := msg.(storeReloadedMsg)
	if !ok {
		t.Fatalf("a changed store must yield a storeReloadedMsg, got %#v", msg)
	}
	if r.rev <= m.lastRev {
		t.Errorf("reload must carry the new revision, got %d (last %d)", r.rev, m.lastRev)
	}
	if find(r.doc, "b") == nil {
		t.Errorf("reloaded document must include the new task b")
	}
}

func TestReloadCheckErrorIsNil(t *testing.T) {
	m, _ := newTestModel(t, "# Work\n\n- [ ] a\n")
	if err := m.store.Close(); err != nil {
		t.Fatal(err)
	}
	if msg := reloadCheck(m.store, m.lastRev-1); msg != nil {
		t.Errorf("an unreadable store must not trigger a reload, got %#v", msg)
	}
}

// --- applying a reload to the model ---

func TestReloadAppliesExternalChange(t *testing.T) {
	m, path := newTestModel(t, "# Work\n\n- [ ] a\n")
	externalAddTask(t, path, "b", find(m.doc, "Work").ID, find(m.doc, "a").ID)
	m = pollOnce(t, m)
	if find(m.doc, "b") == nil {
		t.Fatalf("an external change must be applied to the model")
	}
	if !hasRow(m, "b") {
		t.Errorf("the new task must appear in the rebuilt tree")
	}
}

func TestReloadPreservesSelection(t *testing.T) {
	m, path := newTestModel(t, "# Work\n\n- [ ] a\n- [ ] c\n")
	m = press(m, "down", "down") // Work, a, c -> select c
	if got := m.tree.selected().Title; got != "c" {
		t.Fatalf("precondition: expected c selected, got %q", got)
	}
	externalAddTask(t, path, "b", find(m.doc, "Work").ID, find(m.doc, "a").ID)
	m = pollOnce(t, m)
	if got := m.tree.selected().Title; got != "c" {
		t.Errorf("selection must stay on c across a reload, got %q", got)
	}
}

func TestReloadPreservesFold(t *testing.T) {
	m, path := newTestModel(t, "# Work\n\n- [ ] a\n")
	m = press(m, "space") // Work is selected at start; space folds the header, collapsing it
	externalAddTask(t, path, "b", find(m.doc, "Work").ID, find(m.doc, "a").ID)
	m = pollOnce(t, m)
	work := m.tree.selected()
	if work.Title != "Work" || !m.tree.collapsed[work.ID] {
		t.Errorf("Work must remain collapsed after a reload")
	}
	if hasRow(m, "a") || hasRow(m, "b") {
		t.Errorf("a collapsed category's children must stay hidden after a reload")
	}
}

func TestReloadPreservesViewport(t *testing.T) {
	// A list taller than the viewport, so the scroll position is in play. The
	// window height comes from newTestModel's WindowSizeMsg.
	var b strings.Builder
	b.WriteString("# Work\n\n")
	for i := 0; i < 30; i++ {
		_, _ = fmt.Fprintf(&b, "- [ ] t%d\n", i)
	}
	m, path := newTestModel(t, b.String())

	// Scroll down well past the first page so the viewport is offset from the top.
	for i := 0; i < 30; i++ {
		m = press(m, "down")
	}
	wantVH, wantOffset := m.tree.viewHeight, m.tree.offset
	if wantVH <= 1 || wantOffset == 0 {
		t.Fatalf("precondition: want a sized, scrolled viewport, got viewHeight=%d offset=%d", wantVH, wantOffset)
	}
	sel := m.tree.selected().Title

	// An external write appends a task at the end (below the cursor, so the
	// selected row index is unchanged) and the app reloads.
	externalAddTask(t, path, "t30", find(m.doc, "Work").ID, find(m.doc, "t29").ID)
	m = pollOnce(t, m)

	if got := m.tree.selected().Title; got != sel {
		t.Fatalf("selection must survive the reload: got %q, want %q", got, sel)
	}
	// Regression: a reload that built a fresh tree dropped the viewport state,
	// resetting viewHeight to 0. reconcileOffset then treats the window as one row
	// tall and re-pins the scroll to the cursor — the #10 decoupling breaks until
	// the next terminal resize.
	if m.tree.viewHeight != wantVH {
		t.Errorf("reload must preserve viewHeight (else scroll decoupling breaks): got %d, want %d", m.tree.viewHeight, wantVH)
	}
	if m.tree.offset != wantOffset {
		t.Errorf("reload must preserve the scroll offset (the page must not jump): got %d, want %d", m.tree.offset, wantOffset)
	}
}

func TestReloadDeferredDuringModal(t *testing.T) {
	m, path := newTestModel(t, "# Work\n\n- [ ] a\n")
	m = press(m, "n") // open the add-task form
	if m.mode != modeForm {
		t.Fatal("precondition: form should be open")
	}
	externalAddTask(t, path, "b", find(m.doc, "Work").ID, find(m.doc, "a").ID)
	m = pollOnce(t, m)
	if m.mode != modeForm {
		t.Errorf("a reload must not close an open modal")
	}
	if find(m.doc, "b") != nil {
		t.Errorf("a reload must be deferred while a modal is open")
	}
	// Once the modal closes, the next poll picks the change up.
	m = press(m, "esc")
	m = pollOnce(t, m)
	if find(m.doc, "b") == nil {
		t.Errorf("the deferred reload must apply after the modal closes")
	}
}

func TestReloadIgnoresOwnRevision(t *testing.T) {
	m, _ := newTestModel(t, "# Work\n\n- [ ] a\n")
	m = send(m, storeReloadedMsg{doc: parseDoc(t, "# Work\n\n- [ ] zzz\n"), rev: m.lastRev})
	if find(m.doc, "zzz") != nil {
		t.Errorf("a reload at the revision the model already has must be ignored")
	}
}

func TestOwnWriteDoesNotReportReload(t *testing.T) {
	m, _ := newTestModel(t, "# Work\n\n- [ ] a\n")
	before := m.lastRev
	m = press(m, "down", "space") // toggle a done -> a store write
	if m.lastRev == before {
		t.Fatalf("an own write must record the revision it produced")
	}
	if msg := reloadCheck(m.store, m.lastRev); msg != nil {
		t.Errorf("after our own write, a poll must see no change, got %#v", msg)
	}
}

func TestExternalWriteReloadsKeepingSelection(t *testing.T) {
	m, path := newTestModel(t, "# A\n\n- [ ] a\n- [ ] b\n\n## A2\n\n- [ ] c\n")
	// Rows: A, a, b, A2, c, placeholder. Fold A2, then select b.
	m = press(m, "down", "down", "down", "left", "up")
	if got := m.tree.selected().Title; got != "b" {
		t.Fatalf("precondition: expected b selected, got %q", got)
	}
	a2 := find(m.doc, "A2").ID
	if !m.tree.collapsed[a2] {
		t.Fatalf("precondition: A2 should be folded")
	}

	// A second handle adds a task first under A, which shifts b's row index.
	externalAddTask(t, path, "new", find(m.doc, "A").ID, 0)
	m = pollOnce(t, m)

	if find(m.doc, "new") == nil {
		t.Errorf("the external task must be in the reloaded document")
	}
	if got := m.tree.selected().Title; got != "b" {
		t.Errorf("selection must stay on b, got %q", got)
	}
	if !m.tree.collapsed[a2] || hasRow(m, "c") {
		t.Errorf("A2 must stay folded")
	}
	if m.status != "reloaded — list changed externally" {
		t.Errorf("status = %q, want the external-reload notice", m.status)
	}
}

func TestUndoSurvivesOwnReloadClearedByExternal(t *testing.T) {
	m, path := newTestModel(t, "# Work\n\n- [ ] parent\n  - [ ] c1\n  - [ ] c2\n")
	m = press(m, "down") // parent

	// Own writes: cascade-complete, then un-toggle restores the subtasks.
	m = press(m, "space")
	if len(m.snapshots) != 1 {
		t.Fatalf("the cascade must leave a snapshot across its own reload, got %d", len(m.snapshots))
	}
	m = press(m, "space")
	for _, title := range []string{"parent", "c1", "c2"} {
		if got := find(m.doc, title).Status; got != todo.Open {
			t.Errorf("%s = %v after undo, want open", title, got)
		}
	}
	if got, want := todotest.Dump(t, path), "# Work\n\n- [ ] parent\n  - [ ] c1\n  - [ ] c2\n"; got != want {
		t.Errorf("the undo must be persisted:\n%s", got)
	}

	// Cascade again, then an external write lands: the snapshot is dropped, so
	// the un-toggle reopens only the parent (the subtasks keep their status).
	m = press(m, "space")
	externalAddTask(t, path, "other", find(m.doc, "Work").ID, find(m.doc, "parent").ID)
	m = pollOnce(t, m)
	if len(m.snapshots) != 0 {
		t.Fatalf("an external reload must clear the snapshots, got %d", len(m.snapshots))
	}
	m = press(m, "space")
	if got := find(m.doc, "parent").Status; got != todo.Open {
		t.Errorf("parent = %v after un-toggle, want open", got)
	}
	for _, title := range []string{"c1", "c2"} {
		if got := find(m.doc, title).Status; got != todo.Done {
			t.Errorf("%s = %v, want done (no snapshot to restore)", title, got)
		}
	}
}

func TestWriteErrorShowsAndReloads(t *testing.T) {
	m, _ := newTestModel(t, "# Work\n\n- [ ] a\n")
	if err := m.store.Close(); err != nil {
		t.Fatal(err)
	}
	m = press(m, "down", "space")
	if m.err == nil {
		t.Fatal("a failed write must set m.err")
	}
	if got := m.footer(200); !strings.Contains(got, "save failed:") {
		t.Errorf("footer should report the failed save, got %q", got)
	}
	if find(m.doc, "a") == nil || !hasRow(m, "a") {
		t.Errorf("a failed reload must keep the current view")
	}
}

func TestInitStartsPolling(t *testing.T) {
	m, _ := newTestModel(t, "# Work\n")
	if m.Init() == nil {
		t.Errorf("Init must start the revision-poll loop")
	}
}

// hasRow reports whether a visible tree row has the given title.
func hasRow(m model, title string) bool {
	for _, r := range m.tree.rows {
		if r.item != nil && r.item.Title == title {
			return true
		}
	}
	return false
}
