package tui

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/candy-tools/todo/internal/todo"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// sprintPickPurpose is what choosing a row of the sprint picker does.
type sprintPickPurpose int

const (
	pickJump   sprintPickPurpose = iota // K: show the chosen sprint's board
	pickAssign                          // a: put a backlog task in the chosen sprint
	pickSendTo                          // o: move a sprint task to another sprint or the Backlog
)

// sprintPickRow is one choice of the sprint picker: a sprint, or (backlog set)
// the Backlog.
type sprintPickRow struct {
	sprintID string
	label    string
	backlog  bool
}

// sprintPicker is the dialog behind K, a and o: a titled list of sprints (and,
// for o, the Backlog) with a cursor. taskID is the task p and o act on.
type sprintPicker struct {
	title   string
	purpose sprintPickPurpose
	rows    []sprintPickRow
	cursor  int
	offset  int // the index of the first row shown
	taskID  string
}

// sprintRow is the picker row of sprint s, its label (see sprintLabel): done
// sprints carry a check mark.
func sprintRow(s *todo.Sprint) sprintPickRow {
	label := sprintLabel(s)
	if s.Status == todo.SprintDone {
		label = "✓ " + label
	}
	return sprintPickRow{sprintID: s.ID, label: label}
}

// newJumpPicker lists every sprint of d, the open ones first and then the done
// ones, for K.
func newJumpPicker(d *todo.Document) sprintPicker {
	p := sprintPicker{title: "Go to sprint", purpose: pickJump}
	for _, status := range []todo.SprintStatus{todo.SprintOpen, todo.SprintDone} {
		for _, s := range d.Sprints {
			if s.Status == status {
				p.rows = append(p.rows, sprintRow(s))
			}
		}
	}
	return p
}

// newAssignPicker lists the open sprints of d, for p on a backlog task.
func newAssignPicker(d *todo.Document, task *todo.Item) sprintPicker {
	p := sprintPicker{title: "Assign to sprint", purpose: pickAssign, taskID: task.ID}
	for _, s := range d.OpenSprints() {
		p.rows = append(p.rows, sprintRow(s))
	}
	return p
}

// newSendToPicker lists the Backlog, then the open sprints of d other than the
// one task is in, for o.
func newSendToPicker(d *todo.Document, task *todo.Item) sprintPicker {
	p := sprintPicker{title: "Send to", purpose: pickSendTo, taskID: task.ID}
	p.rows = append(p.rows, sprintPickRow{label: todo.ColBacklog.Title(), backlog: true})
	for _, s := range d.OpenSprints() {
		if s.ID != task.SprintID() {
			p.rows = append(p.rows, sprintRow(s))
		}
	}
	return p
}

// move steps the cursor by delta (±1), stopping at either end.
func (p *sprintPicker) move(delta int) {
	p.cursor = min(max(p.cursor+delta, 0), len(p.rows)-1)
}

// scroll keeps the cursor among the rows the dialog shows on a terminal
// termHeight rows tall: the window only moves when the cursor leaves it.
func (p *sprintPicker) scroll(termHeight int) {
	p.offset = scrollWindow(p.offset, p.cursor, len(p.rows), helpVisibleRows(len(p.rows), termHeight))
}

// view renders the dialog on a terminal termWidth x termHeight: the rows that
// fit, the cursor's always among them and accented, each label cut with "…"
// so the dialog stays narrower than the terminal.
func (p sprintPicker) view(termWidth, termHeight int) string {
	p.scroll(termHeight)
	maxWidth := termWidth - 4
	// The border, the padding either side and the "❯ " gutter.
	labelWidth := max(maxWidth-2-2*helpPad-2, 1)
	var lines []string
	end := min(p.offset+helpVisibleRows(len(p.rows), termHeight), len(p.rows))
	for i := p.offset; i < end; i++ {
		label := ansi.Truncate(p.rows[i].label, labelWidth, "…")
		if i == p.cursor {
			lines = append(lines, selectedRowStyle.Render("❯ "+label))
		} else {
			lines = append(lines, "  "+label)
		}
	}
	sep := helpTextStyle.Render(" · ")
	lines = append(lines, "", hint("enter", "Choose")+sep+hint("esc", "Cancel"))
	body := padBody(strings.Join(lines, "\n"), helpPad)
	width := min(max(lipgloss.Width(body)+2+helpPad, lipgloss.Width(p.title)+8), maxWidth)
	return titledBox(p.title, body, width, lipgloss.Height(body)+2, true)
}

// pickSize is the terminal size the sprint picker lays out for, 80x24 while
// the size is unknown, as in mainView.
func (m model) pickSize() (width, height int) {
	if m.width == 0 {
		return 80, 24
	}
	return m.width, m.height
}

