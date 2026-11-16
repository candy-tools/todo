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

// commit runs write against the store and then reloads, so the view always
// shows what the store holds. A write error is kept for the footer (a success
// clears the last one). The reload happens either way: it records the
// revision this write produced (so the poll does not report it as an external
// change) and rebuilds the tree, keeping the folds of the items that still
// exist, the viewport, the filter and the grab state, and selecting focusID if
// present (else keeping the cursor index, clamped). The undo snapshots are
// kept, since this is the app's own write — unless the reload shows that
// someone else wrote too: a successful store write bumps the revision exactly
// once and a failed one rolls back, so any other revision than that means an
// external write landed since the last load, and the reload is handled as
// external (snapshots dropped, the status line says so). If the reload itself
// fails, the error is shown (the write's when it failed, else the reload's) and
// the trees are rebuilt with focusID selected, from the current document when
// the write succeeded (the caller's in-memory change is what the store now
// holds; its revision is recorded and the next poll loads it as the app's
// own, see unsynced), else from a copy of the document as last loaded, so the change the
// caller made in memory does not stay on screen as if it were saved.
//
// commit reports whether the write succeeded, whatever the reload did, so a
// caller can keep the user's input to retry a failed save.
func (m *model) commit(focusID string, write func(ctx context.Context) error) (saved bool) {
	ctx := context.Background()
	m.err = nil
	expected := m.lastRev
	if err := write(ctx); err != nil {
		m.err = fmt.Errorf("save failed: %w", err)
	} else {
		expected++
		saved = true
	}
	onPH := m.focused().onPlaceholder()
	doc, rev, err := m.store.Load(ctx)
	if err != nil {
		if m.err == nil {
			m.err = fmt.Errorf("reload failed: %w", err)
			m.pollErr = m.err // cleared by the next reload that succeeds (see clearPollErr)
			m.saved = m.doc.Clone()
			m.lastRev, m.unsynced = expected, true
		} else {
			m.doc = m.saved.Clone()
		}
		m.adoptAll(m.doc, focusID, onPH)
		return saved
	}
	m.doc, m.saved, m.lastRev, m.unsynced = doc, doc.Clone(), rev, false
	m.adoptAll(doc, focusID, onPH)
	if rev != expected {
		m.snapshots = map[string]todo.DoneStates{}
		m.status = "reloaded — list changed externally"
	}
	return saved
}
