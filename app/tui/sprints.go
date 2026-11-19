package tui

import (
	"fmt"
	"slices"
	"strings"

	"github.com/candy-tools/todo/internal/todo"
	"github.com/charmbracelet/x/ansi"
)

// sprintList is the Sprint list screen: one row per sprint, and the cursor
// over them. The rows are derived from the list on every use, so a reload
// needs nothing more than a clamp of the cursor.
type sprintList struct {
	cursor int
}

// maxGoalWidth caps the goal column, so a long goal never pushes the counts
// off the row.
const maxGoalWidth = 40

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

// move shifts the cursor by delta rows, clamped to the list.
func (l *sprintList) move(d *todo.Document, delta int) {
	l.cursor += delta
	l.clamp(len(l.rows(d)))
}

// selected returns the sprint under the cursor, or nil when d has none.
func (l *sprintList) selected(d *todo.Document) *todo.Sprint {
	rows := l.rows(d)
	l.clamp(len(rows))
	if len(rows) == 0 {
		return nil
	}
	return rows[l.cursor]
}

// selectID puts the cursor on the sprint with the given ID, when d has it.
func (l *sprintList) selectID(d *todo.Document, id string) {
	if i := slices.IndexFunc(l.rows(d), func(s *todo.Sprint) bool { return s.ID == id }); i >= 0 {
		l.cursor = i
	}
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
	l.clamp(len(rows))
	names, goals := make([]string, len(rows)), make([]string, len(rows))
	nameW, goalW := 0, 0
	for i, s := range rows {
		names[i] = todo.Printable(s.Name)
		goals[i] = ansi.Truncate(todo.Printable(s.Goal), maxGoalWidth, "…")
		nameW = max(nameW, ansi.StringWidth(names[i]))
		goalW = max(goalW, ansi.StringWidth(goals[i]))
	}
	height = max(height, 1)
	start := max(l.cursor-height+1, 0)
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
