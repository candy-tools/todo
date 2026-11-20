package tui

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/candy-tools/todo/internal/todo"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func (m model) View() string {
	switch m.mode {
	case modeForm:
		return placeCenter(m.mainView(true), m.form.view())
	case modeConfirm:
		return placeCenter(m.mainView(true), confirmModal(m.confirmQuestion(), m.confirmOnDelete, m.width))
	case modeHelp:
		return placeCenter(m.mainView(true), helpModal(m.width, m.height, m.helpOffset))
	case modeStatePick, modeStateFilter:
		return placeCenter(m.mainView(true), m.pick.view())
	case modeSprintPick:
		return placeCenter(m.mainView(true), m.sprintPick.view(m.pickSize()))
	default:
		return m.mainView(false)
	}
}

// mainView composes the header, the task tree, and the footer to the terminal
// size. When dim is true the tree renders unfocused (the dimmed backdrop behind
// a modal).
func (m model) mainView(dim bool) string {
	w, h := m.width, m.height
	if w == 0 {
		w, h = 80, 24
	}
	bodyH := h - headerHeight - 1 // the footer takes the last line
	if bodyH < 3 {
		bodyH = 3
	}
	var body string
	switch {
	case m.view.screen == screenPlan:
		body = m.planBody(w, bodyH, dim)
	case m.view.onBoard():
		// The Sprints workspace: the sprint list beside the shown sprint's
		// board, split view or not.
		sw := sidebarWidth(w)
		left := titledBox("Sprints", m.sprints.view(m.doc, sw-2, m.sprintListHeight()), sw, bodyH, !dim && m.listFocus)
		right := titledBox(m.boardTitle(), m.boardPane(m.boardWidth()), w-sw, bodyH, !dim && !m.listFocus)
		body = lipgloss.JoinHorizontal(lipgloss.Top, left, right)
	case m.splitShown():
		sw := sidebarWidth(w)
		left := titledBox("Categories", m.side.view(sw-2, m.treeViewHeight()), sw, bodyH, !dim && m.sideFocus)
		right := titledBox(m.treeTitle(), m.tree.view(w-sw-2, m.treeViewHeight()), w-sw, bodyH, !dim && !m.sideFocus)
		body = lipgloss.JoinHorizontal(lipgloss.Top, left, right)
	default:
		body = titledBox(m.treeTitle(), m.tree.view(w-2, m.treeViewHeight()), w, bodyH, !dim)
	}
	return m.header(w) + "\n" + body + "\n" + m.footer(w)
}

// treeTitle is the title of the main tree's pane off the Sprints workspace:
// "Tasks" in the unified view; on the Backlog, "Backlog", or in the split the
// path of the sidebar's category ("Backlog" on All).
func (m model) treeTitle() string {
	if m.view.all {
		return "Tasks"
	}
	if path := categoryPath(m.backlogSelection()); m.splitShown() && path != "" {
		return path
	}
	return todo.ColBacklog.Title()
}

// boardTitle is the title of the Sprints workspace's board pane: "Closed
// sprints" beside the Closed group row, else the shown sprint's name, marked
// "✓ closed" when it is done, or "Board" when it shows none.
func (m model) boardTitle() string {
	s, ok := m.doc.SprintByID(m.view.sprint)
	switch {
	case m.onClosedRow():
		return "Closed sprints"
	case !ok:
		return "Board"
	case s.Status == todo.SprintDone:
		return todo.Printable(s.Name) + " ✓ closed"
	}
	return todo.Printable(s.Name)
}

// boardPane renders the Sprints workspace's board pane, width wide: the
// closed sprints beside the Closed group row (see closedPane), the shown
// sprint's tree, or a dim line saying why there is none.
func (m model) boardPane(width int) string {
	switch {
	case m.onClosedRow():
		return m.closedPane(width, m.paneRows())
	case m.view.sprint != "":
		return m.boardBody(width)
	case len(m.doc.Sprints) == 0:
		return "  " + helpTextStyle.Render("no sprints yet — n creates one")
	}
	return "  " + helpTextStyle.Render(msgNoOpenSprintsPane)
}

// msgNoOpenSprintsPane is the dim line of the board pane when every sprint is
// closed.
const msgNoOpenSprintsPane = "no open sprints"

