package tui

import (
	"fmt"
	"strings"

	"github.com/candy-tools/todo/internal/todo"
	"github.com/charmbracelet/x/ansi"
)

// switchView moves the tree onto the view its viewState now describes, after
// the model changed it away from from (another tab, say). The selection (by
// item ID) is remembered under from for when that view is shown again; the new
// view gets back its own folds and selection, or no folds and the top row the
// first time (and when its remembered item is no longer shown there).
func (t *tree) switchView(from viewKey) {
	to := t.vs.key()
	if to == from {
		return
	}
	t.selByKey[from] = t.selectedID()
	t.useFolds(to)
	t.rebuild()
	if !t.selectID(t.selByKey[to]) {
		t.selectTop()
	}
}

// selectedID is the ID of the item under the cursor, or 0 when there is none
// (an empty tab, or the placeholder row).
func (t *tree) selectedID() string {
	if t.onPlaceholder() {
		return ""
	}
	if it := t.selected(); it != nil {
		return it.ID
	}
	return ""
}

// stepColumn returns the board tab delta tabs away from c, wrapping around the
// board in either direction.
func stepColumn(c todo.Column, delta int) todo.Column {
	cols := todo.SprintColumns()
	n := len(cols)
	for i, col := range cols {
		if col == c {
			return cols[((i+delta)%n+n)%n]
		}
	}
	return todo.ColNext
}

// tabKey maps a main-view key to the sprint board tab it switches to:
// ] and [ cycle from the current tab, 1–3 jump to a tab. ok is false for
// any other key.
func tabKey(key string, current todo.Column) (todo.Column, bool) {
	switch key {
	case "]":
		return stepColumn(current, 1), true
	case "[":
		return stepColumn(current, -1), true
	}
	if cols := todo.SprintColumns(); len(key) == 1 && key[0] >= '1' && int(key[0]-'1') < len(cols) {
		return cols[key[0]-'1'], true
	}
	return 0, false
}

// boardChrome is how many lines the board pane's head takes above the tree:
// the goal, the progress and the tab bar.
const boardChrome = 3

// barCells is the width of the progress bar in the board head.
const barCells = 20

// boardHead renders the lines above a sprint's tree, exactly boardChrome of
// them, width wide: its goal, its progress with whether it can be closed, and
// the tab bar.
func boardHead(s *todo.Sprint, counts map[todo.Column]int, active todo.Column, width int) []string {
	total := 0
	for _, c := range todo.SprintColumns() {
		total += counts[c]
	}
	done := counts[todo.ColDone]

	goal := todo.Printable(s.Goal)
	if goal == "" {
		goal = "—"
	}
	const goalLabel = "  Goal  "
	goalLine := headerHintSty.Render(goalLabel) + plainStyle.Render(ansi.Truncate(goal, max(width-ansi.StringWidth(goalLabel), 0), "…"))

	filled := 0
	if total > 0 {
		filled = done * barCells / total
	}
	bar := doneStyle.Render(strings.Repeat("█", filled)) + headerHintSty.Render(strings.Repeat("░", barCells-filled))
	var ready string
	switch {
	case s.Status == todo.SprintDone:
		ready = headerHintSty.Render("closed — c in the sidebar reopens it")
	case total == 0:
		ready = headerHintSty.Render("no tasks yet")
	case done < total:
		ready = headerHintSty.Render(fmt.Sprintf("close blocked: %d not done", total-done))
	default:
		ready = doneStyle.Render("ready to close (c in the sidebar)")
	}
	progress := headerHintSty.Render("  Done  ") + bar + fmt.Sprintf(" %d/%d   ", done, total) + ready

	return []string{
		ansi.Truncate(goalLine, width, ""),
		ansi.Truncate(progress, width, ""),
		ansi.Truncate("  "+tabBar(counts, active), width, ""),
	}
}

// tabBar renders a sprint board's tabs: every tab's title and the task count
// in counts, the active one bracketed and accented.
func tabBar(counts map[todo.Column]int, active todo.Column) string {
	cols := todo.SprintColumns()
	parts := make([]string, 0, len(cols))
	for _, c := range cols {
		label := fmt.Sprintf("%s %d", c.Title(), counts[c])
		if c == active {
			parts = append(parts, selectedRowStyle.Render("["+label+"]"))
		} else {
			parts = append(parts, headerHintSty.Render(label))
		}
	}
	return strings.Join(parts, "   ")
}

// switcherBar renders the header's second line: the workspace switcher, the
// active workspace bracketed and accented, then the breadcrumb, e.g.
// " b Backlog   [s Sprints]   p Planning   Sprints › A › Next".
func switcherBar(active screen, crumb string, width int) string {
	entries := []struct {
		on         bool
		key, label string
	}{
		{active == screenBacklog, "b", "Backlog"},
		{active == screenSprint, "s", "Sprints"},
		{false, "p", "Planning"}, // no Planning screen yet: never the active one
	}
	var b strings.Builder
	b.WriteString(" ")
	for _, e := range entries {
		if e.on {
			b.WriteString(selectedRowStyle.Render("[" + e.key + " " + e.label + "]"))
		} else {
			b.WriteString(headerHintSty.Render(e.key + " " + e.label))
		}
		b.WriteString("   ")
	}
	b.WriteString(plainStyle.Render(crumb))
	return ansi.Truncate(b.String(), width, "")
}
