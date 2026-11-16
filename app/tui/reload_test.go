package tui

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/candy-tools/todo/internal/todo"
	"github.com/candy-tools/todo/internal/todo/todotest"
)

// halfWritten is a list file cut off mid-write, as another tool writing in
// place could leave it for a moment.
const halfWritten = `{"version":3,`

// externalAddTask adds a task through a second store handle, standing in for a
// CLI command or an agent writing while the TUI runs.
func externalAddTask(t *testing.T, path, title string, parentID, afterID string) string {
	t.Helper()
	id, _, err := openSecond(t, path).AddTask(context.Background(), todo.Fields{Title: title}, todo.Open, parentID, afterID)
	if err != nil {
		t.Fatalf("external add: %v", err)
	}
	return id
}

// pollOnce runs one change probe and feeds its message, which must not be
// nil, to the model.
func pollOnce(t *testing.T, m model) model {
	t.Helper()
	msg := reloadCheck(m.store, m.lastFP)
	if msg == nil {
		t.Fatal("expected the poll to report a change")
	}
	return send(m, msg)
}

// pollCmdOnce runs one poll the way the tick does and feeds its message, if
// any, to the model.
func pollCmdOnce(m model) model {
	if msg := m.reloadCheckCmd()(); msg != nil {
		return send(m, msg)
	}
	return m
}

// readList returns the bytes of the list file at path.
func readList(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read list: %v", err)
	}
	return data
}

// writeList puts data at path in place, past the store, standing in for
// another tool writing the file.
func writeList(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write list: %v", err)
	}
}

// withVersion returns the list file data with its format version set to v,
// standing in for a file a newer todo wrote.
func withVersion(t *testing.T, data []byte, v int) []byte {
	t.Helper()
	out := bytes.Replace(data, []byte(`"version": 3`), fmt.Appendf(nil, `"version": %d`, v), 1)
	if bytes.Equal(out, data) {
		t.Fatalf("no version in the list file:\n%s", data)
	}
	return out
}

// --- reloadCheck: the change probe ---

func TestReloadCheckUnchangedIsNil(t *testing.T) {
	m, _ := newTestModel(t, "# Work\n\n- [ ] a\n")
	if msg := reloadCheck(m.store, m.lastFP); msg != nil {
		t.Errorf("an unchanged store must yield no reload, got %#v", msg)
	}
}

func TestReloadCheckDetectsChange(t *testing.T) {
	m, path := newTestModel(t, "# Work\n\n- [ ] a\n")
	work, a := find(m.doc, "Work"), find(m.doc, "a")
	externalAddTask(t, path, "b", work.ID, a.ID)
	msg := reloadCheck(m.store, m.lastFP)
	r, ok := msg.(storeReloadedMsg)
	if !ok {
		t.Fatalf("a changed store must yield a storeReloadedMsg, got %#v", msg)
	}
	if r.fp == m.lastFP {
		t.Errorf("the reload must carry the new fingerprint")
	}
	if changed, _, err := m.store.Changed(context.Background(), r.fp); err != nil || changed {
		t.Errorf("the reload's fingerprint must be the file's (changed %v, err %v)", changed, err)
	}
	if find(r.doc, "b") == nil {
		t.Errorf("reloaded document must include the new task b")
	}
}

func TestReloadCheckInvalidFileIsReported(t *testing.T) {
	m, path := newTestModel(t, "# Work\n\n- [ ] a\n")
	writeList(t, path, []byte(halfWritten))
	msg := reloadCheck(m.store, m.lastFP)
	if f, ok := msg.(reloadFailedMsg); !ok || !errors.Is(f.err, todo.ErrInvalidList) {
		t.Errorf("an invalid list file must be reported as a reloadFailedMsg, got %#v", msg)
	}
}