// closedPane renders the read-only pane beside the Closed group row, width x
// height: each closed sprint, newest first, as "── ✓ name" over its goal ("—"
// without one) and its task count, cut with "…"; when they do not all fit,
// "… N more" ends it. With no open sprint it says so first.
func (m model) closedPane(width, height int) string {
	var lines []string
	if len(m.doc.OpenSprints()) == 0 {
		lines = append(lines, "  "+helpTextStyle.Render(msgNoOpenSprintsPane), "")
	}
	var done []*todo.Sprint
	for _, s := range m.doc.Sprints {
		if s.Status == todo.SprintDone {
			done = append(done, s)
		}
	}
	slices.Reverse(done)
	for i, s := range done {
		need := rowLines
		if i < len(done)-1 {
			need++ // room for "… N more" should the next ones not fit
		}
		if len(lines)+need > height {
			if len(lines) < height {
				lines = append(lines, "  "+helpTextStyle.Render(fmt.Sprintf("… %d more", len(done)-i)))
			}
			break
		}
		lines = append(lines, closedSprintLines(s, m.doc.ColumnCounts(s.ID), width)...)
	}
	return strings.Join(lines, "\n")
}

// closedSprintLines renders closed sprint s in the Closed sprints pane, width
// wide: its check mark and name, then its goal and how many tasks it holds.
func closedSprintLines(s *todo.Sprint, counts map[todo.Column]int, width int) []string {
	const lead = "  ── ✓ "
	name := ansi.Truncate(todo.Printable(s.Name), max(width-ansi.StringWidth(lead), 1), "…")
	first := "  " + helpTextStyle.Render("── ") + doneStyle.Render("✓") + " " + categoryStyle.Render(name)
	n := 0
	for _, c := range todo.SprintColumns() {
		n += counts[c]
	}
	count := fmt.Sprintf(" · %d task", n)
	if n != 1 {
		count += "s"
	}
	goal := todo.Printable(s.Goal)
	if goal == "" {
		goal = "—"
	}
	const indent = "     "
	goal = ansi.Truncate(goal, max(width-len(indent)-ansi.StringWidth(count), 1), "…")
	second := indent + plainStyle.Render(goal) + helpTextStyle.Render(count)
	return []string{ansi.Truncate(first, width, ""), ansi.Truncate(second, width, "")}
}

// boardBody renders the shown sprint's board, width wide: as much of its head
// as fits (see boardHeadRows) over its tree, or, in the columns layout, over
// the three columns, the head's tab bar giving way to the column titles.
func (m model) boardBody(width int) string {
	s, ok := m.doc.SprintByID(m.view.sprint)
	if !ok {
		return m.tree.view(width, m.treeViewHeight())
	}
	counts := m.doc.ColumnCounts(s.ID)
	head := boardHead(s, counts, m.view.column, width)
	body := ""
	if m.columnsShown() {
		head[len(head)-1] = columnTitles(counts, m.view.column, width)
		body = m.columnsBody(s.ID, width, m.treeViewHeight())
	} else {
		body = m.tree.view(width, m.treeViewHeight())
	}
	head = head[len(head)-m.boardHeadRows():]
	return strings.Join(append(head, body), "\n")
}

// boardHeadRows is how many of the board head's lines show: all of them, less
// as many as a short pane needs to keep one task row. The head gives up its
// lines from the top, the goal first, then the progress, then the tab bar (in
// the columns layout, the column titles, which take its row).
func (m model) boardHeadRows() int {
	return min(boardChrome, m.paneRows()-1)
}

// boardWidth is the inner width of the Sprints workspace's board pane at the
// current terminal size. It mirrors the layout math in mainView.
func (m model) boardWidth() int {
	w := m.width
	if w == 0 {
		w = 80 // matches mainView's fallback when the terminal size is unknown
	}
	return w - sidebarWidth(w) - 2
}

// treeViewHeight is how many task rows the tree pane can show at the current
// terminal size — the height handed to tree.view — less the board head's rows
// on a sprint board (the same in the columns layout, whose title row takes the
// tab bar's). Update stores it on the trees (see fitTrees) so their
// scroll math (reconcileOffset) matches what mainView renders; the two must
// agree or the viewport clamp would fight the offset.
func (m model) treeViewHeight() int {
	if m.view.onBoard() {
		return m.paneRows() - m.boardHeadRows()
	}
	return m.paneRows()
}

