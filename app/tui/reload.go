package tui

import (
	"context"
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

// pollCmd schedules the next reload tick.
func pollCmd() tea.Cmd {
	return tea.Tick(pollInterval, func(time.Time) tea.Msg { return reloadTickMsg{} })
}

// reloadCheckCmd probes the store off the UI goroutine.
func (m model) reloadCheckCmd() tea.Cmd {
	st, last := m.store, m.lastRev
	return func() tea.Msg { return reloadCheck(st, last) }
}

// reloadCheck reads the store's revision and, when it differs from last, loads
// the list and returns it as a storeReloadedMsg. It returns nil when nothing
// changed or on any read error — a transient failure must never wipe the
// current view.
func reloadCheck(st *todo.Store, last int64) tea.Msg {
	ctx := context.Background()
	rev, err := st.Revision(ctx)
	if err != nil || rev == last {
		return nil
	}
	doc, rev, err := st.Load(ctx)
	if err != nil {
		return nil
	}
	return storeReloadedMsg{doc: doc, rev: rev}
}

// handleReload applies an external change. A message at or behind the revision
// the model already has is ignored: it is the app's own write, or a probe that
// raced one (applying it would roll the view back past that write). While a
// modal is open or an item is grabbed for moving the reload is deferred, so an
// edit is never yanked out from under the user; the next tick re-detects it
// once they return to plain navigation.
func (m model) handleReload(msg storeReloadedMsg) (tea.Model, tea.Cmd) {
	if msg.rev <= m.lastRev {
		return m, nil
	}
	if m.mode != modeMain || m.tree.grabbed {
		return m, nil
	}
	m.applyReload(msg)
	return m, nil
}

// applyReload swaps in the reloaded document, restoring the selection and the
// folds by ID. The in-memory undo snapshots are dropped: the list changed under
// us, and an undo must never write old statuses over someone else's change.
func (m *model) applyReload(msg storeReloadedMsg) {
	onPH := m.tree.onPlaceholder()
	var focus uint
	if sel := m.tree.selected(); sel != nil && !onPH {
		focus = sel.ID
	}
	m.doc, m.lastRev = msg.doc, msg.rev
	m.snapshots = map[uint]todo.DoneStates{}
	m.tree.adopt(msg.doc, focus, onPH)
	m.status = "reloaded — list changed externally"
}