func TestReloadCheckRemovedFileIsReported(t *testing.T) {
	m, path := newTestModel(t, "# Work\n\n- [ ] a\n")
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	msg := reloadCheck(m.store, m.lastFP)
	if f, ok := msg.(reloadFailedMsg); !ok || !errors.Is(f.err, errListRemoved) {
		t.Errorf("a removed list file must be reported as errListRemoved, got %#v", msg)
	}
}

// --- live reload and the app's own writes ---

// TestReloadPicksUpOutsideWrite has a second store add a task: one poll shows
// it, keeps the selection on the same item and says the list changed.
func TestReloadPicksUpOutsideWrite(t *testing.T) {
	m, path := newTestModel(t, "# Work\n\n- [ ] a\n- [ ] c\n")
	m = selectTitle(t, m, "c")
	want := m.tree.selected().ID
	externalAddTask(t, path, "b", find(m.doc, "Work").ID, find(m.doc, "a").ID)

	m = pollCmdOnce(m)
	if find(m.doc, "b") == nil || !hasRow(m, "b") {
		t.Fatalf("the outside write must be shown, rows %v", rowTitles(m.tree))
	}
	if sel := m.tree.selected(); sel == nil || sel.ID != want {
		t.Errorf("selection = %v, want the same item c", sel)
	}
	if m.status != "reloaded — list changed externally" {
		t.Errorf("status = %q, want the external-reload notice", m.status)
	}
}

// TestOwnWriteIsNotExternal completes a task: the app's own write shows no
// external notice, keeps the undo snapshot, and the next poll sees no change.
func TestOwnWriteIsNotExternal(t *testing.T) {
	m, _ := newTestModelOn(t, "# Work\n\n- [/] parent\n  - [ ] c1\n", todo.ColInProgress)
	before := m.lastFP
	m = pickState(t, selectTitle(t, m, "parent"), todo.Done)
	if m.err != nil {
		t.Fatalf("the write failed: %v", m.err)
	}
	if strings.Contains(m.status, "changed externally") {
		t.Errorf("status = %q, an own write must not be reported as external", m.status)
	}
	if len(m.snapshots) != 1 {
		t.Errorf("an own write must keep the undo snapshot, got %d", len(m.snapshots))
	}
	if m.lastFP == before {
		t.Errorf("an own write must record the fingerprint it wrote")
	}
	if msg := reloadCheck(m.store, m.lastFP); msg != nil {
		t.Errorf("after our own write, a poll must see no change, got %#v", msg)
	}
}

// TestCommitAfterOutsideWriteIsExternal has a second store rename a task, then
// the TUI complete another without polling first: the write finds the file
// changed, so the result is handled as external and keeps both changes.
func TestCommitAfterOutsideWriteIsExternal(t *testing.T) {
	m, path := newTestModelOn(t, "# Work\n\n- [/] a\n  - [ ] a1\n- [/] b\n", todo.ColInProgress)
	m = pickState(t, selectTitle(t, m, "a"), todo.Done) // leaves an undo snapshot
	if len(m.snapshots) != 1 {
		t.Fatalf("precondition: the complete must leave a snapshot, got %d", len(m.snapshots))
	}
	title := "b renamed"
	if _, err := openSecond(t, path).Edit(context.Background(), find(m.doc, "b").ID, todo.Changes{Title: &title}); err != nil {
		t.Fatalf("external edit: %v", err)
	}

	m = pickState(t, selectTitle(t, m, "b"), todo.Done)
	if m.err != nil {
		t.Fatalf("the write failed: %v", m.err)
	}
	if m.status != "reloaded — list changed externally" {
		t.Errorf("status = %q, want the external-reload notice", m.status)
	}
	if len(m.snapshots) != 0 {
		t.Errorf("a write that found an outside change must drop the snapshots, got %d", len(m.snapshots))
	}
	if b := find(m.doc, title); b == nil || b.Status != todo.Done {
		t.Errorf("m.doc must hold both changes, b renamed = %v", b)
	}
	if msg := reloadCheck(m.store, m.lastFP); msg != nil {
		t.Errorf("the write's result is what the file holds, got %#v", msg)
	}
}