// sprintListHeight is how many rows the Sprints workspace's sidebar can show:
// the full pane height, whatever the board beside it gives to its own lines.
func (m model) sprintListHeight() int { return m.paneRows() }

// paneRows is how many rows fit inside a pane's border at the current
// terminal size. It mirrors the body-height layout math in mainView.
func (m model) paneRows() int {
	h := m.height
	if m.width == 0 {
		h = 24 // matches mainView's fallback when the terminal size is unknown
	}
	bodyH := h - headerHeight - 1
	if bodyH < 3 {
		bodyH = 3
	}
	vh := bodyH - 2
	if vh < 1 {
		vh = 1
	}
	return vh
}

// headerHeight is how many lines the header takes: the title line and the tab
// bar.
const headerHeight = 2

// header renders the title line (app, version, file, overall progress) and,
// under it, the screen's line: a sprint board's name and tab bar, or the
// Backlog's, the sprint-less Sprints workspace's or the unified view's label.
func (m model) header(width int) string {
	done, total := overallCounts(m.doc)
	left := headerAppStyle.Render("todo") + " " + headerHintSty.Render(m.version) + "  " + headerHintSty.Render(filepath.Base(m.store.Path()))
	right := headerHintSty.Render(fmt.Sprintf("%d/%d done", done, total))
	gap := width - 1 - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}
	title := ansi.Truncate(" "+left+strings.Repeat(" ", gap)+right, width, "")
	return title + "\n" + m.screenLine(width)
}

// screenLine is the header's second line for the view on screen: the
// workspace switcher (Planning marked while it is shown) and the breadcrumb.
func (m model) screenLine(width int) string {
	return switcherBar(m.view.screen, m.crumb(), width)
}

// crumb is the breadcrumb after the switcher: where the view on screen is.
func (m model) crumb() string {
	switch {
	case m.view.all:
		total := 0
		for _, n := range m.doc.ColumnCounts("") {
			total += n
		}
		return fmt.Sprintf("All states · %d tasks", total)
	case m.view.screen == screenPlan:
		return m.planCrumb()
	case m.view.screen == screenBacklog:
		crumb := "Backlog"
		if path := categoryPath(m.backlogSelection()); path != "" {
			crumb += " › " + path
		}
		return fmt.Sprintf("%s · %d tasks waiting", crumb, m.doc.ColumnCounts("")[todo.ColBacklog])
	case m.sprints.onGroup(m.doc):
		return "Sprints › Closed"
	case len(m.doc.Sprints) == 0:
		return "Sprints › no sprints yet"
	}
	s, ok := m.doc.SprintByID(m.view.sprint)
	if !ok {
		return "Sprints"
	}
	name := todo.Printable(s.Name)
	if s.Status == todo.SprintDone {
		name += " ✓"
	}
	return "Sprints › " + name + " › " + m.view.column.Title()
}

// backlogSelection is the item the Backlog's crumb is about: in the split
// view the category under the sidebar's cursor (none on All), whichever pane
// has the keys, else the item in the tree.
func (m model) backlogSelection() *todo.Item {
	t := &m.tree
	if m.splitShown() {
		t = &m.side
	}
	if t.onPlaceholder() {
		return nil
	}
	return t.selected()
}

// categoryPath is the path of it's category from the root, e.g.
// "Work › Reviews", or "" when it is nil or has none.
func categoryPath(it *todo.Item) string {
	var path []string
	for c := it.EnclosingCategory(); c != nil; c = c.Parent {
		if c.Kind == todo.Category {
			path = append([]string{todo.Printable(c.Title)}, path...)
		}
	}
	return strings.Join(path, " › ")
}

