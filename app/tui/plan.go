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

// Footer statuses of Planning.
const (
	msgPlanGone     = "the planned sprint is gone or closed"
	msgOnlyNextBack = "only tasks in Next go back to the Backlog"
	msgOnlyToggleR  = "only new and ready tasks toggle with r"
)

// planRow is one row of a Planning pane: a group header (a category's path,
// or a board column with its count), or a task.
type planRow struct {
	header string
	task   *todo.Item
}

// planner is Planning ("p"): the Backlog's top-level tasks beside one open
// sprint's, where tasks are pulled into the sprint and sent back. It is
// session only, never stored. The rows are derived from the list on every use;
// a side's selection is a task ID, which falls back to the row index it was
// last on (clamped, onto the nearest task) when its task left the side.
type planner struct {
	sprintID string // the planned sprint
	right    bool   // whether the keys go to the sprint side
	// leftSel and rightSel are the selected task of each side ("" for none);
	// leftCur and rightCur the row index it was last seen on; leftOff and
	// rightOff the index of each side's first row shown.
	leftSel, rightSel string
	leftCur, rightCur int
	leftOff, rightOff int
}

// leftRows lists the Backlog side of d: per category in document order, a
// header with the category's path, then that category's own top-level
// backlog tasks, in order. Categories without such a task are skipped.
func (p *planner) leftRows(d *todo.Document) []planRow {
	var rows []planRow
	for _, c := range d.Enumerate() {
		if c.Kind != todo.Category {
			continue
		}
		head := len(rows)
		for _, it := range c.Children {
			if it.Kind == todo.Task && it.SprintID() == "" {
				rows = append(rows, planRow{task: it})
			}
		}
		if len(rows) > head {
			rows = slices.Insert(rows, head, planRow{header: categoryPath(c)})
		}
	}
	return rows
}

// rightRows lists the sprint side of d: for each board column, a header with
// its title and task count, then the planned sprint's top-level tasks there,
// in document order.
func (p *planner) rightRows(d *todo.Document) []planRow {
	tasks := d.SprintTasks(p.sprintID)
	var rows []planRow
	for _, c := range todo.SprintColumns() {
		var in []planRow
		for _, it := range tasks {
			if it.Column() == c {
				in = append(in, planRow{task: it})
			}
		}
		rows = append(rows, planRow{header: fmt.Sprintf("%s (%d)", c.Title(), len(in))})
		rows = append(rows, in...)
	}
	return rows
}

// rows lists the rows of the given side of d.
func (p *planner) rows(d *todo.Document, right bool) []planRow {
	if right {
		return p.rightRows(d)
	}
	return p.leftRows(d)
}

// side returns the selection, its last row index and the window offset of the
// given side.
func (p *planner) side(right bool) (sel *string, cur, off *int) {
	if right {
		return &p.rightSel, &p.rightCur, &p.rightOff
	}
	return &p.leftSel, &p.leftCur, &p.leftOff
}

// sync puts the given side's cursor on its selected task, wherever it is in
// rows now; when that task left the side (or none was selected), on the task
// row nearest the last index, clamped. It returns the cursor's row index, -1
// when the side has no task.
func (p *planner) sync(rows []planRow, right bool) int {
	sel, cur, _ := p.side(right)
	if i := slices.IndexFunc(rows, func(r planRow) bool { return r.task != nil && r.task.ID == *sel }); *sel != "" && i >= 0 {
		*cur = i
		return i
	}
	*sel = ""
	i := taskRowNear(rows, *cur, 1)
	if i >= 0 {
		*cur, *sel = i, rows[i].task.ID
	}
	return i
}

// selected returns the selected task of the given side of d, nil when it has
// none.
func (p *planner) selected(d *todo.Document, right bool) *todo.Item {
	rows := p.rows(d, right)
	if i := p.sync(rows, right); i >= 0 {
		return rows[i].task
	}
	return nil
}

// move steps the focused side's selection by delta rows, onto the nearest
// task row that way (headers are never selected), stopping at either end.
func (p *planner) move(d *todo.Document, delta int) {
	rows := p.rows(d, p.right)
	i := p.sync(rows, p.right)
	if i < 0 {
		return
	}
	dir := 1
	if delta < 0 {
		dir = -1
	}
	if j := taskRowNear(rows, i+delta, dir); j >= 0 {
		sel, cur, _ := p.side(p.right)
		*cur, *sel = j, rows[j].task.ID
	}
}

// fit syncs both sides with d and keeps each cursor inside the height rows
// its pane shows: the window only moves when the cursor leaves it.
func (p *planner) fit(d *todo.Document, height int) {
	for _, right := range []bool{false, true} {
		rows := p.rows(d, right)
		_, cur, off := p.side(right)
		p.sync(rows, right)
		*off = scrollWindow(*off, *cur, len(rows), height)
	}
}

// plan makes sprint id the planned one. Another sprint starts its side at the
// top.
func (p *planner) plan(id string) {
	if id != p.sprintID {
		p.sprintID, p.rightSel, p.rightCur, p.rightOff = id, "", 0, 0
	}
}

