package tui

import (
	"fmt"
	"strings"

	"github.com/candy-tools/todo/internal/todo"
	"github.com/charmbracelet/x/ansi"
)

// minColumnsWidth is the board pane's inner width the columns layout needs
// (three columns of 22 cells and their separators); on a narrower pane the
// board shows as tabs, whatever the layout flag says.
const minColumnsWidth = 72

// columnSep is drawn between the columns of the columns layout.
const columnSep = " │ "

// msgTooNarrow is the footer status for L on a board too narrow for columns.
const msgTooNarrow = "too narrow for columns"

// toggleColumns handles L: it switches the board between tabs and three
// columns, or, on a pane too narrow for columns, says so and leaves the
// layout flag as it is. It does nothing off the board (the Backlog, the
// unified view).
func (m *model) toggleColumns() {
	switch {
	case !m.view.onBoard():
	case m.boardWidth() < minColumnsWidth:
		m.status = msgTooNarrow
	default:
		m.columns = !m.columns
	}
}

// columnsShown reports whether the board renders as three columns: the
// layout is on, the Sprints workspace shows a board (not the unified view)
// and its pane is wide enough.
func (m model) columnsShown() bool {
	return m.columns && m.view.onBoard() && m.boardWidth() >= minColumnsWidth
}

// columnWidth is the width of each of the three columns on a board width
// wide, once the separators are taken out.
func columnWidth(width int) int {
	n := len(todo.SprintColumns())
	return max((width-(n-1)*ansi.StringWidth(columnSep))/n, 1)
}

// columnTitles renders the columns layout's title row, width wide, in place
// of the tab bar: every column's title and its task count in counts, the
// active one accented.
func columnTitles(counts map[todo.Column]int, active todo.Column, width int) string {
	cols := todo.SprintColumns()
	cells := make([]string, len(cols))
	for i, c := range cols {
		style := headerHintSty
		if c == active {
			style = selectedRowStyle
		}
		cells[i] = style.Render(fmt.Sprintf("%s %d", c.Title(), counts[c]))
	}
	return joinColumns(cells, columnWidth(width))
}

// columnsBody renders the columns under the title row, width wide and height
// rows tall: the active column is the board's tree, with its cursor and
// folds; the other two are previews of the sprint's tasks there (see
// columnPreview).
func (m model) columnsBody(sprint string, width, height int) string {
	cw := columnWidth(width)
	cols := todo.SprintColumns()
	cells := make([]string, len(cols))
	for i, c := range cols {
		if c == m.view.column {
			cells[i] = m.tree.view(cw, height)
		} else {
			cells[i] = columnPreview(m.doc, m.view, sprint, c, cw, height)
		}
	}
	return joinColumns(cells, cw)
}

// joinColumns lays cells (each one or more lines) side by side, every line
// cut with "…" and padded to width, separated by columnSep.
func joinColumns(cells []string, width int) string {
	split := make([][]string, len(cells))
	rows := 0
	for i, c := range cells {
		split[i] = strings.Split(c, "\n")
		rows = max(rows, len(split[i]))
	}
	sep := helpTextStyle.Render(columnSep)
	lines := make([]string, rows)
	for r := range rows {
		parts := make([]string, len(split))
		for i, cell := range split {
			line := ""
			if r < len(cell) {
				line = ansi.Truncate(cell[r], width, "…")
			}
			parts[i] = line + strings.Repeat(" ", max(width-ansi.StringWidth(line), 0))
		}
		lines[r] = strings.Join(parts, sep)
	}
	return strings.Join(lines, "\n")
}

// columnPreview renders a column the columns layout does not have the cursor
// in, width wide and at most height rows tall: one row per top-level task of
// sprint in column c that the view's search and state filter keep (as the
// tree would on that tab), in document order, each its state glyph and its
// title cut with "…". When they do not fit, the last row says how many more
// there are; with none, it is a dim "—".
func columnPreview(d *todo.Document, v *viewState, sprint string, c todo.Column, width, height int) string {
	vis := d.ColumnMatches(v.filter, sprint, c)
	// The tabs share one state filter; each tab reads only its own states.
	keep := map[todo.Status]bool{}
	for _, st := range c.States() {
		if v.states[st] {
			keep[st] = true
		}
	}
	if len(keep) > 0 {
		vis = d.KeepStates(vis, keep, sprint, c)
	}
	var tasks []*todo.Item
	for _, it := range d.Enumerate() {
		if vis[it] && it.Kind == todo.Task && !it.IsSubtask() {
			tasks = append(tasks, it)
		}
	}
	if len(tasks) == 0 {
		return helpTextStyle.Render("—")
	}
	height = max(height, 1)
	shown := tasks
	if len(tasks) > height {
		shown = tasks[:height-1]
	}
	lines := make([]string, 0, height)
	for _, it := range shown {
		glyph, glyphStyle := statusGlyph(it.Status, false)
		titleStyle := plainStyle
		if it.IsDone() {
			titleStyle = doneTitleStyle
		}
		row := glyphStyle.Render(glyph) + " " + highlight(todo.Printable(it.DisplayTitle()), v.filter, titleStyle)
		lines = append(lines, ansi.Truncate(row, width, "…"))
	}
	if more := len(tasks) - len(shown); more > 0 {
		lines = append(lines, ansi.Truncate(helpTextStyle.Render(fmt.Sprintf("… %d more", more)), width, "…"))
	}
	return strings.Join(lines, "\n")
}
