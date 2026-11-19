package tui

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/candy-tools/todo/internal/todo"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

// sprintList is the Sprint list screen: one row per sprint, the cursor over
// them and the first row shown. The rows are derived from the list on every
// use. The cursor follows the sprint it is on (selID) when a write or reload
// reorders the rows, and falls back to its index, clamped, when that sprint is
// gone.
type sprintList struct {
	cursor int
	selID  string // the ID of the sprint the cursor is on, "" before the first use
	offset int    // the index of the first row shown
}

// maxGoalWidth and maxNameWidth cap the goal and name columns, so a long one
// never pushes the counts off the row.
const (
	maxGoalWidth = 40
	maxNameWidth = 30
)

// rows lists the sprints of d as the screen shows them: the open ones in
// creation order, then the done ones, newest first.
func (l *sprintList) rows(d *todo.Document) []*todo.Sprint {
	var open, done []*todo.Sprint
	for _, s := range d.Sprints {
		if s.Status == todo.SprintDone {
			done = append(done, s)
		} else {
			open = append(open, s)
		}
	}
	slices.Reverse(done)
	return append(open, done...)
}

// clamp keeps the cursor on one of n rows (on 0 when there are none).
func (l *sprintList) clamp(n int) {
	l.cursor = max(min(l.cursor, n-1), 0)
}

// sync puts the cursor on the sprint it was on, wherever it is in rows now;
// when that sprint is gone it keeps the index, clamped.
func (l *sprintList) sync(rows []*todo.Sprint) {
	if i := slices.IndexFunc(rows, func(s *todo.Sprint) bool { return s.ID == l.selID }); i >= 0 {
		l.cursor = i
		return
	}
	l.clamp(len(rows))
	l.selID = ""
	if len(rows) > 0 {
		l.selID = rows[l.cursor].ID
	}
}

// move shifts the cursor by delta rows, clamped to the list.
func (l *sprintList) move(d *todo.Document, delta int) {
	rows := l.rows(d)
	l.sync(rows)
	l.cursor += delta
	l.clamp(len(rows))
	if len(rows) > 0 {
		l.selID = rows[l.cursor].ID
	}
}

// selected returns the sprint under the cursor, or nil when d has none.
func (l *sprintList) selected(d *todo.Document) *todo.Sprint {
	rows := l.rows(d)
	l.sync(rows)
	if len(rows) == 0 {
		return nil
	}
	return rows[l.cursor]
}

// selectID puts the cursor on the sprint with the given ID, when d has it.
func (l *sprintList) selectID(d *todo.Document, id string) {
	if i := slices.IndexFunc(l.rows(d), func(s *todo.Sprint) bool { return s.ID == id }); i >= 0 {
		l.cursor, l.selID = i, id
	}
}

// window returns the index of the first of height rows to show out of n, the
// current one unless the cursor is outside it: the window only moves when the
// cursor leaves it.
func (l *sprintList) window(n, height int) int {
	off := min(l.offset, max(n-height, 0))
	switch {
	case l.cursor < off:
		off = l.cursor
	case l.cursor >= off+height:
		off = l.cursor - height + 1
	}
	return max(off, 0)
}

// reconcile follows the cursor's sprint through a change of d and keeps it
// inside the height rows shown, as the tree's reconcileOffset does.
func (l *sprintList) reconcile(d *todo.Document, height int) {
	rows := l.rows(d)
	l.sync(rows)
	l.offset = l.window(len(rows), max(height, 1))
}

// view renders the rows of d that fit width x height, the cursor's always
// among them: per sprint a status glyph, the name, the status, the goal and
// its top-level task counts as "Next · In Progress · Done", in aligned
// columns. A list without sprints renders a dim line saying so.
func (l *sprintList) view(d *todo.Document, width, height int) string {
	rows := l.rows(d)
	if len(rows) == 0 {
		return "  " + helpTextStyle.Render("no sprints yet")
	}
	l.sync(rows)
	names, goals := make([]string, len(rows)), make([]string, len(rows))
	nameW, goalW := 0, 0
	for i, s := range rows {
		names[i] = ansi.Truncate(todo.Printable(s.Name), maxNameWidth, "…")
		goals[i] = ansi.Truncate(todo.Printable(s.Goal), maxGoalWidth, "…")
		nameW = max(nameW, ansi.StringWidth(names[i]))
		goalW = max(goalW, ansi.StringWidth(goals[i]))
	}
	height = max(height, 1)
	l.offset = l.window(len(rows), height)
	start := l.offset
	end := min(start+height, len(rows))
	lines := make([]string, 0, end-start)
	for i := start; i < end; i++ {
		s := rows[i]
		glyph, glyphStyle := "●", progStyle
		if s.Status == todo.SprintDone {
			glyph, glyphStyle = "✓", doneStyle
		}
		c := d.ColumnCounts(s.ID)
		counts := fmt.Sprintf("%d · %d · %d", c[todo.ColNext], c[todo.ColInProgress], c[todo.ColDone])
		name, status, goal := padRight(names[i], nameW), padRight(s.Status.String(), 4), padRight(goals[i], goalW)
		var line string
		if i == l.cursor {
			line = selectedRowStyle.Render("❯ " + glyph + " " + name + "   " + status + "   " + goal + "   " + counts)
		} else {
			line = "  " + glyphStyle.Render(glyph) + " " + categoryStyle.Render(name) + "   " +
				helpTextStyle.Render(status) + "   " + plainStyle.Render(goal) + "   " + helpTextStyle.Render(counts)
		}
		lines = append(lines, ansi.Truncate(line, width, ""))
	}
	return strings.Join(lines, "\n")
}