// TestHalfWrittenFileKeepsView breaks the file: the first two failed polls
// show nothing, the third shows the error, and the view never changes. A
// valid file again is reloaded and clears the error.
func TestHalfWrittenFileKeepsView(t *testing.T) {
	m, path := newTestModel(t, "# Work\n\n- [ ] a\n")
	good := readList(t, path)
	rows := rowTitles(m.tree)
	writeList(t, path, []byte(halfWritten))

	for tick := 1; tick <= 2; tick++ {
		m = pollCmdOnce(m)
		if m.err != nil {
			t.Fatalf("tick %d: a transient read failure must not be shown yet, got %v", tick, m.err)
		}
		if got := rowTitles(m.tree); !slices.Equal(got, rows) {
			t.Fatalf("tick %d: rows = %v, want %v", tick, got, rows)
		}
	}
	m = pollCmdOnce(m)
	if m.err == nil || !strings.Contains(m.err.Error(), "reload failed") {
		t.Fatalf("tick 3: m.err = %v, want the reload failure", m.err)
	}
	if got := rowTitles(m.tree); !slices.Equal(got, rows) {
		t.Errorf("tick 3: rows = %v, want %v", got, rows)
	}

	writeList(t, path, good)
	externalAddTask(t, path, "b", find(m.doc, "Work").ID, find(m.doc, "a").ID)
	m = pollCmdOnce(m)
	if m.err != nil || m.pollFails != 0 {
		t.Errorf("the reload must clear the error: err %v, pollFails %d", m.err, m.pollFails)
	}
	if !hasRow(m, "b") {
		t.Errorf("the valid file must be reloaded, rows %v", rowTitles(m.tree))
	}
}

// TestRestoredFileClearsPollError breaks the file until the error shows, then
// puts back its exact bytes: the poll finds the file unchanged, and must
// still take the error out of the footer.
func TestRestoredFileClearsPollError(t *testing.T) {
	m, path := newTestModel(t, "# Work\n\n- [ ] a\n")
	good := readList(t, path)
	writeList(t, path, []byte(halfWritten))
	for range 3 {
		m = pollCmdOnce(m)
	}
	if m.err == nil {
		t.Fatal("precondition: three failed polls must show the error")
	}

	writeList(t, path, good)
	m = pollCmdOnce(m)
	if m.err != nil || m.pollErr != nil || m.pollFails != 0 {
		t.Errorf("an unchanged poll after failures must clear them: err %v, pollErr %v, pollFails %d",
			m.err, m.pollErr, m.pollFails)
	}
}

// TestRemovedFileKeepsView removes the list file: after three polls the footer
// says so, the rows stay and no file is created; a file written back is
// reloaded.
func TestRemovedFileKeepsView(t *testing.T) {
	m, path := newTestModel(t, "# Work\n\n- [ ] a\n")
	rows := rowTitles(m.tree)
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	for range 3 {
		m = pollCmdOnce(m)
	}
	if got := m.footer(200); !strings.Contains(got, "list file was removed") {
		t.Errorf("footer = %q, want the removed-file error", got)
	}
	if got := rowTitles(m.tree); !slices.Equal(got, rows) {
		t.Errorf("rows = %v, want %v", got, rows)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("a list file exists at the path after the polls (stat err %v); none may be created", err)
	}

	// A save finds no task to change: it fails, and the view keeps the list
	// rather than show the empty one a missing file reads as.
	m = pickState(t, selectTitle(t, m, "a"), todo.Done)
	if !errors.Is(m.err, todo.ErrNotFound) {
		t.Errorf("m.err = %v, want the failed save (ErrNotFound)", m.err)
	}
	if got := rowTitles(m.tree); !slices.Equal(got, rows) {
		t.Errorf("after the failed save rows = %v, want %v", got, rows)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("a list file exists at the path after the failed save (stat err %v)", err)
	}

	if _, _, err := openSecond(t, path).AppendCategory(context.Background(), "Back", ""); err != nil {
		t.Fatalf("write the file back: %v", err)
	}
	m = pollCmdOnce(m)
	if m.pollErr != nil || find(m.doc, "Back") == nil || find(m.doc, "a") != nil {
		t.Errorf("the file written back must be reloaded: poll error %v, rows %v", m.pollErr, rowTitles(m.tree))
	}
}

