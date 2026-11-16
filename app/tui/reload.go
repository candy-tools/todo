package tui

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/candy-tools/todo/internal/todo"
	tea "github.com/charmbracelet/bubbletea"
)

// pollInterval is how often todo checks the store's revision to pick up writes
// made outside the app (by a CLI command, or an agent). It is short enough to
// feel live, and the check is a single-row read.
const pollInterval = time.Second

// reloadTickMsg fires on every poll interval; handling it kicks off a revision
// check and re-arms the next tick.
type reloadTickMsg struct{}

// storeReloadedMsg carries a document freshly loaded from the store after its
// revision moved, together with the revision that load reflects (used to tell
// the app's own writes apart from external ones).
type storeReloadedMsg struct {
	doc *todo.Document
	rev int64
}

// reloadFailedMsg reports that the poll could not read the store: its file was
// removed or replaced (writes are refused, see todo.ErrFileReplaced), a newer
// todo upgraded its schema, or it became unreadable. The error goes to the
// footer once per distinct failure; the current view stays. A replaced file is
// reopened instead when a file is back at the path (see reopenStore).
type reloadFailedMsg struct{ err error }

// storeReopenedMsg carries the store reopened at the path after the file the
// model's store (old) had open was replaced, with the document and revision
// loaded from it.
type storeReopenedMsg struct {
	old, st *todo.Store
	doc     *todo.Document
	rev     int64
}

// reloadRecoveredMsg reports that a poll succeeded after one failed, so the
// failure can be taken out of the footer.
type reloadRecoveredMsg struct{}

// pollCmd schedules the next reload tick.
func pollCmd() tea.Cmd {
	return tea.Tick(pollInterval, func(time.Time) tea.Msg { return reloadTickMsg{} })
}

// reloadCheckCmd probes the store off the UI goroutine. While a poll failure
// is shown, a probe that succeeds without a change reports the recovery. While
// the model is unsynced the probe loads the list even at lastRev. When the
// store file was replaced and a reload could be applied now, the probe reopens
// the store at the path (see reopenStore).
func (m model) reloadCheckCmd() tea.Cmd {
	st, last, failing := m.store, m.lastRev, m.pollErr != nil
	if m.unsynced {
		last = -1 // matches no revision, so the list is loaded
	}
	var reopen func() (*todo.Store, error)
	if m.canApplyReload() {
		reopen = m.opener
	}
	return func() tea.Msg {
		msg := reloadCheck(st, last)
		if f, ok := msg.(reloadFailedMsg); ok && reopen != nil && errors.Is(f.err, todo.ErrFileReplaced) {
			return reopenStore(st, reopen, f)
		}
		if msg == nil && failing {
			return reloadRecoveredMsg{}
		}
		return msg
	}
}

// reloadCheck reads the store's revision and, when it differs from last, loads
// the list and returns it as a storeReloadedMsg. It returns nil when nothing
// changed, and a reloadFailedMsg on any error, which never wipes the current
// view. The file is checked first, so a removed or replaced file is reported
// before any read; Revision and Load also fail with todo.ErrFileReplaced once
// that happens (their reads may have run on the file now at the path), so a
// replacement during the check is reported too.
func reloadCheck(st *todo.Store, last int64) tea.Msg {
	if err := st.CheckFile(); err != nil {
		return reloadFailedMsg{err: err}
	}
	ctx := context.Background()
	rev, err := st.Revision(ctx)
	if err != nil {
		return reloadFailedMsg{err: err}
	}
	if rev == last {
		return nil
	}
	doc, rev, err := st.Load(ctx)
	if err != nil {
		return reloadFailedMsg{err: err}
	}
	return storeReloadedMsg{doc: doc, rev: rev}
}

