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

// sprintList is the Sprints workspace's sidebar: the open sprints, then one
// Closed group row that folds the done ones, the cursor over them and the first
// row shown. The rows are derived from the list on every use. The cursor follows
// the row it is on (selID) when a write or reload reorders the rows, and falls
// back to its index, clamped, when that row is gone.
type sprintList struct {
	cursor int
	selID  string // the key of the row the cursor is on, "" before the first use
	offset int    // the index of the first row shown
	// closedOpen is whether the Closed group shows the done sprints.
	closedOpen bool
}

// sprintListRow is one row of the sidebar: a sprint, or the Closed group.
type sprintListRow struct {
	sprint *todo.Sprint // nil on the group row
	group  bool
}

// maxNameWidth caps a sprint's name wherever it is shown beside other text, so
// a long one never pushes the rest off the row.
const maxNameWidth = 30

// groupKey is the selID of the Closed group row; sprint IDs never look like it.
const groupKey = "·closed"

// key identifies the row across reloads.
func (r sprintListRow) key() string {
	if r.group {
		return groupKey
	}
	return r.sprint.ID
}

// rows lists the rows of d as the screen shows them: the open sprints in
// creation order; then, when d has done sprints, the Closed group row; then,
// only while the group is unfolded, the done sprints, newest first.
func (l *sprintList) rows(d *todo.Document) []sprintListRow {
	var rows []sprintListRow
	var done []*todo.Sprint
	for _, s := range d.Sprints {
		if s.Status == todo.SprintDone {
			done = append(done, s)
		} else {
			rows = append(rows, sprintListRow{sprint: s})
		}
	}
	if len(done) == 0 {
		return rows
	}
	rows = append(rows, sprintListRow{group: true})
	if l.closedOpen {
		slices.Reverse(done)
		for _, s := range done {
			rows = append(rows, sprintListRow{sprint: s})
		}
	}
	return rows
}

// clamp keeps the cursor on one of n rows (on 0 when there are none).
func (l *sprintList) clamp(n int) {
	l.cursor = max(min(l.cursor, n-1), 0)
}

// sync puts the cursor on the row it was on, wherever it is in rows now; when
// that row is gone it keeps the index, clamped.
func (l *sprintList) sync(rows []sprintListRow) {
	if i := slices.IndexFunc(rows, func(r sprintListRow) bool { return r.key() == l.selID }); i >= 0 {
		l.cursor = i
		return
	}
	l.clamp(len(rows))
	l.selID = ""
	if len(rows) > 0 {
		l.selID = rows[l.cursor].key()
	}
}

// move shifts the cursor by delta rows, clamped to the list.
func (l *sprintList) move(d *todo.Document, delta int) {
	rows := l.rows(d)
	l.sync(rows)
	l.cursor += delta
	l.clamp(len(rows))
	if len(rows) > 0 {
		l.selID = rows[l.cursor].key()
	}
}

// selected returns the sprint under the cursor, or nil when d has none or the
// cursor is on the group row.
func (l *sprintList) selected(d *todo.Document) *todo.Sprint {
	rows := l.rows(d)
	l.sync(rows)
	if len(rows) == 0 {
		return nil
	}
	return rows[l.cursor].sprint
}

// onGroup reports whether the cursor is on the Closed group row.
func (l *sprintList) onGroup(d *todo.Document) bool {
	rows := l.rows(d)
	l.sync(rows)
	return len(rows) > 0 && rows[l.cursor].group
}

// selectID puts the cursor on the sprint with the given ID, when d has it,
// unfolding the Closed group for a done one.
func (l *sprintList) selectID(d *todo.Document, id string) {
	if s, ok := d.SprintByID(id); ok && s.Status == todo.SprintDone {
		l.closedOpen = true
	}
	l.selectKey(d, id)
}

// selectGroup puts the cursor on the Closed group row, when d has one.
func (l *sprintList) selectGroup(d *todo.Document) { l.selectKey(d, groupKey) }

func (l *sprintList) selectKey(d *todo.Document, key string) {
	if i := slices.IndexFunc(l.rows(d), func(r sprintListRow) bool { return r.key() == key }); i >= 0 {
		l.cursor, l.selID = i, key
	}
}

// window returns the index of the first of height rows to show out of n, the
// current one unless the cursor is outside it: the window only moves when the
// cursor leaves it.
func (l *sprintList) window(n, height int) int {
	return scrollWindow(l.offset, l.cursor, n, height)
}

// scrollWindow returns the index of the first of height rows to show out of n
// with the cursor on row cursor: offset (clamped) unless the cursor is outside
// that window, else the nearest one holding it.
func scrollWindow(offset, cursor, n, height int) int {
	off := min(offset, max(n-height, 0))
	switch {
	case cursor < off:
		off = cursor
	case cursor >= off+height:
		off = cursor - height + 1
	}
	return max(off, 0)
}

// rowLines is how many lines every row of the sidebar takes.
const rowLines = 2

// fitRows is how many rows fit in height lines (at least one).
func fitRows(height int) int { return max(height/rowLines, 1) }

// reconcile follows the cursor's row through a change of d and keeps it inside
// the rows that fit height lines, as the tree's reconcileOffset does.
func (l *sprintList) reconcile(d *todo.Document, height int) {
	rows := l.rows(d)
	l.sync(rows)
	l.offset = l.window(len(rows), fitRows(height))
}

