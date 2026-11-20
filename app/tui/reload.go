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

// probeMsg is what one probe of the list file reports: msg is nil when the
// file is unchanged, else a storeReloadedMsg or a reloadFailedMsg. gen is the
// generation of the model's list when the probe started (see model.docGen):
// a probe that started before the model took another list reports on a file
// the model no longer shows, so it is ignored.
type probeMsg struct {
	gen uint64
	msg tea.Msg
}

// storeReloadedMsg carries a document freshly loaded from the store after its
// file changed, together with the fingerprint of the bytes it came from.
type storeReloadedMsg struct {
	doc *todo.Document
	fp  todo.Fingerprint
}

// reloadFailedMsg reports that the poll could not read the list: the file was
// removed, is not a valid list (half written, say), was written by a newer
// todo, or could not be read. The view never changes; the error goes to the
// footer once enough polls in a row failed (see handleReloadFailed).
type reloadFailedMsg struct{ err error }

// pollCmd schedules the next reload tick.
func pollCmd() tea.Cmd {
	return tea.Tick(pollInterval, func(time.Time) tea.Msg { return reloadTickMsg{} })
}

// startProbe starts a probe of the list file, unless one is in flight: then
// it returns nil, and the tick after its message starts the next one.
func (m *model) startProbe() tea.Cmd {
	if m.probing {
		return nil
	}
	m.probing = true
	return m.reloadCheckCmd()
}

// reloadCheckCmd probes the store off the UI goroutine and reports what it
// found as a probeMsg. The probe only reads (Changed and Load); every write
// runs on the UI goroutine.
func (m model) reloadCheckCmd() tea.Cmd {
	st, last, gen := m.store, m.lastFP, m.docGen
	return func() tea.Msg { return probeMsg{gen: gen, msg: reloadCheck(st, last)} }
}

// handleProbe takes a probe's report. A stale one, from a probe that started
// before the model took its current list, is ignored: the next tick probes
// again from that list and finds anything newer. Otherwise an unchanged file
// ends a run of failed polls (it was put back as the model last loaded it),
// and a reload or a failure is handled as such.
func (m model) handleProbe(p probeMsg) (tea.Model, tea.Cmd) {
	m.probing = false
	if p.gen != m.docGen {
		return m, nil
	}
	switch msg := p.msg.(type) {
	case storeReloadedMsg:
		return m.handleReload(msg)
	case reloadFailedMsg:
		return m.handleReloadFailed(msg)
	}
	m.clearPollErr()
	return m, nil
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
	return storeReloadedMsg{doc: doc, fp: fp}
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

// handleReload applies an external change, found by a probe that started
// from the list the model shows (see handleProbe). A message at the
// fingerprint the model already has is ignored: nothing changed since the
// model took the list. While a modal is open or an item is grabbed for moving
// the reload is deferred, so an edit is never yanked out from under the user;
// the next tick re-detects it once they return to plain navigation. The undo
// snapshots are dropped at once all the same: the list changed under them,
// even if someone changes it back before the reload applies. The search bar
// is not a modal: a reload applies while it is open.
func (m model) handleReload(msg storeReloadedMsg) (tea.Model, tea.Cmd) {
	m.clearPollErr()
	if msg.fp == m.lastFP {
		return m, nil
	}
	if !m.canApplyReload() {
		m.snapshots = map[string]todo.DoneStates{}
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
// The reload notice goes first, so a status the adopt sets (Planning left for
// a gone sprint) wins over it.
func (m *model) applyReload(msg storeReloadedMsg) {
	onPH := m.focused().onPlaceholder()
	var focus string
	if sel := m.focused().selected(); sel != nil && !onPH {
		focus = sel.ID
	}
	m.take(msg.doc, msg.fp)
	m.noteExternal()
	m.adoptAll(msg.doc, focus, onPH)
}

// leaveGoneSprint moves the Sprints workspace from a board whose sprint the
// list no longer holds (it was deleted) to the first open sprint, or to none,
// with the sidebar focused on it. A workspace that shows no sprint takes the
// first open one when one appears (unless the unified view is over it). Under
// the unified view only the hidden sprint falls back: the view stays on. The
// view is not stored: the list that changed is no write of this session. A
// cursor on the Closed group row stays there, beside its pane, whatever sprint
// the workspace falls back to.
func (m *model) leaveGoneSprint() {
	if m.view.screen != screenSprint {
		return
	}
	if _, ok := m.doc.SprintByID(m.view.sprint); ok {
		m.cursorOnShown()
		return
	}
	if m.view.sprint == "" && m.view.all {
		return
	}
	id := m.defaultSprint()
	switch {
	case id == m.view.sprint:
		if id == "" {
			// Still nothing open to show: the cursor rests on the Closed
			// group row, with the fold as the user left it.
			m.selectRow(groupKey)
			return
		}
	case m.view.all:
		m.setShownSprint(id)
	default:
		m.switchScreen(screenSprint, id)
		m.listFocus = true
	}
	if m.onGroupRow() {
		m.selectRow(groupKey)
		return
	}
	m.selectInList(id)
}

// cursorOnShown keeps the sidebar and the board agreeing after a change of the
// list: the cursor stays on the shown sprint, unfolding Closed when it was
// closed elsewhere, unless it is on the Closed group row, whose pane lists the
// closed sprints while the stored view keeps the last sprint shown (and which,
// when the group is gone, goes to the shown sprint).
func (m *model) cursorOnShown() {
	if m.onGroupRow() {
		return
	}
	m.selectInList(m.view.sprint)
}

// onGroupRow reports whether the sidebar's cursor was on the Closed group row
// and the list still has that row, before the cursor is synced to the list.
func (m *model) onGroupRow() bool { return m.sprints.selID == groupKey && doneSprints(m.doc) > 0 }

// noteExternal drops the undo snapshots and says so in the footer, after the
// model took a list that someone else changed.
func (m *model) noteExternal() {
	m.snapshots = map[string]todo.DoneStates{}
	m.status = "reloaded — list changed externally"
}
