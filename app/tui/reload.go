package tui

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/candy-tools/todo/internal/todo"
	tea "github.com/charmbracelet/bubbletea"
)

// pollInterval is how often todo checks the list file to pick up writes made
// outside the app (by a CLI command, or an agent). It is short enough to feel
// live, and the check only stats the file, hashing it when the stat moved.
const pollInterval = time.Second

// pollFailsShown is how many polls in a row must fail before the failure is
// shown. A failure or two is usually transient — another tool caught writing
// the file in place — and the next poll reads it whole.
const pollFailsShown = 3

// errListRemoved is the poll failure for a list file that is gone. The view
// keeps the list, and nothing is created at the path until a write.
var errListRemoved = errors.New("list file was removed")

// reloadTickMsg fires on every poll interval; handling it kicks off a change
// check and re-arms the next tick.
type reloadTickMsg struct{}

// storeReloadedMsg carries a document freshly loaded from the store after its
// file changed, together with the fingerprint of the bytes it came from (used
// to tell the app's own writes apart from external ones) and the fingerprint
// the probe compared against (the model's lastFP when the probe started).
type storeReloadedMsg struct {
	doc   *todo.Document
	fp    todo.Fingerprint
	since todo.Fingerprint
}

// reloadFailedMsg reports that the poll could not read the list: the file was
// removed, is not a valid list (half written, say), was written by a newer
// todo, or could not be read. The view never changes; the error goes to the
// footer once enough polls in a row failed (see handleReloadFailed).
type reloadFailedMsg struct{ err error }

// reloadRecoveredMsg reports that a poll found the file unchanged after polls
// that failed — it was put back as the model last loaded it — so the failure
// count is reset and the failure taken out of the footer.
type reloadRecoveredMsg struct{}

// pollCmd schedules the next reload tick.
func pollCmd() tea.Cmd {
	return tea.Tick(pollInterval, func(time.Time) tea.Msg { return reloadTickMsg{} })
}

// reloadCheckCmd probes the store off the UI goroutine. The probe only reads
// (Changed and Load); every write runs on the UI goroutine. While polls are
// failing, a probe that finds the file unchanged reports the recovery.
func (m model) reloadCheckCmd() tea.Cmd {
	st, last, failing := m.store, m.lastFP, m.pollFails > 0
	return func() tea.Msg {
		msg := reloadCheck(st, last)
		if msg == nil && failing {
			return reloadRecoveredMsg{}
		}
		return msg
	}
}

// listReader is the read side of the store the poll uses (todo.Store).
type listReader interface {
	Changed(ctx context.Context, since todo.Fingerprint) (bool, todo.Fingerprint, error)
	Load(ctx context.Context) (*todo.Document, todo.Fingerprint, error)
}

// reloadCheck asks the store whether the file changed since last and, when it
// did, loads the list and returns it as a storeReloadedMsg. It returns nil
// when nothing changed, and a reloadFailedMsg on any error, or when the file
// is gone (errListRemoved) — also when it vanished between the check and the
// load, which reads as an empty list — so the current view is never wiped.
func reloadCheck(st listReader, last todo.Fingerprint) tea.Msg {
	ctx := context.Background()
	changed, now, err := st.Changed(ctx, last)
	if err != nil {
		return reloadFailedMsg{err: err}
	}
	if !changed {
		return nil
	}
	if now.IsMissing() {
		return reloadFailedMsg{err: errListRemoved}
	}
	doc, fp, err := st.Load(ctx)
	if err != nil {
		return reloadFailedMsg{err: err}
	}
	if fp.IsMissing() {
		return reloadFailedMsg{err: errListRemoved}
	}
	return storeReloadedMsg{doc: doc, fp: fp, since: last}
}

// handleReloadFailed counts a failed poll. Once pollFailsShown polls in a row
// failed, the failure is shown in the footer, once: the same failure on the
// next ticks is not set again, so it does not override an error shown since (a
// failed save), while a different one replaces what is shown. The view never
// changes.
func (m model) handleReloadFailed(msg reloadFailedMsg) (tea.Model, tea.Cmd) {
	m.pollFails++
	if m.pollFails < pollFailsShown {
		return m, nil
	}
	err := fmt.Errorf("reload failed: %w", msg.err)
	if m.pollErr != nil && m.pollErr.Error() == err.Error() {
		return m, nil
	}
	m.pollErr, m.err = err, err
	return m, nil
}

// clearPollErr forgets the poll failures after a poll succeeded, taking the
// one shown out of the footer unless another error has replaced it there.
func (m *model) clearPollErr() {
	m.pollFails = 0
	if m.pollErr == nil {
		return
	}
	if m.err == m.pollErr {
		m.err = nil
	}
	m.pollErr = nil
}

// handleReload applies an external change. A message at the fingerprint the
// model already has is ignored: nothing changed since the model took the list.
// So is one from a probe that raced one of the app's own writes: started
// before the commit (its since is no longer lastFP), it carries the file as
// that write found it (ownRead), which the write's result already includes;
// applying it would roll the view back until the next poll. A probe started
// after the write is applied even at ownRead, as the file was put back to
// those bytes since (a git checkout, say). While a modal is open or an item is
// grabbed for moving the reload is deferred, so an edit is never yanked out
// from under the user; the next tick re-detects it once they return to plain
// navigation. The search bar is not a modal: a reload applies while it is
// open.
func (m model) handleReload(msg storeReloadedMsg) (tea.Model, tea.Cmd) {
	m.clearPollErr()
	if msg.fp == m.lastFP {
		return m, nil
	}
	if msg.fp == m.ownRead && msg.since != m.lastFP {
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
func (m *model) applyReload(msg storeReloadedMsg) {
	onPH := m.focused().onPlaceholder()
	var focus string
	if sel := m.focused().selected(); sel != nil && !onPH {
		focus = sel.ID
	}
	m.doc, m.lastFP = msg.doc, msg.fp
	m.adoptAll(msg.doc, focus, onPH)
	m.noteExternal()
}

// noteExternal drops the undo snapshots and says so in the footer, after the
// model took a list that someone else changed.
func (m *model) noteExternal() {
	m.snapshots = map[string]todo.DoneStates{}
	m.status = "reloaded — list changed externally"
}