// pickHeight is the terminal height the sprint picker lays out for (see
// pickSize).
func (m model) pickHeight() int {
	_, h := m.pickSize()
	return h
}

// openSprintPick opens picker p, scrolled to its cursor.
func (m *model) openSprintPick(p sprintPicker) {
	p.scroll(m.pickHeight())
	m.sprintPick, m.mode = p, modeSprintPick
}

// openJumpPicker is K: it opens the dialog listing every sprint, the cursor on
// the board's sprint when one is shown, or says there is none.
func (m model) openJumpPicker() (tea.Model, tea.Cmd) {
	p := newJumpPicker(m.doc)
	if len(p.rows) == 0 {
		m.status = msgNoOpenSprints
		return m, nil
	}
	if m.view.onBoard() {
		p.cursor = max(slices.IndexFunc(p.rows, func(r sprintPickRow) bool { return r.sprintID == m.view.sprint }), 0)
	}
	m.openSprintPick(p)
	return m, nil
}

// openAssignPicker is p on a backlog task: it opens the dialog listing the open
// sprints, or says there is none.
func (m model) openAssignPicker(task *todo.Item) (tea.Model, tea.Cmd) {
	p := newAssignPicker(m.doc, task)
	if len(p.rows) == 0 {
		m.status = msgNoOpenSprints
		return m, nil
	}
	m.openSprintPick(p)
	return m, nil
}

// openSendToPicker is o: it opens the dialog that moves the selected sprint task
// to another open sprint or back to the Backlog. A category, a subtask (it
// follows its task), a backlog task and a task of a closed sprint get a footer
// hint instead.
func (m model) openSendToPicker() (tea.Model, tea.Cmd) {
	it := m.selectedItem()
	switch {
	case it == nil:
		return m, nil
	case !it.IsTask():
		m.status = "only tasks move between sprints"
		return m, nil
	case it.IsSubtask():
		m.status = "subtasks move with their task"
		return m, nil
	case it.SprintID() == "":
		m.status = "use a to assign it to a sprint"
		return m, nil
	case m.sprintClosed(it.SprintID()):
		m.status = todo.ErrSprintClosed.Error()
		return m, nil
	}
	m.openSprintPick(newSendToPicker(m.doc, it))
	return m, nil
}

// updateSprintPick handles keys in the sprint picker: the arrows move (the
// rows scroll with the cursor), enter applies the highlighted row, esc or q
// cancels.
func (m model) updateSprintPick(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up":
		m.sprintPick.move(-1)
		m.sprintPick.scroll(m.pickHeight())
	case "down":
		m.sprintPick.move(1)
		m.sprintPick.scroll(m.pickHeight())
	case "esc", "q":
		m.mode = modeMain
	case "enter":
		p := m.sprintPick
		m.mode, m.status = modeMain, ""
		return m.applySprintPick(p, p.rows[p.cursor])
	}
	return m, nil
}

// applySprintPick carries out row r chosen in picker p: it shows the sprint,
// assigns the task, or sends it to the Backlog.
func (m model) applySprintPick(p sprintPicker, r sprintPickRow) (tea.Model, tea.Cmd) {
	switch {
	case p.purpose == pickJump:
		m.showSprint(r.sprintID)
	case r.backlog:
		m.commitMoved(p.taskID, func(ctx context.Context, s *todo.Store) (todo.WriteResult, error) {
			return s.Unassign(ctx, p.taskID)
		})
	default:
		m.commitMoved(p.taskID, func(ctx context.Context, s *todo.Store) (todo.WriteResult, error) {
			return s.Assign(ctx, p.taskID, r.sprintID)
		})
	}
	return m, nil
}

// commitMoved commits write, which moves task id to another sprint or the
// Backlog, like commit. When the task left the view, the cursor keeps its row
// index (clamped). When it left, or the view is the unified one (where it stays
// shown but its membership changed), the footer says where the task went, e.g.
// "moved to Sprint 2 — new" or "moved to Backlog", unless a save error or the
// external-reload notice already holds it.
func (m *model) commitMoved(id string, write storeWrite) {
	m.commit(id, write)
	it, ok := m.doc.ByID(id)
	if !ok || (m.focused().shown[it] && !m.view.all) || m.err != nil || m.status != "" {
		return
	}
	if s, ok := m.doc.SprintByID(it.SprintID()); ok {
		m.status = fmt.Sprintf("moved to %s — %s", sprintLabel(s), it.Status.Label())
	} else {
		m.status = "moved to " + todo.ColBacklog.Title()
	}
}