// reopenStore reopens the store at old's path after its file was replaced, and
// loads the list from it. A path with no file stays failed: nothing is created
// there, and a later poll reopens the file once one is back. The old store is
// closed first, which empties its WAL at the path (see todo.Store.Close), so
// the new file does not read the old one's pages. Any failure is returned as a
// reloadFailedMsg (failed, when the file is gone).
func reopenStore(old *todo.Store, reopen func() (*todo.Store, error), failed reloadFailedMsg) tea.Msg {
	if ok, err := todo.StoreExists(old.Path()); err != nil || !ok {
		return failed
	}
	_ = old.Close()
	if ok, err := todo.StoreExists(old.Path()); err != nil || !ok {
		return failed
	}
	st, err := reopen()
	if err != nil {
		return reloadFailedMsg{err: err}
	}
	doc, rev, err := st.Load(context.Background())
	if err != nil {
		_ = st.Close()
		return reloadFailedMsg{err: err}
	}
	return storeReopenedMsg{old: old, st: st, doc: doc, rev: rev}
}

// handleReopen swaps in the store reopened after a file replacement and
// applies its list as an external change, clearing the error. It is dropped,
// and the new store closed, when the model's store changed since the probe (a
// probe that raced another reopen) or a reload cannot be applied now: a dialog
// or a grab works on the old list's IDs, which name other items in the new
// file. The next poll reopens it again.
func (m model) handleReopen(msg storeReopenedMsg) (tea.Model, tea.Cmd) {
	if msg.old != m.store || !m.canApplyReload() {
		_ = msg.st.Close()
		return m, nil
	}
	m.store = msg.st
	m.err, m.pollErr = nil, nil
	m.lastRev, m.unsynced = -1, false // matches no revision: the list is external
	m.applyReload(storeReloadedMsg{doc: msg.doc, rev: msg.rev})
	return m, nil
}

// handleReloadFailed shows a poll failure in the footer, once: the same
// failure on the next ticks is not set again, so it does not override an error
// shown since (a failed save), while a different one replaces what is shown.
func (m model) handleReloadFailed(msg reloadFailedMsg) (tea.Model, tea.Cmd) {
	err := fmt.Errorf("reload failed: %w", msg.err)
	if m.pollErr != nil && m.pollErr.Error() == err.Error() {
		return m, nil
	}
	m.pollErr, m.err = err, err
	return m, nil
}

// clearPollErr forgets the poll failure after a poll succeeded, taking it out
// of the footer unless another error has replaced it there.
func (m *model) clearPollErr() {
	if m.pollErr == nil {
		return
	}
	if m.err == m.pollErr {
		m.err = nil
	}
	m.pollErr = nil
}

// handleReload applies an external change. A message at or behind the revision
// the model already has is ignored: it is the app's own write, or a probe that
// raced one (applying it would roll the view back past that write) — except at
// lastRev while unsynced, when it is the list the app's own write left. While a
// modal is open or an item is grabbed for moving the reload is deferred, so an
// edit is never yanked out from under the user; the next tick re-detects it
// once they return to plain navigation. The search bar is not a modal: a
// reload applies while it is open.
func (m model) handleReload(msg storeReloadedMsg) (tea.Model, tea.Cmd) {
	m.clearPollErr()
	if msg.rev < m.lastRev || (msg.rev == m.lastRev && !m.unsynced) {
		return m, nil
	}
	if !m.canApplyReload() {
		return m, nil
	}
	m.applyReload(msg)
	return m, nil
}

// canApplyReload reports whether an external reload can be applied now: in
// plain navigation or the search bar, with no item grabbed.
func (m model) canApplyReload() bool {
	return (m.mode == modeMain || m.mode == modeSearch) && !m.focused().grabbed
}

// applyReload swaps in the reloaded document, restoring the selection and the
// folds by ID. The in-memory undo snapshots are dropped: the list changed under
// us, and an undo must never write old statuses over someone else's change.
// A reload at lastRev (only applied while unsynced) is the app's own write read
// back: the snapshots stay and no notice is shown.
func (m *model) applyReload(msg storeReloadedMsg) {
	onPH := m.focused().onPlaceholder()
	var focus string
	if sel := m.focused().selected(); sel != nil && !onPH {
		focus = sel.ID
	}
	own := msg.rev == m.lastRev
	m.doc, m.saved, m.lastRev, m.unsynced = msg.doc, msg.doc.Clone(), msg.rev, false
	m.adoptAll(msg.doc, focus, onPH)
	if own {
		return
	}
	m.snapshots = map[string]todo.DoneStates{}
	m.status = "reloaded — list changed externally"
}
