package tui

import (
	"context"
	"fmt"

	"github.com/candy-tools/todo/internal/todo"
)

// commitStatus commits write, a status change of top-level task id, like
// commit. When the change took the task out of the tab, the cursor keeps its
// row index (clamped), now on the row that took the task's place, and the
// footer says where the task went, e.g. "moved to Next — new". A failed write
// or an external-reload notice takes the footer instead.
func (m *model) commitStatus(id string, write func(ctx context.Context) error) {
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

// commit runs write, one store write, and shows its result, so the view always
// shows what the store holds. A write error is kept for the footer (a success
// clears the last one). Either way the trees are rebuilt, keeping the folds of
// the items that still exist, the viewport, the filter and the grab state, and
// selecting focusID if present (else keeping the cursor index, clamped).
//
// On success the list is the one the write left (the store's LastWrite), read
// under the write's lock, so the file is not read again; its fingerprint is
// recorded, so the poll does not report the app's own write as an external
// change. The undo snapshots are kept — unless the write found the file other
// than the model last loaded it: someone else wrote in between, so the result
// is handled as external (snapshots dropped, the status line says so).
//
// On failure the list is loaded again, and handled the same way when it moved;
// when that fails too, a copy of the document as last loaded is put back, so
// the change the caller made in memory does not stay on screen as if saved. A
// write that found the file removed and wrote nothing (clearing the done
// tasks of the empty list a missing file reads as) is such a failure too.
//
// commit reports whether the write succeeded, so a caller can keep the user's
// input to retry a failed save. write must run on the UI goroutine: LastWrite
// is only this write's result while no other goroutine writes through the
// store.
func (m *model) commit(focusID string, write func(ctx context.Context) error) (saved bool) {
	ctx := context.Background()
	m.err = nil
	onPH := m.focused().onPlaceholder()
	before := m.store.LastWrite()
	if err := write(ctx); err != nil {
		m.err = fmt.Errorf("save failed: %w", err)
		m.reload(ctx)
		m.adoptAll(m.doc, focusID, onPH)
		return false
	}
	r := m.store.LastWrite()
	if r.Doc == nil || r.Doc == before.Doc {
		// write succeeded without a store write: there is no result to
		// adopt, so read the list as it is.
		m.reload(ctx)
		m.adoptAll(m.doc, focusID, onPH)
		return true
	}
	if r.Read == todo.Missing && r.Wrote == todo.Missing && m.lastFP != todo.Missing {
		// The file is gone and the write, finding nothing to change in the
		// empty list that reads as, wrote nothing: adopting that empty list
		// would wipe the view the removed-file error keeps. Fail as removed.
		m.err = fmt.Errorf("save failed: %w", errListRemoved)
		m.doc = m.saved.Clone()
		m.adoptAll(m.doc, focusID, onPH)
		return false
	}
	external := r.Read != m.lastFP
	// m.doc aliases r.Doc, the store's LastWrite().Doc, and the handlers
	// change m.doc in place; that is safe only while nothing else reads
	// LastWrite().Doc (commit compares it by pointer only).
	m.doc, m.saved, m.lastFP = r.Doc, r.Doc.Clone(), r.Wrote
	// The write read and validated the file, so earlier poll failures are
	// over: a later one counts from zero.
	m.clearPollErr()
	m.adoptAll(m.doc, focusID, onPH)
	if external {
		m.noteExternal()
	}
	return true
}

// reload loads the list for commit when the write left no result to adopt
// (it failed), taking it as an external change when its file moved since the
// last load. When the load fails,
// or finds the file removed (which the poll reports, keeping the view), m.doc
// goes back to a copy of the list as last loaded.
func (m *model) reload(ctx context.Context) {
	doc, fp, err := m.store.Load(ctx)
	if err != nil || (fp == todo.Missing && m.lastFP != todo.Missing) {
		m.doc = m.saved.Clone()
		return
	}
	external := fp != m.lastFP
	m.doc, m.saved, m.lastFP = doc, doc.Clone(), fp
	if external {
		m.noteExternal()
	}
}
