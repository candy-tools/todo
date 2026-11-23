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
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// sprintList is the Sprints workspace's sidebar: the sprints in two sections,
// each opened by its rule as a board's sections are (see sprintSection), the
// cursor over the sprints and the first entry shown. The rows, the sprints,
// are derived from the list on every use; the rules are drawn between them and
// never hold the cursor. The cursor follows the sprint it is on (selID) when a
// write or reload reorders the rows, and falls back to its index, clamped,
// when that sprint is gone.
type sprintList struct {
	cursor int    // the index in sidebarRows of the sprint under the cursor
	selID  string // that sprint's ID, "" before the first use
	offset int    // the index of the first entry shown (see sidebarEntries)
}

// sprintLabel is how a sprint is named on screen: its reference, then its name
// made safe to draw ("TODO-001 Sprint 13"), or the name alone when it has no
// reference. Names may repeat; the reference tells such sprints apart.
func sprintLabel(s *todo.Sprint) string {
	if s.Ref == "" {
		return todo.Printable(s.Name)
	}
	return s.Ref + " " + todo.Printable(s.Name)
}

// maxNameWidth caps a sprint's name wherever it is shown beside other text, so
// a long one never pushes the rest off the row.
const maxNameWidth = 30

// sprintSection is a section of the Sprints sidebar: one per sprint state
// (see todo.SprintStatus), in the order the states progress.
type sprintSection int

const (
	sectionOpen sprintSection = iota
	sectionReady
	sectionInProgress
	sectionDone
)

// sprintSections lists the sidebar's sections in the order it shows them.
var sprintSections = []sprintSection{sectionOpen, sectionReady, sectionInProgress, sectionDone}

// sectionFor is the sidebar section of sprint state st.
func sectionFor(st todo.SprintStatus) sprintSection {
	switch st {
	case todo.SprintReady:
		return sectionReady
	case todo.SprintInProgress:
		return sectionInProgress
	case todo.SprintDone:
		return sectionDone
	}
	return sectionOpen
}

// status is the sprint state the section lists.
func (s sprintSection) status() todo.SprintStatus {
	switch s {
	case sectionReady:
		return todo.SprintReady
	case sectionInProgress:
		return todo.SprintInProgress
	case sectionDone:
		return todo.SprintDone
	}
	return todo.SprintOpen
}

// sectionOf is the sidebar section sprint s of d is listed in: the one of its
// state, which its tasks decide.
func sectionOf(d *todo.Document, s *todo.Sprint) sprintSection {
	return sectionFor(d.SprintStatus(s.ID))
}

// title is the section's name, on its rule and in the footer.
func (s sprintSection) title() string { return s.status().Title() }

// style is the colour of the section's rule.
func (s sprintSection) style() lipgloss.Style { return sprintStatusStyle(s.status()) }

// sprintStatusGlyph returns the glyph and colour of sprint state st, borrowed
// from the task states it stands for: open ◌ (as New), ready ◉, in progress
// ◐, done ✓.
func sprintStatusGlyph(st todo.SprintStatus) (string, lipgloss.Style) {
	switch st {
	case todo.SprintReady:
		return "◉", nextStyle
	case todo.SprintInProgress:
		return "◐", progStyle
	case todo.SprintDone:
		return "✓", doneStyle
	}
	return "◌", nextStyle
}

// sprintStatusStyle is the colour of sprint state st's section rule: plain
// for Open, then the board's Next cyan, In Progress orange and Done green.
func sprintStatusStyle(st todo.SprintStatus) lipgloss.Style {
	switch st {
	case todo.SprintReady:
		return nextStyle
	case todo.SprintInProgress:
		return progStyle
	case todo.SprintDone:
		return doneStyle
	}
	return plainStyle
}

// step returns the section delta sections away from s, wrapping around the
// sidebar in either direction, as stepColumn does on a board.
func (s sprintSection) step(delta int) sprintSection {
	n := len(sprintSections)
	return sprintSections[((int(s)+delta)%n+n)%n]
}