// padRight pads s with spaces to w terminal cells.
func padRight(s string, w int) string {
	return s + strings.Repeat(" ", max(w-ansi.StringWidth(s), 0))
}

// moveSprints moves the Sprint list's cursor by delta rows and keeps it shown.
func (m *model) moveSprints(delta int) {
	m.sprints.move(m.doc, delta)
	m.sprints.reconcile(m.doc, m.treeViewHeight())
}

// openSprintForm opens the sprint form: for edit, that sprint's name and goal,
// else a blank one to add a sprint.
func (m model) openSprintForm(edit *todo.Sprint) (tea.Model, tea.Cmd) {
	v, editing := todo.Fields{}, edit != nil
	m.target = targetAddSprint
	if editing {
		// Shown as the rows show them: a legacy value may hold control characters.
		v = todo.Fields{Title: todo.Printable(edit.Name), Description: todo.PrintableLines(edit.Goal)}
		m.target, m.editingSprint = targetEditSprint, edit.ID
	}
	m.form = newForm(v, formSprint, editing)
	m.form.setWidth(m.width)
	m.mode = modeForm
	return m, textinput.Blink
}

// writeSprint runs one store write of the Sprint list through commit, then
// keeps the cursor on its sprint, wherever the write moved it.
func (m *model) writeSprint(write storeWrite) (saved bool) {
	saved = m.commit("", write)
	m.sprints.reconcile(m.doc, m.treeViewHeight())
	return saved
}

// submitSprint stores the open sprint form, for submitForm: it adds the
// sprint and puts the cursor on it, or writes the fields of the edited sprint
// that changed. A failed write reopens the form with what was typed, unless
// the edited sprint is gone.
func (m *model) submitSprint() {
	v := m.form.values()
	if m.target == targetAddSprint {
		var id string
		if !m.writeSprint(func(ctx context.Context, st *todo.Store) (res todo.WriteResult, err error) {
			id, res, err = st.AddSprint(ctx, v.Title, v.Description)
			return res, err
		}) {
			m.mode = modeForm
			return
		}
		m.sprints.selectID(m.doc, id)
		m.sprints.reconcile(m.doc, m.treeViewHeight())
		return
	}
	ch := m.form.sprintChanges()
	if ch == (todo.SprintChanges{}) {
		return
	}
	id := m.editingSprint
	if !m.writeSprint(func(ctx context.Context, st *todo.Store) (todo.WriteResult, error) {
		return st.EditSprint(ctx, id, ch)
	}) {
		if _, ok := m.doc.SprintByID(id); ok {
			m.mode = modeForm
		}
	}
}

// toggleSprintClosed closes the selected sprint when it is open, else reopens
// it. A close the store refuses, as tasks are not done, says so in the footer
// ("can't close: 3 tasks not done") rather than as a save error.
func (m model) toggleSprintClosed() (tea.Model, tea.Cmd) {
	s := m.sprints.selected(m.doc)
	if s == nil {
		return m, nil
	}
	id, closing := s.ID, s.Status == todo.SprintOpen
	if !m.writeSprint(func(ctx context.Context, st *todo.Store) (todo.WriteResult, error) {
		if closing {
			return st.CloseSprint(ctx, id)
		}
		return st.ReopenSprint(ctx, id)
	}) {
		var notDone *todo.SprintNotDoneError
		if errors.As(m.err, &notDone) {
			m.err, m.status = nil, "can't close: "+notDone.Error()
		}
	}
	return m, nil
}

// openDeleteSprint opens the confirm dialog for the selected sprint (focused on
// the safe Cancel button). A no-op on an empty list.
func (m model) openDeleteSprint() (tea.Model, tea.Cmd) {
	s := m.sprints.selected(m.doc)
	if s == nil {
		return m, nil
	}
	m.deletingSprint = s
	m.confirmOnDelete = false
	m.confirmAction = confirmDeleteSprint
	m.mode = modeConfirm
	return m, nil
}

// doDeleteSprint deletes the confirmed sprint; its unfinished tasks return to
// the Backlog. A sprint holding done tasks is refused, and the footer says so
// rather than reporting a save error.
func (m model) doDeleteSprint() (tea.Model, tea.Cmd) {
	s := m.deletingSprint
	m.deletingSprint = nil
	m.mode = modeMain
	if s == nil {
		return m, nil
	}
	// Tasks change state when they return to the Backlog, which ends the
	// accidental-complete undo window.
	m.snapshots = map[string]todo.DoneStates{}
	id := s.ID
	if !m.writeSprint(func(ctx context.Context, st *todo.Store) (todo.WriteResult, error) {
		return st.DeleteSprint(ctx, id)
	}) && errors.Is(m.err, todo.ErrSprintHasDone) {
		m.err, m.status = nil, todo.ErrSprintHasDone.Error()
	}
	return m, nil
}
