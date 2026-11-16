package tui

import (
	"context"
	"errors"
	"fmt"

	"github.com/candy-tools/todo/internal/todo"
)

// commitStatus commits write, a status change of top-level task id, like
// commit. When the change took the task out of the tab, the cursor keeps its
// row index (clamped), now on the row that took the task's place, and the
// footer says where the task went, e.g. "moved to Next — new". A failed write
// or an external-reload notice takes the footer instead.
func (m *model) commitStatus(id string, write storeWrite) {
	m.commit(id, write)
	m.noteLeftTab(id)
}

// noteLeftTab reports whether item id, after a commit, is no longer shown in
// the tab (moved to another column, or gone). When a task left for another
// column, the footer says where it went, e.g. "moved to Next — new" (a subtask
// goes with its task, so its task's column and state are named), unless a save
// error or the external-reload notice already holds the footer.
func (m *model) noteLeftTab(id string) bool {
	it, ok := m.doc.ByID(id)
	if ok && m.focused().shown[it] {
		return false
	}
	if ok && it.IsTask() && m.err == nil && m.status == "" {
		task := it
		if it.IsSubtask() {
			task = it.Parent
		}
		// In the split view's task list a task can leave by moving to a
		// category outside the one picked in the sidebar — still in view, just
		// out of scope: name that category.
		if t := m.focused(); t.pane == paneTasks && t.visibleItems()[it] {
			if c := task.EnclosingCategory(); c != nil {
				m.status = "moved to " + todo.Printable(c.Title)
				return true
			}
		}
		m.status = fmt.Sprintf("moved to %s — %s", task.Status.Column().Title(), task.Status.Label())
	}
	return true
}

// storeWrite is one store write as commit runs it, on st, the store commit
// hands it (see writer): it returns the write's result.
type storeWrite func(ctx context.Context, st *todo.Store) (todo.WriteResult, error)

// writer returns the store every write goes through. Once the model has read
// the list from a file, a write requires that file to still be there
// (Store.Existing), so a list removed since is never silently recreated; it
// fails with todo.ErrListMissing and writes nothing. A model that started on
// a missing file writes through the plain store, so its first write creates
// the file.
func (m *model) writer() *todo.Store {
	if m.lastFP.IsMissing() {
		return m.store
	}
	return m.store.Existing()
}

// commit runs write, one store write, and shows its result, so the view always
// shows what the store holds. A write error is kept for the footer (a success
// clears the last one). Either way the trees are rebuilt, keeping the folds of
// the items that still exist, the viewport, the filter and the grab state, and
// selecting focusID if present (else keeping the cursor index, clamped).
//
// On success the list is the one the write left (its WriteResult), read
// under the write's lock, so the file is not read again; its fingerprint is
// recorded, so the poll does not report the app's own write as an external
// change. The undo snapshots are kept — unless the write found the file other
// than the model last loaded it: someone else wrote in between, so the result
// is handled as external (snapshots dropped, the status line says so).
//
// On failure the list is loaded again, and handled the same way when it moved;
// when that fails too, the view keeps the list it shows, which no handler
// changes before its write. A write that finds the file the model read
// removed fails as errListRemoved and writes nothing (see writer); the view
// keeps the list.
//
// commit reports whether the write succeeded, so a caller can keep the user's
// input to retry a failed save.
func (m *model) commit(focusID string, write storeWrite) (saved bool) {
	ctx := context.Background()
	m.err = nil
	onPH := m.focused().onPlaceholder()
	r, err := write(ctx, m.writer())
	if err != nil {
		if errors.Is(err, todo.ErrListMissing) {
			err = errListRemoved
		}
		m.err = fmt.Errorf("save failed: %w", err)
		m.reload(ctx)
		m.adoptAll(m.doc, focusID, onPH)
		return false
	}
	external := r.Read != m.lastFP
	m.take(r.Doc, r.Wrote)
	// The write read and validated the file, so earlier poll failures are
	// over: a later one counts from zero.
	m.clearPollErr()
	m.adoptAll(m.doc, focusID, onPH)
	if external {
		m.noteExternal()
	}
	return true
}

// take makes doc, read from the file at fingerprint fp, the list the model
// shows, and starts a new generation of it: a probe started before reports on
// a list the model no longer shows (see handleProbe), even when fp is the
// fingerprint it started from (a change and its undo leave the same bytes).
func (m *model) take(doc *todo.Document, fp todo.Fingerprint) {
	m.doc, m.lastFP = doc, fp
	m.docGen++
}

// reload loads the list for commit after a failed write, which left no
// result to adopt, taking it as an external change when its file moved since
// the last load. When the load fails, or finds the file removed (which the
// poll reports), m.doc stays the list the model shows.
func (m *model) reload(ctx context.Context) {
	doc, fp, err := m.store.Load(ctx)
	if err != nil || (fp.IsMissing() && !m.lastFP.IsMissing()) {
		return
	}
	external := fp != m.lastFP
	m.take(doc, fp)
	if external {
		m.noteExternal()
	}
}
