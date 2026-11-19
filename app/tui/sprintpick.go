package tui

import (
	"context"
	"fmt"
	"strings"

	"github.com/candy-tools/todo/internal/todo"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// sprintPickPurpose is what choosing a row of the sprint picker does.
type sprintPickPurpose int

const (
	pickJump   sprintPickPurpose = iota // K: show the chosen sprint's board
	pickAssign                          // p: put a backlog task in the chosen sprint
	pickSendTo                          // o: move a sprint task to another sprint or the Backlog
)

// sprintPickRow is one choice of the sprint picker: a sprint, or (backlog set)
// the Backlog.
type sprintPickRow struct {
	sprintID string
	label    string
	backlog  bool
}

// sprintPicker is the dialog behind K, p and o: a titled list of sprints (and,
// for o, the Backlog) with a cursor. taskID is the task p and o act on.
type sprintPicker struct {
	title   string
	purpose sprintPickPurpose
	rows    []sprintPickRow
	cursor  int
	taskID  string
}

// sprintRow is the picker row of sprint s: done sprints carry a check mark.
func sprintRow(s *todo.Sprint) sprintPickRow {
	label := s.Name
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

// view renders the dialog: one row per choice, the cursor row accented.
func (p sprintPicker) view() string {
	var lines []string
	for i, r := range p.rows {
		if i == p.cursor {
			lines = append(lines, selectedRowStyle.Render("❯ "+r.label))
		} else {
			lines = append(lines, "  "+r.label)
		}
	}
	sep := helpTextStyle.Render(" · ")
	lines = append(lines, "", hint("enter", "Choose")+sep+hint("esc", "Cancel"))
	body := padBody(strings.Join(lines, "\n"), helpPad)
	width := max(lipgloss.Width(body)+2+helpPad, lipgloss.Width(p.title)+8)
	return titledBox(p.title, body, width, lipgloss.Height(body)+2, true)
}

// openJumpPicker is K: it opens the dialog listing every sprint, or says there
// is none.
func (m model) openJumpPicker() (tea.Model, tea.Cmd) {
	p := newJumpPicker(m.doc)
	if len(p.rows) == 0 {
		m.status = msgNoOpenSprints
		return m, nil
	}
	m.sprintPick, m.mode = p, modeSprintPick
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
	m.sprintPick, m.mode = p, modeSprintPick
	return m, nil
}

// openSendToPicker is o: it opens the dialog that moves the selected sprint task
// to another open sprint or back to the Backlog. A category, a subtask (it
// follows its task) and a backlog task get a footer hint instead.
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
		m.status = "use p to assign it to a sprint"
		return m, nil
	}
	m.sprintPick, m.mode = newSendToPicker(m.doc, it), modeSprintPick
	return m, nil
}

// updateSprintPick handles keys in the sprint picker: the arrows move, enter
// applies the highlighted row, esc or q cancels.
func (m model) updateSprintPick(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up":
		m.sprintPick.move(-1)
	case "down":
		m.sprintPick.move(1)
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
		m.status = fmt.Sprintf("moved to %s — %s", s.Name, it.Status.Label())
	} else {
		m.status = "moved to " + todo.ColBacklog.Title()
	}
}
