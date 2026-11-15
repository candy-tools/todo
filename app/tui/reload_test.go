package tui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/candy-tools/todo/internal/todo"
	"github.com/candy-tools/todo/internal/todo/todotest"
	"github.com/go-bumbu/db/gormdb"
	"github.com/go-bumbu/db/gormdb/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// externalAddTask adds a task through a second store handle, standing in for a
// CLI command or an agent writing while the TUI runs.
func externalAddTask(t *testing.T, path, title string, parentID, afterID uint) uint {
	t.Helper()
	id, err := openSecond(t, path).AddTask(context.Background(), todo.Fields{Title: title}, todo.Open, parentID, afterID)
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

func TestReloadCheckErrorIsReported(t *testing.T) {
	m, _ := newTestModel(t, "# Work\n\n- [ ] a\n")
	if err := m.store.Close(); err != nil {
		t.Fatal(err)
	}
	msg := reloadCheck(m.store, m.lastRev-1)
	if f, ok := msg.(reloadFailedMsg); !ok || f.err == nil {
		t.Errorf("an unreadable store must be reported as a reloadFailedMsg, got %#v", msg)
	}
}

// openRaw opens a plain database handle on the store file at path, bypassing
// the store's checks; it is closed when the test ends.
func openRaw(t *testing.T, path string) *gorm.DB {
	t.Helper()
	db, err := gormdb.Open(gormdb.Cfg{
		Driver: gormdb.DriverSqlite,
		Sqlite: gormdb.SqliteCfg{Path: path},
		Logger: logger.Discard,
	}, gormdb.Drivers{Sqlite: sqlite.Driver{}})
	if err != nil {
		t.Fatalf("open raw handle: %v", err)
	}
	t.Cleanup(func() { _ = gormdb.Close(db) })
	return db
}

// setSchemaVersion records schema version v in the store at path through a
// second database handle, standing in for a newer todo that migrated the file
// (or, set back, for the file becoming readable again) while the TUI runs.
func setSchemaVersion(t *testing.T, path string, v int) {
	t.Helper()
	db := openRaw(t, path)
	if err := db.Exec("UPDATE meta SET schema_version = ? WHERE id = 1", v).Error; err != nil {
		t.Fatalf("set schema version: %v", err)
	}
}

// schemaVersionOf reads the schema version recorded in the store at path.
func schemaVersionOf(t *testing.T, path string) int {
	t.Helper()
	db := openRaw(t, path)
	var v int
	if err := db.Raw("SELECT schema_version FROM meta WHERE id = 1").Scan(&v).Error; err != nil {
		t.Fatalf("read schema version: %v", err)
	}
	return v
}

// pollCmdOnce runs one poll the way the tick does and feeds its message, if
// any, to the model.
func pollCmdOnce(m model) model {
	if msg := m.reloadCheckCmd()(); msg != nil {
		return send(m, msg)
	}
	return m
}

// TestReloadFailureShownAndCleared upgrades the store under the running TUI:
// the failing poll must put the error in the footer instead of dropping it,
// and the next poll that succeeds must clear it again.
func TestReloadFailureShownAndCleared(t *testing.T) {
	m, path := newTestModel(t, "# Work\n\n- [ ] a\n")
	v := schemaVersionOf(t, path)
	setSchemaVersion(t, path, v+1)

	m = pollCmdOnce(m)
	if !errors.Is(m.err, todo.ErrNewerSchema) {
		t.Fatalf("m.err = %v, want the failed reload (ErrNewerSchema)", m.err)
	}
	if got := m.footer(200); !strings.Contains(got, "reload failed:") || !strings.Contains(got, "newer todo") {
		t.Errorf("footer should report the failed reload, got %q", got)
	}

	setSchemaVersion(t, path, v)
	m = pollCmdOnce(m)
	if m.err != nil {
		t.Errorf("a successful poll must clear the reload error, got %v", m.err)
	}
}

// TestReloadFailureShownOnce has the same poll failure repeat: once shown it
// must not be set again, so an unrelated error that replaced it (a failed
// save) stays in the footer; a poll that succeeds must not clear that one.
func TestReloadFailureShownOnce(t *testing.T) {
	m, path := newTestModel(t, "# Work\n\n- [ ] a\n")
	v := schemaVersionOf(t, path)
	setSchemaVersion(t, path, v+1)

	m = pollCmdOnce(m)
	if !errors.Is(m.err, todo.ErrNewerSchema) {
		t.Fatalf("m.err = %v, want the failed reload", m.err)
	}
	saveErr := errors.New("save failed: boom")
	m.err = saveErr
	m = pollCmdOnce(m)
	if !errors.Is(m.err, saveErr) {
		t.Errorf("a repeated reload failure must not replace another error, got %v", m.err)
	}

	setSchemaVersion(t, path, v)
	m = pollCmdOnce(m)
	if !errors.Is(m.err, saveErr) {
		t.Errorf("a successful poll must not clear an unrelated error, got %v", m.err)
	}
}

// TestReloadFailureDistinctShown has the poll fail with a different error: it
// replaces the one shown.
func TestReloadFailureDistinctShown(t *testing.T) {
	m, path := newTestModel(t, "# Work\n\n- [ ] a\n")
	v := schemaVersionOf(t, path)
	setSchemaVersion(t, path, v+1)
	m = pollCmdOnce(m)
	m.err = errors.New("save failed: boom")
	setSchemaVersion(t, path, v+2)
	m = pollCmdOnce(m)
	if got := m.footer(200); !strings.Contains(got, fmt.Sprintf("schema %d", v+2)) {
		t.Errorf("a distinct reload failure must be shown, got %q", got)
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

func TestReloadAppliedWhileSearchBarOpen(t *testing.T) {
	m, path := newTestModel(t, "# Work\n\n- [ ] a\n")
	m = press(m, "/")
	m = typeText(m, "a")
	externalAddTask(t, path, "ab", find(m.doc, "Work").ID, find(m.doc, "a").ID)
	m = pollOnce(t, m)
	if find(m.doc, "ab") == nil {
		t.Errorf("a reload must be applied while the search bar is open")
	}
	if m.mode != modeSearch {
		t.Errorf("a reload must leave the search bar open")
	}
	if m.view.filter != "a" || m.search.Value() != "a" {
		t.Errorf("a reload must keep the query: filter %q, input %q", m.view.filter, m.search.Value())
	}
	if !hasRow(m, "ab") {
		t.Errorf("the new task matches the query and must be shown, rows %v", rowTitles(m.tree))
	}
}

func TestReloadDeferredWhileGrabbed(t *testing.T) {
	m, path := newTestModel(t, "# Work\n\n- [ ] a\n")
	m = selectTitle(t, m, "a")
	m = press(m, "m")
	if !m.tree.grabbed {
		t.Fatal("precondition: m should grab a")
	}
	externalAddTask(t, path, "b", find(m.doc, "Work").ID, find(m.doc, "a").ID)
	m = pollOnce(t, m)
	if find(m.doc, "b") != nil {
		t.Errorf("a reload must be deferred while an item is grabbed")
	}
	if !m.tree.grabbed {
		t.Errorf("a deferred reload must leave the item grabbed")
	}
	m = press(m, "m") // drop
	m = pollOnce(t, m)
	if find(m.doc, "b") == nil {
		t.Errorf("the deferred reload must apply after the item is dropped")
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
	m = pickState(t, press(m, "down"), todo.Done) // complete a -> a store write
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
	m, path := newTestModelOn(t, "# Work\n\n- [/] parent\n  - [ ] c1\n  - [ ] c2\n", todo.ColInProgress)
	m = press(m, "down") // parent

	// Own writes: cascade-complete, then un-toggle restores the subtasks.
	m = pickState(t, m, todo.Done)
	if len(m.snapshots) != 1 {
		t.Fatalf("the cascade must leave a snapshot across its own reload, got %d", len(m.snapshots))
	}
	m = showOn(t, m, todo.ColDone, "parent") // the done parent left In Progress
	m = press(m, "t", "enter")               // t starts on the prior state: enter undoes
	if got := find(m.doc, "parent").Status; got != todo.Working {
		t.Errorf("parent = %v after undo, want working", got)
	}
	for _, title := range []string{"c1", "c2"} {
		if got := find(m.doc, title).Status; got != todo.Open {
			t.Errorf("%s = %v after undo, want open", title, got)
		}
	}
	if got, want := todotest.Dump(t, path), "# Work\n\n- [/] parent\n  - [ ] c1\n  - [ ] c2\n"; got != want {
		t.Errorf("the undo must be persisted:\n%s", got)
	}

	// Cascade again, then an external write lands: the snapshot is dropped, so
	// the un-toggle reopens only the parent (the subtasks keep their status).
	m = showOn(t, m, todo.ColInProgress, "parent")
	m = pickState(t, m, todo.Done)
	externalAddTask(t, path, "other", find(m.doc, "Work").ID, find(m.doc, "parent").ID)
	m = pollOnce(t, m)
	if len(m.snapshots) != 0 {
		t.Fatalf("an external reload must clear the snapshots, got %d", len(m.snapshots))
	}
	m = showOn(t, m, todo.ColDone, "parent")
	if m2 := press(m, "t"); m2.pick.selected() != todo.Done {
		t.Errorf("without a snapshot t must start on done, got %v", m2.pick.selected())
	}
	m = pickState(t, m, todo.Working)
	if got := find(m.doc, "parent").Status; got != todo.Working {
		t.Errorf("parent = %v after reopening without a snapshot, want working", got)
	}
	for _, title := range []string{"c1", "c2"} {
		if got := find(m.doc, title).Status; got != todo.Done {
			t.Errorf("%s = %v, want done (no snapshot to restore)", title, got)
		}
	}
}

// TestOwnWriteDetectsExternalWriteBeforePoll covers an external write landing
// between polls: the app's next own write must notice that the revision moved
// by more than its own bump, treat the reload as external and drop the undo
// snapshots, so un-toggling the parent never writes old statuses over it.
func TestOwnWriteDetectsExternalWriteBeforePoll(t *testing.T) {
	m, path := newTestModelOn(t, "# Work\n\n- [/] parent\n  - [ ] c1\n  - [ ] c2\n- [ ] other\n", todo.ColInProgress)
	m = pickState(t, press(m, "down"), todo.Done) // cascade-complete parent
	if len(m.snapshots) != 1 {
		t.Fatalf("precondition: the cascade must leave a snapshot, got %d", len(m.snapshots))
	}

	// An agent reopens c1 through a second handle; no poll runs.
	c1 := find(m.doc, "c1").ID
	if err := openSecond(t, path).SetStatuses(context.Background(), map[uint]todo.Status{c1: todo.Open}); err != nil {
		t.Fatalf("external write: %v", err)
	}

	// An own, unrelated write: defer "other".
	m = showOn(t, m, todo.ColBacklog, "other")
	m = pickState(t, m, todo.Deferred)
	if len(m.snapshots) != 0 {
		t.Errorf("an own write that absorbed an external one must clear the snapshots, got %d", len(m.snapshots))
	}
	if m.status != "reloaded — list changed externally" {
		t.Errorf("status = %q, want the external-reload notice", m.status)
	}

	// Un-toggle the parent: it reopens to working, c1 keeps the agent's status,
	// and c2 stays done because the dropped snapshot no longer restores it.
	m = showOn(t, m, todo.ColDone, "parent")
	m = pickState(t, m, todo.Working)
	if got := find(m.doc, "c1").Status; got != todo.Open {
		t.Errorf("c1 = %v after un-toggle, want open (the agent's change)", got)
	}
	if got, want := todotest.Dump(t, path), "# Work\n\n- [/] parent\n  - [ ] c1\n  - [x] c2\n- [>] other\n"; got != want {
		t.Errorf("store after un-toggle:\n%s\nwant:\n%s", got, want)
	}
}

func TestWriteErrorShowsAndReloads(t *testing.T) {
	m, _ := newTestModelOn(t, "# Work\n\n- [/] a\n", todo.ColInProgress)
	if err := m.store.Close(); err != nil {
		t.Fatal(err)
	}
	m = pickState(t, press(m, "down"), todo.Done)
	if m.err == nil {
		t.Fatal("a failed write must set m.err")
	}
	if got := m.footer(200); !strings.Contains(got, "save failed:") {
		t.Errorf("footer should report the failed save, got %q", got)
	}
	// The write and the reload both failed: the unsaved toggle must not stay on
	// screen as if saved, so a is still working on the In Progress tab.
	if got := find(m.doc, "a").Status; got != todo.Working {
		t.Errorf("a = %v after a failed save, want working (the unsaved change undone)", got)
	}
	if !hasRow(m, "a") || m.tree.selected() == nil || m.tree.selected().Title != "a" {
		t.Errorf("a must stay shown and selected on In Progress, rows %v", rowTitles(m.tree))
	}
}

// TestFailedWriteAndReloadRestoresTree deletes an item while the store is
// unreachable: the write and the reload both fail, so the view goes back to the
// document as last loaded — the item is still there, its rows rebuilt — and
// the selection stays valid.
func TestFailedWriteAndReloadRestoresTree(t *testing.T) {
	m, _ := newTestModel(t, "# Work\n\n- [ ] a\n- [ ] b\n")
	m = press(m, "down", "down") // b
	if err := m.store.Close(); err != nil {
		t.Fatal(err)
	}
	m = press(m, "d", "y")
	if m.err == nil || !strings.Contains(m.footer(200), "save failed:") {
		t.Errorf("footer should report the failed save, got %q", m.footer(200))
	}
	if find(m.doc, "b") == nil || !hasRow(m, "b") {
		t.Errorf("the unsaved delete must be undone in the view, rows %v", rowTitles(m.tree))
	}
	if sel := m.tree.selected(); sel == nil || sel.Title != "a" {
		t.Errorf("selection = %v, want a (the row above the item whose delete failed)", sel)
	}
}

// TestFailedWriteAndReloadRestoresEdit edits a task while the store is
// unreachable: the edited title must not stay on screen as if saved, and a
// second failed write must still restore the same, last loaded document.
func TestFailedWriteAndReloadRestoresEdit(t *testing.T) {
	m, _ := newTestModel(t, "# Work\n\n- [ ] a\n")
	if err := m.store.Close(); err != nil {
		t.Fatal(err)
	}
	m = press(m, "down") // a
	for range 2 {
		m = press(m, "e")
		if m.mode != modeForm {
			t.Fatalf("e should open the edit form, mode = %v", m.mode)
		}
		m = typeText(m, "-edited")
		m = press(m, "tab", "tab", "enter")
		if m.err == nil || !strings.Contains(m.footer(200), "save failed:") {
			t.Errorf("footer should report the failed save, got %q", m.footer(200))
		}
		if find(m.doc, "a-edited") != nil || find(m.doc, "a") == nil || !hasRow(m, "a") {
			t.Errorf("the unsaved edit must be undone in the view, rows %v", rowTitles(m.tree))
		}
		if sel := m.tree.selected(); sel == nil || sel.Title != "a" {
			t.Errorf("selection = %v, want a", sel)
		}
	}
}

// TestReloadFailureAfterSavedWrite has the write succeed and only the reload
// fail: the footer must say the reload failed, not the save, and the rows must
// match the in-memory change.
func TestReloadFailureAfterSavedWrite(t *testing.T) {
	m, path := newTestModel(t, "# Work\n\n- [ ] a\n- [ ] b\n")
	a, b := find(m.doc, "a"), find(m.doc, "b")
	m.doc.Remove(b)
	m.commit(a.ID, func(ctx context.Context) error {
		err := m.store.Delete(ctx, b.ID)
		_ = m.store.Close() // the reload that follows fails
		return err
	})
	if got := m.footer(200); !strings.Contains(got, "reload failed:") || strings.Contains(got, "save failed") {
		t.Errorf("footer should report the failed reload, got %q", got)
	}
	if hasRow(m, "b") {
		t.Errorf("the tree rows must be rebuilt after a failed reload")
	}
	if got := m.tree.selected().Title; got != "a" {
		t.Errorf("selection = %q, want a", got)
	}
	if got := todotest.Dump(t, path); strings.Contains(got, "b") {
		t.Errorf("the delete must be saved:\n%s", got)
	}
}

// TestOwnWriteWithFailedReloadNotExternal has an own add succeed while the
// reload after it fails once. The next poll must load the list (the added task
// gets its stored ID) as the app's own write: no external-reload notice, and
// the undo snapshots kept.
func TestOwnWriteWithFailedReloadNotExternal(t *testing.T) {
	m, path := newTestModel(t, "# Work\n\n- [ ] a\n")
	v := schemaVersionOf(t, path)
	work, a := find(m.doc, "Work"), find(m.doc, "a")
	m.snapshots[a.ID] = todo.DoneStates{a.ID: todo.Open}
	added := todo.NewTask("b", "", false)
	work.AppendTask(added)
	m.commit(0, func(ctx context.Context) error {
		_, err := m.store.AppendTask(ctx, todo.Fields{Title: "b"}, todo.Open, work.ID)
		setSchemaVersion(t, path, v+1) // the reload that follows fails
		return err
	})
	if !strings.Contains(m.footer(200), "reload failed:") {
		t.Fatalf("precondition: footer should report the failed reload, got %q", m.footer(200))
	}
	setSchemaVersion(t, path, v)

	m = pollCmdOnce(m)
	if m.status == "reloaded — list changed externally" {
		t.Errorf("the app's own write must not be reported as an external change")
	}
	if len(m.snapshots) != 1 {
		t.Errorf("the app's own write must keep the undo snapshots, got %d", len(m.snapshots))
	}
	if b := find(m.doc, "b"); b == nil || b.ID == 0 {
		t.Errorf("the poll must load the list the write left, got b = %v", b)
	}
	if msg := reloadCheck(m.store, m.lastRev); msg != nil {
		t.Errorf("once loaded, a poll must see no change, got %#v", msg)
	}
}

// TestFailedWriteRevertsToStore has the write fail while the reload works: the
// in-memory change is discarded and the view shows what the store holds.
func TestFailedWriteRevertsToStore(t *testing.T) {
	m, _ := newTestModel(t, "# Work\n\n- [ ] a\n- [ ] b\n")
	b := find(m.doc, "b")
	m.doc.Remove(b)
	m.commit(b.ID, func(context.Context) error { return errors.New("boom") })
	if got := m.footer(200); !strings.Contains(got, "save failed: boom") {
		t.Errorf("footer should report the failed save, got %q", got)
	}
	if find(m.doc, "b") == nil || !hasRow(m, "b") {
		t.Errorf("the view must revert to the store's state, which still has b")
	}
	if got := m.tree.selected().Title; got != "b" {
		t.Errorf("selection = %q, want b", got)
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

// TestReplacedFileShowsErrorAndRefusesWrite swaps another store in over the
// open one by rename, as `git checkout` or a sync tool would: the poll must
// report it, and a write must be refused rather than land in the unlinked file
// and vanish when the TUI quits.
func TestReplacedFileShowsErrorAndRefusesWrite(t *testing.T) {
	m, path := newTestModelOn(t, "# Work\n\n- [/] a\n", todo.ColInProgress)
	// A second link keeps the file the store holds reachable once another is
	// renamed over its path, so the end of the test can read it.
	held := path + ".held"
	if err := os.Link(path, held); err != nil {
		t.Fatal(err)
	}
	other := todotest.Seed(t, "# Other\n")
	if err := os.Rename(other, path); err != nil {
		t.Fatal(err)
	}

	msg := reloadCheck(m.store, m.lastRev)
	if msg == nil {
		t.Fatal("the poll must report a replaced store file")
	}
	m = send(m, msg)
	if m.err == nil || !errors.Is(m.err, todo.ErrFileReplaced) {
		t.Fatalf("m.err = %v, want ErrFileReplaced", m.err)
	}

	m = pickState(t, press(m, "down"), todo.Done)
	if !errors.Is(m.err, todo.ErrFileReplaced) || !strings.Contains(m.footer(200), "save failed:") {
		t.Errorf("footer should report the refused save, got %q", m.footer(200))
	}
	if _, _, err := m.store.Load(context.Background()); !errors.Is(err, todo.ErrFileReplaced) {
		t.Fatalf("Load = %v, want ErrFileReplaced (a read may run on the file now at the path)", err)
	}
	if err := m.store.Close(); err != nil {
		t.Fatal(err)
	}
	st, err := todo.OpenStore(held)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	doc, _, err := st.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got := find(doc, "a").Status; got != todo.Working {
		t.Errorf("a = %v in the unlinked file, want working (the write must be refused)", got)
	}
}

// A status change while another process holds the writer lock fails fast with
// the store's busy message in the footer, instead of freezing the UI for the
// 5 s busy timeout, and leaves the task where it was.
func TestStatusChangeWhileLockedShowsBusy(t *testing.T) {
	m, path := newTestModel(t, "# C\n- [ ] a\n")
	m = selectTitle(t, m, "a")
	todotest.HoldWriteLock(t, path)

	start := time.Now()
	m = press(m, "p")
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("the status change blocked the UI for %v", elapsed)
	}
	if got := m.footer(200); !strings.Contains(got, "save failed: "+todo.ErrBusy.Error()) {
		t.Errorf("footer = %q, want the busy message", got)
	}
	if st := find(m.doc, "a").Status; st != todo.Open {
		t.Errorf("a shows %v after the failed save, want open", st)
	}
}

// statusIn reads the status of the item titled title from the store file at
// path through a second handle.
func statusIn(t *testing.T, path, title string) todo.Status {
	t.Helper()
	doc, _, err := openSecond(t, path).Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	it := find(doc, title)
	if it == nil {
		t.Fatalf("%q is not in the store at %s", title, path)
	}
	return it.Status
}

// TestReplacedFileIsReopened renames another store in over the open one after
// the TUI wrote to it: the next poll must reopen the store at the path, show
// the new file's list as an external change, clear the error, and send the
// writes that follow into the new file.
func TestReplacedFileIsReopened(t *testing.T) {
	m, path := newTestModel(t, "# Work\n\n- [ ] a\n")
	m = press(selectTitle(t, m, "a"), "p") // a write, so the old store has a WAL
	other := todotest.Seed(t, "# Other\n\n- [ ] b\n")
	if err := os.Rename(other, path); err != nil {
		t.Fatal(err)
	}
	m = send(m, reloadCheck(m.store, m.lastRev))
	if !errors.Is(m.err, todo.ErrFileReplaced) {
		t.Fatalf("m.err = %v, want ErrFileReplaced", m.err)
	}

	m = pollCmdOnce(m)
	t.Cleanup(func() { _ = m.store.Close() })
	if m.err != nil {
		t.Errorf("the reopen must clear the error, got %v", m.err)
	}
	if find(m.doc, "b") == nil || find(m.doc, "a") != nil || !hasRow(m, "b") {
		t.Fatalf("the view must show the new file's list, rows %v", rowTitles(m.tree))
	}
	if !strings.Contains(m.status, "changed externally") {
		t.Errorf("status = %q, want the external-change notice", m.status)
	}

	m = press(selectTitle(t, m, "b"), "p")
	if m.err != nil {
		t.Fatalf("a write after the reopen failed: %v", m.err)
	}
	if got := statusIn(t, path, "b"); got == todo.Open {
		t.Errorf("b = %v in the file at the path, want the status change written there", got)
	}
}

// TestRemovedFileIsNotRecreated removes the store file under the TUI: the
// poll must keep reporting it and never create a new database at the path;
// once a store file is put back, the next poll reopens it.
func TestRemovedFileIsNotRecreated(t *testing.T) {
	m, path := newTestModel(t, "# Work\n\n- [ ] a\n")
	if err := os.Rename(path, path+".moved"); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		m = pollCmdOnce(m)
		if !errors.Is(m.err, todo.ErrFileReplaced) {
			t.Fatalf("m.err = %v, want ErrFileReplaced", m.err)
		}
		if ok, err := todo.StoreExists(path); err != nil || ok {
			t.Fatalf("a store file exists at the path after the poll (%v, %v); none may be created", ok, err)
		}
	}

	if err := os.Rename(todotest.Seed(t, "# Other\n\n- [ ] b\n"), path); err != nil {
		t.Fatal(err)
	}
	m = pollCmdOnce(m)
	t.Cleanup(func() { _ = m.store.Close() })
	if m.err != nil || find(m.doc, "b") == nil {
		t.Errorf("the file put back must be reopened: err %v, rows %v", m.err, rowTitles(m.tree))
	}
}

// TestReopenDeferredWhileModal replaces the store file while a dialog is open:
// the reopen must wait for plain navigation, so a dialog opened on the old
// list never writes to the new file, whose IDs name other items.
func TestReopenDeferredWhileModal(t *testing.T) {
	m, path := newTestModel(t, "# Work\n\n- [ ] a\n")
	m = press(selectTitle(t, m, "a"), "t")
	if err := os.Rename(todotest.Seed(t, "# Other\n\n- [ ] b\n"), path); err != nil {
		t.Fatal(err)
	}
	old := m.store
	m = pollCmdOnce(m)
	if m.store != old || find(m.doc, "a") == nil {
		t.Fatalf("the store must not be reopened while a dialog is open")
	}
	m = press(m, "esc")
	m = pollCmdOnce(m)
	t.Cleanup(func() { _ = m.store.Close() })
	if m.store == old || find(m.doc, "b") == nil {
		t.Errorf("back in navigation, the poll must reopen the store")
	}
}