func (m model) footer(width int) string {
	switch {
	case m.mode == modeSearch:
		// The live search bar replaces the footer while typing a query.
		return ansi.Truncate(" "+helpKeyStyle.Render("/")+" "+m.search.View(), width, "")
	case m.err != nil:
		return ansi.Truncate(" "+errStyle.Render(m.err.Error()), width, "")
	case m.status != "":
		// A transient message (a refused move, where a task went) wins over the
		// move and filter lines; the next key clears it.
		return ansi.Truncate(" "+helpTextStyle.Render(m.status), width, "")
	case m.view.screen == screenPlan:
		// Planning's own keys; the search and state filter do not apply there.
		return fitHints(m.planFooter(), width)
	case m.onList():
		// The sidebar's own keys, ahead of the board's search filter: the
		// selected sprint's, or the fold on the Closed group row.
		parts := []string{hint("?", "Help"), hint("n", "New")}
		switch {
		case m.onClosedRow():
			parts = append(parts, hint("←→", "Fold"))
		case m.sprints.selected(m.doc) != nil:
			parts = append(parts, hint("e", "Edit"), hint("c", "Close/reopen"), hint("d", "Delete"), hint("tab", "Board"))
		}
		return fitHints(append(parts, hint("q", "Quit")), width)
	case m.focused().grabbed:
		// Move mode: the arrows reorder/re-nest the grabbed item.
		parts := []string{
			helpKeyStyle.Render("MOVING"),
			hint("↑↓", "Move"), hint("→", "Child"), hint("←", "Sibling"), hint("m/esc", "Drop"),
		}
		return ansi.Truncate(" "+strings.Join(parts, "  "), width, "")
	case m.view.filter != "" || len(m.view.activeStates()) > 0:
		// A search and/or state filter is applied but the bar is closed: show it,
		// with how to edit/clear.
		var info []string
		if m.view.filter != "" {
			info = append(info, helpKeyStyle.Render("filter")+helpTextStyle.Render(": "+m.view.filter), hint("/", "Edit"))
		}
		if states := m.view.activeStates(); len(states) > 0 {
			names := make([]string, len(states))
			for i, st := range states {
				names[i] = st.Label()
			}
			info = append(info, helpKeyStyle.Render("states")+helpTextStyle.Render(": "+strings.Join(names, ", ")), hint("f", "Edit"))
		}
		info = append(info, hint("esc", "Clear"))
		return ansi.Truncate(" "+strings.Join(info, "  "), width, "")
	}
	// The full key list lives in the "?" dialog; the footer only points at it.
	parts := []string{hint("?", "Help"), hint("/", "Search"), hint("q", "Quit")}
	switch {
	case m.view.onBoard():
		parts = m.boardHints()
	case m.view.screen == screenBacklog && !m.view.all:
		parts = m.backlogHints()
	case m.splitShown():
		parts = append(parts, hint("tab", "Pane"))
	}
	return fitHints(parts, width)
}

// boardHints are the board's footer hints: the column keys, a/A to move a task
// unless the sprint is closed, then L where the board is wide enough for columns.
func (m model) boardHints() []string {
	parts := []string{hint("?", "Help"), hint("tab", "Sprints"), hint("[ ] 1-3", "Column")}
	if !m.sprintClosed(m.view.sprint) { // a closed sprint's tasks cannot move
		parts = append(parts, hint("a/A", "Move task"))
	}
	switch {
	case m.columnsShown():
		parts = append(parts, hint("L", "Tabs"))
	case m.boardWidth() >= minColumnsWidth:
		parts = append(parts, hint("L", "Columns"))
	}
	return append(parts, hint("/", "Search"), hint("q", "Quit"))
}

// backlogHints are the Backlog's footer hints, the most important first: the
// way to the help, between the split's panes, to a sprint and to the other
// workspaces.
func (m model) backlogHints() []string {
	parts := []string{hint("?", "Help")}
	split := "Split"
	if m.splitShown() {
		parts = append(parts, hint("tab", "Pane"))
		split = "Single tree"
	}
	return append(parts, hint("a", "Assign"), hint("v", split), hint("s", "Sprints"), hint("p", "Plan"),
		hint("/", "Search"), hint("q", "Quit"))
}

// fitHints joins the footer hints that fit width, from the first: those that
// do not fit are dropped whole, never cut.
func fitHints(parts []string, width int) string {
	line := ""
	for _, p := range parts {
		next := line + "  " + p
		if line == "" {
			next = " " + p
		}
		if ansi.StringWidth(next) > width {
			break
		}
		line = next
	}
	return line
}

// overallCounts totals done/total tasks across the whole document.
func overallCounts(d *todo.Document) (done, total int) {
	for _, r := range d.Roots {
		if r.Kind == todo.Task {
			total++
			if r.IsDone() {
				done++
			}
		}
		dn, tt := r.TaskCounts()
		done += dn
		total += tt
	}
	return done, total
}