// view renders the rows of d that fit width x height lines, the cursor's
// always among them (marked and highlighted). Every row is two lines. A sprint
// shows a status glyph, its name and its top-level task counts "Next·In
// Progress·Done" right-aligned, then its goal; the group row shows a fold
// glyph, "Closed" and the number of done sprints. Names and goals are cut with
// "…" to the width: nothing wraps. A list without sprints renders a dim line
// saying so.
func (l *sprintList) view(d *todo.Document, width, height int) string {
	rows := l.rows(d)
	if len(rows) == 0 {
		return "  " + helpTextStyle.Render("no sprints yet")
	}
	l.sync(rows)
	l.offset = l.window(len(rows), fitRows(height))
	end := min(l.offset+fitRows(height), len(rows))
	lines := make([]string, 0, rowLines*(end-l.offset))
	for i := l.offset; i < end; i++ {
		first, second := l.rowLines(d, rows[i], width, i == l.cursor)
		lines = append(lines, first, second)
	}
	return strings.Join(lines, "\n")
}

// doneSprints counts the done sprints of d.
func doneSprints(d *todo.Document) (n int) {
	for _, s := range d.Sprints {
		if s.Status == todo.SprintDone {
			n++
		}
	}
	return n
}

// rowLines renders the two lines of row r, width cells wide.
func (l *sprintList) rowLines(d *todo.Document, r sprintListRow, width int, cur bool) (first, second string) {
	marker := "  "
	if cur {
		marker = "❯ "
	}
	var glyph, name, counts, goal, indent string
	var glyphStyle, nameStyle = progStyle, categoryStyle
	switch {
	case r.group:
		glyph, name, nameStyle, glyphStyle = "▸", "Closed", helpTextStyle, helpTextStyle
		if l.closedOpen {
			glyph = "▾"
		}
		counts = fmt.Sprint(doneSprints(d))
	default:
		s := r.sprint
		glyph, name, goal = "●", todo.Printable(s.Name), todo.Printable(s.Goal)
		if s.Status == todo.SprintDone {
			glyph, glyphStyle, nameStyle = "✓", doneStyle, helpTextStyle
			indent = "  " // nested under the Closed row
		}
		c := d.ColumnCounts(s.ID)
		counts = fmt.Sprintf("%d·%d·%d", c[todo.ColNext], c[todo.ColInProgress], c[todo.ColDone])
	}
	// indent, marker, glyph, space, name, gap, counts
	lead := 4 + len(indent)
	name = ansi.Truncate(name, max(width-lead-1-ansi.StringWidth(counts), 1), "…")
	gap := strings.Repeat(" ", max(width-lead-ansi.StringWidth(name)-ansi.StringWidth(counts), 1))
	goal = ansi.Truncate(goal, max(width-lead, 1), "…")
	if cur {
		first = selectedRowStyle.Render(indent + marker + glyph + " " + name + gap + counts)
		second = selectedRowStyle.Render(indent + "    " + goal)
	} else {
		first = indent + marker + glyphStyle.Render(glyph) + " " + nameStyle.Render(name) + gap + helpTextStyle.Render(counts)
		second = indent + "    " + helpTextStyle.Render(goal)
	}
	return ansi.Truncate(first, width, ""), ansi.Truncate(second, width, "")
}

// moveSprints moves the sidebar's cursor by delta rows, keeps it shown, and
// shows its sprint's board.
func (m *model) moveSprints(delta int) {
	m.sprints.move(m.doc, delta)
	m.sprints.reconcile(m.doc, m.sprintListHeight())
	m.followList()
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

// writeSprint runs one store write of the sidebar through commit, then keeps
// the cursor on its sprint, wherever the write moved it, and the board on the
// cursor's sprint.
func (m *model) writeSprint(write storeWrite) (saved bool) {
	saved = m.commit("", write)
	m.sprints.reconcile(m.doc, m.sprintListHeight())
	m.followList()
	return saved
}

// submitSprint stores the open sprint form, for submitForm: it adds the
// sprint, puts the cursor on it and shows its board, or writes the fields of
// the edited sprint
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
		m.selectInList(id)
		m.followList()
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
	// A sprint that is about to be closed must stay in the rows, so the Closed
	// group unfolds first (and folds back if the store refuses).
	was := m.sprints.closedOpen
	m.sprints.closedOpen = m.sprints.closedOpen || closing
	if !m.writeSprint(func(ctx context.Context, st *todo.Store) (todo.WriteResult, error) {
		if !closing {
			return st.ReopenSprint(ctx, id)
		}
		res, err := st.CloseSprint(ctx, id)
		if errors.Is(err, todo.ErrSprintNotDone) {
			err = fmt.Errorf("can't close: %w", err) // a refusal, shown as the status
		}
		return res, err
	}) {
		m.sprints.closedOpen = was
		m.sprints.reconcile(m.doc, m.sprintListHeight())
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
	next := m.rowAfterDelete(id)
	m.writeSprint(func(ctx context.Context, st *todo.Store) (todo.WriteResult, error) {
		return st.DeleteSprint(ctx, id)
	})
	if _, ok := m.doc.SprintByID(id); !ok && next != "" {
		m.selectRow(next)
		m.followList()
	}
	return m, nil
}

// rowAfterDelete is the key of the row the sidebar falls back to when sprint
// id is deleted: the row that takes its index, clamped to the last one (the
// Closed group row goes with the last done sprint). "" when no other row is
// left.
func (m *model) rowAfterDelete(id string) string {
	var rest []string
	at := 0
	lastDone := false
	if s, ok := m.doc.SprintByID(id); ok {
		lastDone = s.Status == todo.SprintDone && doneSprints(m.doc) == 1
	}
	for i, r := range m.sprints.rows(m.doc) {
		switch {
		case r.key() == id:
			at = i
		case r.group && lastDone:
		default:
			rest = append(rest, r.key())
		}
	}
	if len(rest) == 0 {
		return ""
	}
	return rest[min(at, len(rest)-1)]
}