// TestNoOpWriteWhileFileRemovedKeepsView removes the file and clears the done
// tasks: the write finds an empty list with nothing to remove and writes
// nothing. It must not adopt that empty list or clear the removed-file error:
// it fails as removed, and the view keeps the list.
func TestNoOpWriteWhileFileRemovedKeepsView(t *testing.T) {
	m, path := newTestModel(t, "# Work\n\n- [x] a\n- [ ] b\n")
	rows, fp := rowTitles(m.tree), m.lastFP
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	for range 3 {
		m = pollCmdOnce(m)
	}
	if !errors.Is(m.err, errListRemoved) {
		t.Fatalf("precondition: m.err = %v, want the removed-file error", m.err)
	}

	m = press(m, "D", "y")
	if !errors.Is(m.err, errListRemoved) {
		t.Errorf("m.err = %v, want the write to fail as removed", m.err)
	}
	if got := m.footer(200); !strings.Contains(got, "list file was removed") || strings.Contains(got, "Removed 0") {
		t.Errorf("footer = %q, want the removed-file error", got)
	}
	if got := rowTitles(m.tree); !slices.Equal(got, rows) {
		t.Errorf("rows = %v, want %v", got, rows)
	}
	if m.lastFP != fp {
		t.Errorf("lastFP moved to %x, want the last loaded %x", m.lastFP, fp)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("a list file exists at the path after the no-op write (stat err %v)", err)
	}
}

// TestSaveFailsOnInvalidFile breaks the file and completes a task: the write
// is refused, the file is left alone, and the view shows the last good list.
func TestSaveFailsOnInvalidFile(t *testing.T) {
	m, path := newTestModel(t, "# Work\n\n- [ ] a\n")
	writeList(t, path, []byte(halfWritten))

	m = pickState(t, selectTitle(t, m, "a"), todo.Done)
	got := m.footer(200)
	if !strings.Contains(got, "save failed:") || !strings.Contains(got, todo.ErrInvalidList.Error()) {
		t.Errorf("footer = %q, want the refused save naming the invalid list file", got)
	}
	if data := readList(t, path); string(data) != halfWritten {
		t.Errorf("the refused save changed the file: %q", data)
	}
	if a := find(m.doc, "a"); a == nil || a.Status != todo.Open || !hasRow(m, "a") {
		t.Errorf("the view must show the last good list, a = %v, rows %v", a, rowTitles(m.tree))
	}
}

// TestSaveBusy completes a task while another process holds the write lock:
// the save fails fast with the busy message, instead of freezing the UI for
// the CLI's busy timeout, and the task stays as it was.
func TestSaveBusy(t *testing.T) {
	m, path := newTestModel(t, "# C\n- [ ] a\n")
	m = selectTitle(t, m, "a")
	todotest.HoldWriteLock(t, path)

	start := time.Now()
	m = pickState(t, m, todo.Done)
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("the save blocked the UI for %v", elapsed)
	}
	if got := m.footer(200); !strings.Contains(got, "save failed: list busy") {
		t.Errorf("footer = %q, want the busy message", got)
	}
	if st := find(m.doc, "a").Status; st != todo.Open || !hasRow(m, "a") {
		t.Errorf("a shows %v after the failed save, want open and shown", st)
	}
}

// --- poll failures in the footer ---