// taskRowNear returns the index of the task row nearest i (clamped into rows),
// looking first in direction dir (±1), then the other way; -1 when rows hold
// no task.
func taskRowNear(rows []planRow, i, dir int) int {
	if len(rows) == 0 {
		return -1
	}
	i = min(max(i, 0), len(rows)-1)
	for _, d := range []int{dir, -dir} {
		for j := i; j >= 0 && j < len(rows); j += d {
			if rows[j].task != nil {
				return j
			}
		}
	}
	return -1
}

// openPlanning is p from the Backlog or the Sprints workspace: it plans the
// shown sprint when it is open, else the last viewed sprint while it is open,
// else the first open one, with the keys on the Backlog side. With no open
// sprint the footer says how to make one. Planning itself is never stored,
// but entering it stores the view it leaves when the list holds another (a
// sprint browsed to in the sidebar), or a quit from Planning would forget it.
// Like b and s, it starts the footer afresh: an earlier failure is cleared,
// and only a failure of that save shows over Planning.
func (m *model) openPlanning() {
	id := m.view.sprint
	if s, ok := m.doc.SprintByID(id); !m.view.onBoard() || !ok || s.Status != todo.SprintOpen {
		var found bool
		if id, found = m.lastOpenSprint(); !found {
			m.status = msgNoOpenSprints
			return
		}
	}
	m.err = nil
	if m.viewOnScreen() != m.doc.View {
		m.saveView()
	}
	m.switchScreen(screenPlan, "")
	m.plan.plan(id)
	m.plan.right = false
}

// leavePlanning is esc or p in Planning: it shows the planned sprint's board
// with the cursor in its Next section, where planning put the tasks (when it
// holds any), the board focused, and stores the view.
func (m *model) leavePlanning() {
	m.showSprint(m.plan.sprintID)
	m.listFocus = false
	if i, ok := m.tree.sectionStart(todo.ColNext); ok {
		m.tree.showSectionAt(i)
	}
}

// leaveGonePlan leaves Planning for the Sprints workspace, without storing the
// view, when the list no longer holds the planned sprint open (another
// session deleted or closed it, say), and says so in the footer.
func (m *model) leaveGonePlan() {
	if m.view.screen != screenPlan {
		return
	}
	if s, ok := m.doc.SprintByID(m.plan.sprintID); ok && s.Status == todo.SprintOpen {
		return
	}
	m.openWorkspace(m.sprintsTarget())
	m.status = msgPlanGone
}

// afterPlanWrite runs after a write of Planning. When it left Planning, the
// sprint was gone or closed under the write: that, not the write's error or
// refusal, is what the footer says.
func (m *model) afterPlanWrite() {
	if m.view.screen != screenPlan {
		m.err = nil
		m.status = msgPlanGone
	}
}

// updatePlan handles the keys of Planning: the arrows move the focused side's
// selection, tab switches side, → or a pulls the Backlog side's task into the
// sprint, ← or A sends the sprint side's Next task back, r toggles it between
// new and ready, k and K change the planned sprint, esc and p leave. b, s, q
// and ? work as everywhere; every other key does nothing.
func (m model) updatePlan(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key := msg.String(); key {
	case "b", "s", "q", "?":
		next, cmd, _ := m.updateScreenKey(msg)
		return next, cmd
	case "esc", "p":
		m.leavePlanning()
	case "tab", "shift+tab":
		m.plan.right = !m.plan.right
	case "up":
		m.plan.move(m.doc, -1)
	case "down":
		m.plan.move(m.doc, 1)
	case "pgup":
		m.plan.move(m.doc, -navStep)
	case "pgdown":
		m.plan.move(m.doc, navStep)
	case "k":
		if id, ok := nextOpenAfter(m.doc, m.plan.sprintID); ok {
			m.plan.plan(id)
		}
	case "K":
		p := newPlanPicker(m.doc)
		p.cursor = max(slices.IndexFunc(p.rows, func(r sprintPickRow) bool { return r.sprintID == m.plan.sprintID }), 0)
		m.openSprintPick(p)
	case "right", "a":
		if !m.plan.right {
			m.pullIn()
		}
	case "left", "A":
		if m.plan.right {
			m.sendBack()
		}
	case "r":
		if m.plan.right {
			m.toggleReady()
		}
	}
	return m, nil
}

// pullIn assigns the Backlog side's task to the planned sprint, where it
// becomes new and selected; the Backlog side keeps its row index.
func (m *model) pullIn() {
	it := m.plan.selected(m.doc, false)
	if it == nil {
		return
	}
	id, sprint := it.ID, m.plan.sprintID
	m.commitMoved(id, func(ctx context.Context, s *todo.Store) (todo.WriteResult, error) {
		return s.Assign(ctx, id, sprint)
	})
	if got, ok := m.doc.ByID(id); ok && got.SprintID() == sprint {
		m.plan.rightSel = id
	}
	m.afterPlanWrite()
}