// sidebarRows lists the sprints of d as the sidebar shows them, section by
// section (see sprintSections): each in creation order, but the done ones
// newest first.
func sidebarRows(d *todo.Document) []*todo.Sprint {
	by := make(map[sprintSection][]*todo.Sprint, len(sprintSections))
	for _, s := range d.Sprints {
		sec := sectionOf(d, s)
		by[sec] = append(by[sec], s)
	}
	slices.Reverse(by[sectionDone])
	var rows []*todo.Sprint
	for _, sec := range sprintSections {
		rows = append(rows, by[sec]...)
	}
	return rows
}

// sidebarEntry is one entry the sidebar draws: a section's rule, or the
// sprint at index row of its rows.
type sidebarEntry struct {
	section sprintSection
	count   int // on a rule: how many sprints its section holds
	row     int // -1 on a rule
}

// rule reports whether the entry is a section's rule.
func (e sidebarEntry) rule() bool { return e.row < 0 }

// rowLines is how many lines a sprint's row of the sidebar takes; a rule takes
// one.
const rowLines = 2

// linesOf is how many lines the entries es take.
func linesOf(es []sidebarEntry) (n int) {
	for _, e := range es {
		n++
		if !e.rule() {
			n += rowLines - 1
		}
	}
	return n
}

// sidebarEntries lays rows (see sidebarRows) out under their sections' rules:
// each section's rule, then its sprints. An empty section shows as its rule
// alone, as a board's does.
func sidebarEntries(d *todo.Document, rows []*todo.Sprint) []sidebarEntry {
	es := make([]sidebarEntry, 0, len(rows)+len(sprintSections))
	i := 0
	for _, sec := range sprintSections {
		at := len(es)
		es = append(es, sidebarEntry{section: sec, row: -1})
		for ; i < len(rows) && sectionOf(d, rows[i]) == sec; i++ {
			es = append(es, sidebarEntry{section: sec, row: i})
		}
		es[at].count = len(es) - at - 1
	}
	return es
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

// move shifts the cursor by delta sprints, clamped to the list: the rules
// between the sections take no step.
func (l *sprintList) move(d *todo.Document, delta int) {
	rows := sidebarRows(d)
	l.sync(rows)
	l.cursor += delta
	l.clamp(len(rows))
	if len(rows) > 0 {
		l.selID = rows[l.cursor].ID
	}
}

// selected returns the sprint under the cursor, or nil when d has none.
func (l *sprintList) selected(d *todo.Document) *todo.Sprint {
	rows := sidebarRows(d)
	l.sync(rows)
	if len(rows) == 0 {
		return nil
	}
	return rows[l.cursor]
}

// section is the section of the sprint under the cursor, Open when there is
// none.
func (l *sprintList) section(d *todo.Document) sprintSection {
	if s := l.selected(d); s != nil {
		return sectionOf(d, s)
	}
	return sectionOpen
}

// selectID puts the cursor on the sprint with the given ID, when d has it.
func (l *sprintList) selectID(d *todo.Document, id string) {
	if i := slices.IndexFunc(sidebarRows(d), func(s *todo.Sprint) bool { return s.ID == id }); i >= 0 {
		l.cursor, l.selID = i, id
	}
}

// showSection puts the cursor on the first sprint of section sec, with the
// window starting at the section's rule as far as the entries below fill
// height lines, as a board's showSectionAt does. It reports false, and moves
// nothing, when sec holds no sprint.
func (l *sprintList) showSection(d *todo.Document, sec sprintSection, height int) bool {
	rows := sidebarRows(d)
	i := slices.IndexFunc(rows, func(s *todo.Sprint) bool { return sectionOf(d, s) == sec })
	if i < 0 {
		return false
	}
	l.cursor, l.selID = i, rows[i].ID
	l.offset = slices.IndexFunc(sidebarEntries(d, rows), func(e sidebarEntry) bool { return e.row == i }) - 1 // the rule
	l.reconcile(d, height)
	return true
}

// reconcile follows the cursor's sprint through a change of d and keeps its
// row whole inside height lines, as the tree's reconcileOffset does.
func (l *sprintList) reconcile(d *todo.Document, height int) { l.layout(d, height) }

// layout syncs the cursor to d and moves the window so the cursor's row shows
// whole in height lines (see sidebarWindow). It returns the rows, their
// entries and the index of the cursor's entry (-1 when d has no sprint).
func (l *sprintList) layout(d *todo.Document, height int) (rows []*todo.Sprint, es []sidebarEntry, cur int) {
	rows = sidebarRows(d)
	l.sync(rows)
	es = sidebarEntries(d, rows)
	cur = slices.IndexFunc(es, func(e sidebarEntry) bool { return e.row == l.cursor })
	if cur < 0 {
		l.offset = 0
		return rows, es, cur
	}
	l.offset = sidebarWindow(es, l.offset, cur, height)
	return rows, es, cur
}

// sidebarWindow returns the index of the first of the entries es to show in
// height lines with the cursor's row on entry cur: offset, unless that leaves
// the cursor's row out, whole or in part, else the nearest one holding it, so
// the window only moves when the cursor leaves it. The window never starts so
// far down that it ends short of the pane's last line, and the rules right
// above it come into view while the cursor's row still fits, as the tree's
// headings do.
func sidebarWindow(es []sidebarEntry, offset, cur, height int) int {
	off := min(offset, lastStart(es, height), cur)
	for off < cur && linesOf(es[off:cur+1]) > height {
		off++
	}
	for off > 0 && es[off-1].rule() && linesOf(es[off-1:cur+1]) <= height {
		off--
	}
	return off
}

// lastStart is the last of the entries es a window height lines tall can
// start at and still reach its last line, 0 when es fits whole.
func lastStart(es []sidebarEntry, height int) int {
	for i := len(es) - 1; i > 0; i-- {
		if linesOf(es[i:]) >= height {
			return i
		}
	}
	return 0
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

// view renders the sidebar of d, width x height lines, from the first entry
// shown (see layout): the sections' rules (see sectionRule) and the sprints'
// rows (see sidebarRowLines), the cursor's marked and highlighted and always whole,
// even in a pane too short for it. A list without sprints renders a dim line
// saying so.
func (l *sprintList) view(d *todo.Document, width, height int) string {
	rows, es, cur := l.layout(d, height)
	if len(rows) == 0 {
		return "  " + helpTextStyle.Render("no sprints yet")
	}
	keep := max(height, linesOf(es[l.offset:cur+1]))
	lines := make([]string, 0, keep+1)
	for _, e := range es[l.offset:] {
		if len(lines) >= keep {
			break
		}
		if e.rule() {
			lines = append(lines, sectionRule(e.section.title(), e.count, e.section.style(), width))
			continue
		}
		first, second := sidebarRowLines(d, rows[e.row], width, e.row == l.cursor)
		lines = append(lines, first, second)
	}
	return strings.Join(lines[:min(len(lines), keep)], "\n")
}

// sidebarRowLines renders the two lines of sprint s's row in the sidebar, width
// cells wide: its state's glyph (see sprintStatusGlyph), its label (see sprintLabel) and its top-level task
// counts "Next·In Progress·Done" right-aligned, then its goal ("—" without
// one). Names and goals are cut with "…" to the width: nothing wraps.
func sidebarRowLines(d *todo.Document, s *todo.Sprint, width int, cur bool) (first, second string) {
	marker := "  "
	if cur {
		marker = "❯ "
	}
	st := d.SprintStatus(s.ID)
	glyph, glyphStyle := sprintStatusGlyph(st)
	nameStyle := categoryStyle
	if st == todo.SprintDone {
		nameStyle = helpTextStyle
	}
	name, goal := sprintLabel(s), todo.Printable(s.Goal)
	if goal == "" {
		goal = "—"
	}
	c := d.ColumnCounts(s.ID)
	counts := fmt.Sprintf("%d·%d·%d", c[todo.ColNext], c[todo.ColInProgress], c[todo.ColDone])
	const lead = 4 // marker, glyph, space
	name = ansi.Truncate(name, max(width-lead-1-ansi.StringWidth(counts), 1), "…")
	gap := strings.Repeat(" ", max(width-lead-ansi.StringWidth(name)-ansi.StringWidth(counts), 1))
	goal = ansi.Truncate(goal, max(width-lead, 1), "…")
	if cur {
		first = selectedRowStyle.Render(marker + glyph + " " + name + gap + counts)
		second = selectedRowStyle.Render("    " + goal)
	} else {
		first = marker + glyphStyle.Render(glyph) + " " + nameStyle.Render(name) + gap + helpTextStyle.Render(counts)
		second = "    " + helpTextStyle.Render(goal)
	}
	return ansi.Truncate(first, width, ""), ansi.Truncate(second, width, "")
}

// sidebarSectionKey maps a key of the Sprints sidebar to the section it jumps
// to: ] and [ step from current, the section of the cursor's sprint, 1 to 4
// name Open, Ready, In Progress and Done. dir is the way ] (1) or [ (-1)
// steps, 0 for a digit; ok is false for any other key.
func sidebarSectionKey(key string, current sprintSection) (sec sprintSection, dir int, ok bool) {
	switch key {
	case "]":
		return current.step(1), 1, true
	case "[":
		return current.step(-1), -1, true
	}
	if len(key) == 1 && key[0] >= '1' && int(key[0]-'1') < len(sprintSections) {
		return sprintSections[key[0]-'1'], 0, true
	}
	return 0, 0, false
}

// jumpSprintSection moves the sidebar's cursor onto the first sprint of
// section sec (see sidebarSectionKey) and shows its board, as jumpSection does
// on a board: a step skips an empty section, wrapping around, and a digit on
// an empty one says so in the footer.
func (m *model) jumpSprintSection(sec sprintSection, dir int) {
	for range sprintSections {
		if m.sprints.showSection(m.doc, sec, m.sprintListHeight()) {
			m.followList()
			return
		}
		if dir == 0 {
			m.status = "nothing in " + sec.title()
			return
		}
		sec = sec.step(dir)
	}
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

// openDeleteSprint opens the confirm dialog for the selected sprint (focused on
// the safe Cancel button). A sprint with tasks not done is refused at once, in
// the footer ("can't delete: 3 tasks not done"): only a done or empty one may
// be deleted. A no-op on an empty list.
func (m model) openDeleteSprint() (tea.Model, tea.Cmd) {
	s := m.sprints.selected(m.doc)
	if s == nil {
		return m, nil
	}
	if n := m.doc.Unfinished(s.ID); n > 0 {
		m.status = "can't delete: " + (&todo.SprintNotDoneError{Remaining: n}).Error()
		return m, nil
	}
	m.deletingSprint = s
	m.confirmOnYes = false
	m.confirmAction = confirmDeleteSprint
	m.mode = modeConfirm
	return m, nil
}

// doDeleteSprint deletes the confirmed sprint and all its tasks. A sprint
// with a task not done since the dialog opened (another writer reopened one)
// is refused, and the footer says so rather than reporting a save error.
func (m model) doDeleteSprint() (tea.Model, tea.Cmd) {
	s := m.deletingSprint
	m.deletingSprint = nil
	m.mode = modeMain
	if s == nil {
		return m, nil
	}
	// The sprint's tasks go with it, which ends the accidental-complete undo
	// window.
	m.snapshots = map[string]todo.DoneStates{}
	id := s.ID
	next := m.rowAfterDelete(id)
	m.writeSprint(func(ctx context.Context, st *todo.Store) (todo.WriteResult, error) {
		res, err := st.DeleteSprint(ctx, id)
		if errors.Is(err, todo.ErrSprintNotDone) {
			err = fmt.Errorf("can't delete: %w", err) // a refusal, shown as the status
		}
		return res, err
	})
	if _, ok := m.doc.SprintByID(id); !ok && next != "" {
		m.selectInList(next)
		m.followList()
	}
	return m, nil
}

// rowAfterDelete is the ID of the sprint the sidebar falls back to when sprint
// id is deleted: the one that takes its index, clamped to the last one. ""
// when no other sprint is left.
func (m *model) rowAfterDelete(id string) string {
	rows := sidebarRows(m.doc)
	at := max(slices.IndexFunc(rows, func(s *todo.Sprint) bool { return s.ID == id }), 0)
	rest := slices.DeleteFunc(rows, func(s *todo.Sprint) bool { return s.ID == id })
	if len(rest) == 0 {
		return ""
	}
	return rest[min(at, len(rest)-1)].ID
}