// failPolls runs n polls on a list file a newer todo wrote (format version v),
// enough to show the failure once n reaches 3.
func failPolls(t *testing.T, m model, path string, good []byte, v, n int) model {
	t.Helper()
	writeList(t, path, withVersion(t, good, v))
	for range n {
		m = pollCmdOnce(m)
	}
	return m
}

// TestReloadFailureShownAndCleared has a newer todo write the file: the
// failing polls put the error in the footer, and the next poll that loads the
// list clears it.
func TestReloadFailureShownAndCleared(t *testing.T) {
	m, path := newTestModel(t, "# Work\n\n- [ ] a\n")
	good := readList(t, path)
	m = failPolls(t, m, path, good, 4, 3)
	if !errors.Is(m.err, todo.ErrNewerSchema) {
		t.Fatalf("m.err = %v, want the failed reload (ErrNewerSchema)", m.err)
	}
	if got := m.footer(200); !strings.Contains(got, "reload failed:") || !strings.Contains(got, "newer todo") {
		t.Errorf("footer should report the failed reload, got %q", got)
	}

	writeList(t, path, good)
	externalAddTask(t, path, "b", find(m.doc, "Work").ID, "")
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
	good := readList(t, path)
	m = failPolls(t, m, path, good, 4, 3)
	if !errors.Is(m.err, todo.ErrNewerSchema) {
		t.Fatalf("m.err = %v, want the failed reload", m.err)
	}
	saveErr := errors.New("save failed: boom")
	m.err = saveErr
	m = pollCmdOnce(m)
	if !errors.Is(m.err, saveErr) {
		t.Errorf("a repeated reload failure must not replace another error, got %v", m.err)
	}

	writeList(t, path, good)
	m = pollCmdOnce(m)
	if !errors.Is(m.err, saveErr) {
		t.Errorf("a successful poll must not clear an unrelated error, got %v", m.err)
	}
}