// sendBack returns the sprint side's task to the Backlog, where it becomes
// open and selected, when it is in Next; the sprint side keeps its row index.
func (m *model) sendBack() {
	it := m.plan.selected(m.doc, true)
	switch {
	case it == nil:
		return
	case it.Column() != todo.ColNext:
		m.status = msgOnlyNextBack
		return
	}
	id := it.ID
	m.commitMoved(id, func(ctx context.Context, s *todo.Store) (todo.WriteResult, error) {
		return s.Unassign(ctx, id)
	})
	if got, ok := m.doc.ByID(id); ok && got.SprintID() == "" {
		m.plan.leftSel = id
	}
	m.afterPlanWrite()
}

// toggleReady flips the sprint side's task between new and ready.
func (m *model) toggleReady() {
	it := m.plan.selected(m.doc, true)
	var st todo.Status
	switch {
	case it == nil:
		return
	case it.Status == todo.New:
		st = todo.Ready
	case it.Status == todo.Ready:
		st = todo.New
	default:
		m.status = msgOnlyToggleR
		return
	}
	m.commit(it.ID, stateWrite(it.ID, st, todo.StateOptions{}))
	m.afterPlanWrite()
}

// planBody renders Planning's two panes side by side, width wide and height
// tall: the Backlog on the left, the planned sprint on the right, the focused
// one highlighted (none when dim, behind a dialog).
func (m model) planBody(width, height int, dim bool) string {
	lw := width / 2
	rw := width - lw
	rows := m.paneRows()
	left := titledBox(todo.ColBacklog.Title(), m.planPane(false, lw-2, rows), lw, height, !dim && !m.plan.right)
	right := titledBox(m.planTitle(), m.planPane(true, rw-2, rows), rw, height, !dim && m.plan.right)
	return lipgloss.JoinHorizontal(lipgloss.Top, left, right)
}

// planTitle is the sprint pane's title: the planned sprint's name and goal,
// or just its name without a goal.
func (m model) planTitle() string {
	s, ok := m.doc.SprintByID(m.plan.sprintID)
	switch {
	case !ok:
		return ""
	case strings.TrimSpace(s.Goal) == "":
		return todo.Printable(s.Name)
	}
	return todo.Printable(s.Name) + " — " + todo.Printable(s.Goal)
}

// planPane renders the rows of one side that fit width x height, the
// selection always among them. A side without a task draws its headers; the
// Backlog side, which has none then, a dim line saying it is empty (the sprint
// side always has its three column headers).
func (m model) planPane(right bool, width, height int) string {
	p := m.plan // a copy: rendering never moves the real selection
	rows := p.rows(m.doc, right)
	cur := p.sync(rows, right)
	if len(rows) == 0 {
		return "  " + helpTextStyle.Render("nothing in the Backlog")
	}
	_, _, off := p.side(right)
	start := scrollWindow(*off, max(cur, 0), len(rows), height)
	end := min(start+height, len(rows))
	lines := make([]string, 0, end-start)
	for i := start; i < end; i++ {
		lines = append(lines, ansi.Truncate(planRowString(rows[i], i == cur), width, "…"))
	}
	return strings.Join(lines, "\n")
}

// planRowString renders one Planning row: a header as the split view's task
// list draws its group headers; a task as its state glyph, title, state and
// subtask progress, accented behind a "❯ " gutter when selected.
func planRowString(r planRow, selected bool) string {
	if r.task == nil {
		return "  " + helpTextStyle.Render("── ") + categoryStyle.Render(r.header)
	}
	it := r.task
	glyph, glyphStyle := statusGlyph(it.Status, false)
	state, _, _ := taskTrailer(it, "")
	title := todo.Printable(it.DisplayTitle())
	if selected {
		return selectedRowStyle.Render("❯ " + glyph + " " + title + state)
	}
	titleStyle := plainStyle
	if it.IsDone() {
		titleStyle = doneTitleStyle
	}
	return "  " + glyphStyle.Render(glyph) + " " + titleStyle.Render(title) + helpTextStyle.Render(state)
}

// planCrumb is Planning's breadcrumb: the planned sprint's name, its tasks in
// Next and the tasks waiting in the Backlog.
func (m model) planCrumb() string {
	name := ""
	if s, ok := m.doc.SprintByID(m.plan.sprintID); ok {
		name = todo.Printable(s.Name)
	}
	return fmt.Sprintf("%s · %d in Next · %d in the Backlog", name,
		m.doc.ColumnCounts(m.plan.sprintID)[todo.ColNext], m.doc.ColumnCounts("")[todo.ColBacklog])
}

// planFooter is Planning's footer: the focused side's keys.
func (m model) planFooter() []string {
	if m.plan.right {
		return []string{hint("?", "Help"), hint("←/A", "Back"), hint("r", "Ready"), hint("tab", "Backlog side"), hint("k", "Other sprint"), hint("esc", "Done"), hint("q", "Quit")}
	}
	return []string{hint("?", "Help"), hint("→/a", "Pull in"), hint("tab", "Sprint side"), hint("k", "Other sprint"), hint("esc", "Done"), hint("q", "Quit")}
}
