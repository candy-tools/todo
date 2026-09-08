package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/candy-tools/todo/internal/todo"
)

// --- reloadCheck: the pure file-change probe ---

func TestReloadCheckUnchangedIsNil(t *testing.T) {
	path := filepath.Join(t.TempDir(), "todo.md")
	content := "# Work\n\n- [ ] a\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if msg := reloadCheck(path, content); msg != nil {
		t.Errorf("an unchanged file must yield no reload, got %#v", msg)
	}
}

func TestReloadCheckDetectsChange(t *testing.T) {
	path := filepath.Join(t.TempDir(), "todo.md")
	changed := "# Work\n\n- [ ] a\n- [ ] b\n"
	if err := os.WriteFile(path, []byte(changed), 0o644); err != nil {
		t.Fatal(err)
	}
	msg := reloadCheck(path, "# Work\n\n- [ ] a\n")
	r, ok := msg.(fileReloadedMsg)
	if !ok {
		t.Fatalf("a changed file must yield a fileReloadedMsg, got %#v", msg)
	}
	if r.content != changed {
		t.Errorf("reload must carry the new content, got %q", r.content)
	}
	if find(r.doc, "b") == nil {
		t.Errorf("reloaded document must include the new task b")
	}
}

func TestReloadCheckMissingFileIsNil(t *testing.T) {
	if msg := reloadCheck(filepath.Join(t.TempDir(), "nope.md"), "x"); msg != nil {
		t.Errorf("a missing/unreadable file must not trigger a reload, got %#v", msg)
	}
}

// --- applying a reload to the model ---

func TestReloadAppliesExternalChange(t *testing.T) {
	m, _ := newTestModel(t, "# Work\n\n- [ ] a\n")
	m = send(m, fileReloadedMsg{doc: todo.Parse("# Work\n\n- [ ] a\n- [ ] b\n"), content: "# Work\n\n- [ ] a\n- [ ] b\n"})
	if find(m.doc, "b") == nil {
		t.Fatalf("an external change must be applied to the model")
	}
	if !hasRow(m, "b") {
		t.Errorf("the new task must appear in the rebuilt tree")
	}
}

func TestReloadPreservesSelection(t *testing.T) {
	m, _ := newTestModel(t, "# Work\n\n- [ ] a\n- [ ] c\n")
	m = press(m, "down", "down") // Work, a, c -> select c
	if got := m.tree.selected().Title; got != "c" {
		t.Fatalf("precondition: expected c selected, got %q", got)
	}
	m = send(m, fileReloadedMsg{doc: todo.Parse("# Work\n\n- [ ] a\n- [ ] b\n- [ ] c\n"), content: "x"})
	if got := m.tree.selected().Title; got != "c" {
		t.Errorf("selection must stay on c across a reload, got %q", got)
	}
}

func TestReloadPreservesFold(t *testing.T) {
	m, _ := newTestModel(t, "# Work\n\n- [ ] a\n")
	m = press(m, "space") // Work is selected at start; space folds the header, collapsing it
	m = send(m, fileReloadedMsg{doc: todo.Parse("# Work\n\n- [ ] a\n- [ ] b\n"), content: "x"})
	work := m.tree.selected()
	if work.Title != "Work" || !m.tree.collapsed[work] {
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
	src := b.String()
	m, _ := newTestModel(t, src)

	// Scroll down well past the first page so the viewport is offset from the top.
	for i := 0; i < 30; i++ {
		m = press(m, "down")
	}
	wantVH, wantOffset := m.tree.viewHeight, m.tree.offset
	if wantVH <= 1 || wantOffset == 0 {
		t.Fatalf("precondition: want a sized, scrolled viewport, got viewHeight=%d offset=%d", wantVH, wantOffset)
	}
	sel := m.tree.selected().Title

	// An external edit appends a task at the end (below the cursor, so the selected
	// row index is unchanged) and the app reloads.
	changed := src + "- [ ] t30\n"
	m = send(m, fileReloadedMsg{doc: todo.Parse(changed), content: changed})

	if got := m.tree.selected().Title; got != sel {
		t.Fatalf("selection must survive the reload: got %q, want %q", got, sel)
	}
	// Regression: applyReload built a fresh tree and dropped the viewport state,
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
	m, _ := newTestModel(t, "# Work\n\n- [ ] a\n")
	m = press(m, "n") // open the add-task form
	if m.mode != modeForm {
		t.Fatal("precondition: form should be open")
	}
	m = send(m, fileReloadedMsg{doc: todo.Parse("# Work\n\n- [ ] a\n- [ ] b\n"), content: "x"})
	if m.mode != modeForm {
		t.Errorf("a reload must not close an open modal")
	}
	if find(m.doc, "b") != nil {
		t.Errorf("a reload must be deferred while a modal is open")
	}
}

func TestReloadIgnoresOwnWrite(t *testing.T) {
	m, _ := newTestModel(t, "# Work\n\n- [ ] a\n")
	m.lastContent = "the exact bytes we last wrote"
	m = send(m, fileReloadedMsg{doc: todo.Parse("# Work\n\n- [ ] zzz\n"), content: "the exact bytes we last wrote"})
	if find(m.doc, "zzz") != nil {
		t.Errorf("a reload matching our own last write must be ignored")
	}
}

func TestSaveUpdatesLastContent(t *testing.T) {
	m, path := newTestModel(t, "# Work\n\n- [ ] a\n")
	m = press(m, "down", "space") // toggle a done -> triggers a save
	if msg := reloadCheck(path, m.lastContent); msg != nil {
		t.Errorf("after our own save, a poll must see no change, got %#v", msg)
	}
}

func TestInitStartsPolling(t *testing.T) {
	m, _ := newTestModel(t, "# Work\n")
	if m.Init() == nil {
		t.Errorf("Init must start the file-poll loop")
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