// TestReloadFailureDistinctShown has the poll fail with a different error: it
// replaces the one shown.
func TestReloadFailureDistinctShown(t *testing.T) {
	m, path := newTestModel(t, "# Work\n\n- [ ] a\n")
	good := readList(t, path)
	m = failPolls(t, m, path, good, 4, 3)
	m.err = errors.New("save failed: boom")
	m = failPolls(t, m, path, good, 5, 1)
	if got := m.footer(200); !strings.Contains(got, "schema 5") {
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

func TestReloadIgnoresOwnFingerprint(t *testing.T) {
	m, _ := newTestModel(t, "# Work\n\n- [ ] a\n")
	m = send(m, storeReloadedMsg{doc: parseDoc(t, "# Work\n\n- [ ] zzz\n"), fp: m.lastFP})
	if find(m.doc, "zzz") != nil {
		t.Errorf("a reload at the fingerprint the model already has must be ignored")
	}
}

// TestProbeRacingOwnWriteIsIgnored has a probe load an outside write, then the
// app commit its own write before the probe's message arrives: the write found
// the outside change and adopted both, so the late message, carrying the file
// as the write found it, must not roll the view back.
func TestProbeRacingOwnWriteIsIgnored(t *testing.T) {
	m, path := newTestModelOn(t, "# Work\n\n- [/] a\n- [/] b\n", todo.ColInProgress)
	title := "b renamed"
	if _, err := openSecond(t, path).Edit(context.Background(), find(m.doc, "b").ID, todo.Changes{Title: &title}); err != nil {
		t.Fatalf("external edit: %v", err)
	}
	late := reloadCheck(m.store, m.lastFP) // the probe started before the commit
	if _, ok := late.(storeReloadedMsg); !ok {
		t.Fatalf("precondition: the probe must load the outside write, got %#v", late)
	}

	m = pickState(t, selectTitle(t, m, "a"), todo.Done)
	if m.err != nil {
		t.Fatalf("the write failed: %v", m.err)
	}
	wrote := m.lastFP
	m = send(m, late)
	if m.lastFP != wrote {
		t.Errorf("lastFP = %x, want the own write's %x", m.lastFP, wrote)
	}
	if a := find(m.doc, "a"); a == nil || a.Status != todo.Done {
		t.Errorf("the late probe rolled the own write back: a = %v", a)
	}
	if find(m.doc, title) == nil {
		t.Errorf("the outside rename must stay")
	}
	if msg := reloadCheck(m.store, m.lastFP); msg != nil {
		t.Errorf("the view must match the file, got %#v", msg)
	}
}

// TestRevertToPreWriteContentIsReloaded has the app write, then something
// outside put back the exact bytes the write replaced (a git checkout, an undo
// in another tool): a probe started after the write must apply it, although
// its fingerprint is the one the app's write found.
func TestRevertToPreWriteContentIsReloaded(t *testing.T) {
	m, path := newTestModelOn(t, "# Work\n\n- [/] a\n", todo.ColInProgress)
	before, orig := m.lastFP, readList(t, path)
	m = pickState(t, selectTitle(t, m, "a"), todo.Done)
	if m.err != nil || m.lastFP == before {
		t.Fatalf("precondition: the write failed: %v", m.err)
	}
	writeList(t, path, orig)

	m = pollOnce(t, m)
	if m.lastFP != before {
		t.Errorf("lastFP = %x, want the reverted file's %x", m.lastFP, before)
	}
	if a := find(m.doc, "a"); a == nil || a.Status != todo.Working {
		t.Errorf("the revert must be shown: a = %v", a)
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
	externalAddTask(t, path, "new", find(m.doc, "A").ID, "")
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
// between polls: the app's next own write must notice that the file it found
// is not the one it last loaded, treat its result as external and drop the
// undo snapshots, so un-toggling the parent never writes old statuses over it.
func TestOwnWriteDetectsExternalWriteBeforePoll(t *testing.T) {
	m, path := newTestModelOn(t, "# Work\n\n- [/] parent\n  - [ ] c1\n  - [ ] c2\n- [ ] other\n", todo.ColInProgress)
	m = pickState(t, press(m, "down"), todo.Done) // cascade-complete parent
	if len(m.snapshots) != 1 {
		t.Fatalf("precondition: the cascade must leave a snapshot, got %d", len(m.snapshots))
	}

	// An agent reopens c1 through a second handle; no poll runs.
	c1 := find(m.doc, "c1").ID
	if _, err := openSecond(t, path).SetStatuses(context.Background(), map[string]todo.Status{c1: todo.Open}); err != nil {
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

// --- failed writes ---

func TestWriteErrorShowsAndReloads(t *testing.T) {
	m, path := newTestModelOn(t, "# Work\n\n- [/] a\n", todo.ColInProgress)
	writeList(t, path, []byte(halfWritten))
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

// TestFailedWriteAndReloadRestoresTree deletes an item while the file is
// invalid: the write and the reload both fail, so the view goes back to the
// document as last loaded — the item is still there, its rows rebuilt — and
// the selection stays valid.
func TestFailedWriteAndReloadRestoresTree(t *testing.T) {
	m, path := newTestModel(t, "# Work\n\n- [ ] a\n- [ ] b\n")
	m = press(m, "down", "down") // b
	writeList(t, path, []byte(halfWritten))
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

// TestFailedWriteAndReloadRestoresEdit edits a task while the file is
// invalid: the edited title must not stay on screen as if saved, and a second
// failed write must still restore the same, last loaded document.
func TestFailedWriteAndReloadRestoresEdit(t *testing.T) {
	m, path := newTestModel(t, "# Work\n\n- [ ] a\n")
	writeList(t, path, []byte(halfWritten))
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

// TestSavedWriteAdoptsWriteResult breaks the file right after a write: commit
// takes the list from the write itself, so it neither reads the file again
// nor reports an error, and the view shows the write's result.
func TestSavedWriteAdoptsWriteResult(t *testing.T) {
	m, path := newTestModel(t, "# Work\n\n- [ ] a\n- [ ] b\n")
	a, b := find(m.doc, "a"), find(m.doc, "b")
	m.doc.Remove(b)
	var wrote todo.Fingerprint
	saved := m.commit(a.ID, func(ctx context.Context) (todo.WriteResult, error) {
		res, err := m.store.Delete(ctx, b.ID)
		writeList(t, path, []byte(halfWritten)) // a read after the write fails
		wrote = res.Wrote
		return res, err
	})
	if !saved || m.err != nil {
		t.Fatalf("the write must succeed without error: saved %v, err %v", saved, m.err)
	}
	if hasRow(m, "b") || find(m.doc, "b") != nil {
		t.Errorf("the view must show the write's result, rows %v", rowTitles(m.tree))
	}
	if got := m.tree.selected().Title; got != "a" {
		t.Errorf("selection = %q, want a", got)
	}
	if m.lastFP != wrote {
		t.Errorf("lastFP must be the fingerprint the write left")
	}
}

// TestFailedWriteRevertsToStore has the write fail while the reload works: the
// in-memory change is discarded and the view shows what the store holds.
func TestFailedWriteRevertsToStore(t *testing.T) {
	m, _ := newTestModel(t, "# Work\n\n- [ ] a\n- [ ] b\n")
	b := find(m.doc, "b")
	m.doc.Remove(b)
	m.commit(b.ID, func(context.Context) (todo.WriteResult, error) { return todo.WriteResult{}, errors.New("boom") })
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

// TestFailedWriteReloadsOutsideChange has a write fail after another process
// changed the list: the reload shows that change and drops the undo
// snapshots, as a poll would.
func TestFailedWriteReloadsOutsideChange(t *testing.T) {
	m, path := newTestModel(t, "# Work\n\n- [ ] a\n")
	a := find(m.doc, "a")
	m.snapshots[a.ID] = todo.DoneStates{a.ID: todo.Open}
	externalAddTask(t, path, "b", find(m.doc, "Work").ID, a.ID)
	m.commit(a.ID, func(context.Context) (todo.WriteResult, error) { return todo.WriteResult{}, errors.New("boom") })
	if find(m.doc, "b") == nil || !hasRow(m, "b") {
		t.Errorf("the reload must show the outside change, rows %v", rowTitles(m.tree))
	}
	if len(m.snapshots) != 0 {
		t.Errorf("an outside change must drop the snapshots, got %d", len(m.snapshots))
	}
	if msg := reloadCheck(m.store, m.lastFP); msg != nil {
		t.Errorf("the reload must record the file's fingerprint, got %#v", msg)
	}
}

func TestInitStartsPolling(t *testing.T) {
	m, _ := newTestModel(t, "# Work\n")
	if m.Init() == nil {
		t.Errorf("Init must start the change-poll loop")
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

// TestReplacedFileIsReloaded renames another list in over the file, as `git
// checkout` or a sync tool would: the next poll shows the new list as an
// external change, and the writes that follow go to the file at the path.
func TestReplacedFileIsReloaded(t *testing.T) {
	m, path := newTestModel(t, "# Work\n\n- [ ] a\n")
	m = press(selectTitle(t, m, "a"), "p")
	if err := os.Rename(todotest.Seed(t, "# Other\n\n- [ ] b\n"), path); err != nil {
		t.Fatal(err)
	}

	m = pollCmdOnce(m)
	if m.err != nil {
		t.Errorf("a replaced file is a change, not an error: %v", m.err)
	}
	if find(m.doc, "b") == nil || find(m.doc, "a") != nil || !hasRow(m, "b") {
		t.Fatalf("the view must show the new file's list, rows %v", rowTitles(m.tree))
	}
	if !strings.Contains(m.status, "changed externally") {
		t.Errorf("status = %q, want the external-change notice", m.status)
	}

	m = press(selectTitle(t, m, "b"), "p")
	if m.err != nil {
		t.Fatalf("a write after the replacement failed: %v", m.err)
	}
	if got := statusIn(t, path, "b"); got == todo.Open {
		t.Errorf("b = %v in the file at the path, want the status change written there", got)
	}
}

// vanishingList is a list whose file is there when the probe asks whether it
// changed and gone by the time the probe loads it.
type vanishingList struct{ fp todo.Fingerprint }

func (v vanishingList) Changed(context.Context, todo.Fingerprint) (bool, todo.Fingerprint, error) {
	return true, v.fp, nil
}

func (vanishingList) Load(context.Context) (*todo.Document, todo.Fingerprint, error) {
	return &todo.Document{}, todo.Fingerprint{}, nil
}

// TestFileRemovedMidPollKeepsView has the file vanish between the probe's
// change check and its load: the empty list a missing file reads as must not
// replace the view; the poll reports the file removed.
func TestFileRemovedMidPollKeepsView(t *testing.T) {
	m, _ := newTestModel(t, "# Work\n\n- [ ] a\n")
	rows := rowTitles(m.tree)
	msg := reloadCheck(vanishingList{fp: todo.Fingerprint{1}}, m.lastFP)
	if f, ok := msg.(reloadFailedMsg); !ok || !errors.Is(f.err, errListRemoved) {
		t.Fatalf("a file gone by the load must be reported as errListRemoved, got %#v", msg)
	}
	m = send(m, msg)
	if got := rowTitles(m.tree); !slices.Equal(got, rows) {
		t.Errorf("rows = %v, want %v", got, rows)
	}
}

// TestCommitWithoutStoreWriteReloads commits a change made in memory through a
// write that succeeds without a store write, before any write and after one:
// there is no write result to adopt, so the view shows the stored list.
func TestCommitWithoutStoreWriteReloads(t *testing.T) {
	for _, afterWrite := range []bool{false, true} {
		m, _ := newTestModel(t, "# Work\n\n- [ ] a\n- [ ] b\n")
		if afterWrite {
			m = press(selectTitle(t, m, "a"), "p")
			if m.err != nil {
				t.Fatalf("precondition: the write failed: %v", m.err)
			}
		}
		b := find(m.doc, "b")
		m.doc.Remove(b)
		if !m.commit(b.ID, func(context.Context) (todo.WriteResult, error) { return todo.WriteResult{}, nil }) {
			t.Fatalf("afterWrite=%v: the commit must report success", afterWrite)
		}
		if find(m.doc, "b") == nil || !hasRow(m, "b") {
			t.Errorf("afterWrite=%v: the in-memory change must give way to the stored list, rows %v",
				afterWrite, rowTitles(m.tree))
		}
		if m.err != nil {
			t.Errorf("afterWrite=%v: m.err = %v", afterWrite, m.err)
		}
	}
}

// TestSavedWriteResetsPollFails has two polls fail and the file then come
// back before the next poll: a save that succeeds proves the file readable,
// so a later transient failure counts from zero again and is not shown after
// a single tick.
func TestSavedWriteResetsPollFails(t *testing.T) {
	m, path := newTestModel(t, "# Work\n\n- [ ] a\n- [ ] b\n")
	good := readList(t, path)
	writeList(t, path, []byte(halfWritten))
	m = pollCmdOnce(pollCmdOnce(m))
	if m.pollFails != 2 || m.err != nil {
		t.Fatalf("precondition: pollFails %d, err %v", m.pollFails, m.err)
	}
	writeList(t, path, good)

	m = press(selectTitle(t, m, "a"), "p")
	if m.err != nil {
		t.Fatalf("the save failed: %v", m.err)
	}
	if m.pollFails != 0 {
		t.Errorf("a successful save must reset the poll failures, got %d", m.pollFails)
	}
	writeList(t, path, []byte(halfWritten))
	m = pollCmdOnce(m)
	if m.err != nil {
		t.Errorf("one failed poll after the save must not show yet, got %v", m.err)
	}
}
