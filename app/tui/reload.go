package tui

import (
	"context"
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
// footer once per distinct failure; the current view stays.
type reloadFailedMsg struct{ err error }

// reloadRecoveredMsg reports that a poll succeeded after one failed, so the
// failure can be taken out of the footer.
type reloadRecoveredMsg struct{}

// pollCmd schedules the next reload tick.
func pollCmd() tea.Cmd {
	return tea.Tick(pollInterval, func(time.Time) tea.Msg { return reloadTickMsg{} })
}

// reloadCheckCmd probes the store off the UI goroutine. While a poll failure
// is shown, a probe that succeeds without a change reports the recovery. While
// the model is unsynced the probe loads the list even at lastRev.
func (m model) reloadCheckCmd() tea.Cmd {
	st, last, failing := m.store, m.lastRev, m.pollErr != nil
	if m.unsynced {
		last = -1 // matches no revision, so the list is loaded
	}
	return func() tea.Msg {
		msg := reloadCheck(st, last)
		if msg == nil && failing {
			return reloadRecoveredMsg{}
		}
		return msg
	}
}

// reloadCheck reads the store's revision and, when it differs from last, loads
// the list and returns it as a storeReloadedMsg. It returns nil when nothing
// changed, and a reloadFailedMsg on any error, which never wipes the current
// view. The file is checked first: when it was removed or replaced, the open
// handle still reads the old copy, so its revision says nothing about the file
// at the path.
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
	if (m.mode != modeMain && m.mode != modeSearch) || m.focused().grabbed {
		return m, nil
	}
	m.applyReload(msg)
	return m, nil
}

// applyReload swaps in the reloaded document, restoring the selection and the
// folds by ID. The in-memory undo snapshots are dropped: the list changed under
// us, and an undo must never write old statuses over someone else's change.
// A reload at lastRev (only applied while unsynced) is the app's own write read
// back: the snapshots stay and no notice is shown.
func (m *model) applyReload(msg storeReloadedMsg) {
	onPH := m.focused().onPlaceholder()
	var focus uint
	if sel := m.focused().selected(); sel != nil && !onPH {
		focus = sel.ID
	}
	own := msg.rev == m.lastRev
	m.doc, m.saved, m.lastRev, m.unsynced = msg.doc, msg.doc.Clone(), msg.rev, false
	m.adoptAll(msg.doc, focus, onPH)
	if own {
		return
	}
	m.snapshots = map[uint]todo.DoneStates{}
	m.status = "reloaded — list changed externally"
}
