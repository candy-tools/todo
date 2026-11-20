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

// barCells is the width of the progress bar in the board head, and
// minBarCells the narrowest it shrinks to so the readiness fits beside it.
const (
	barCells    = 20
	minBarCells = 8
)

// boardHead renders the lines above a sprint's tree, exactly boardChrome of
// them, width wide: its goal, its progress with whether it can be closed, and
// the tab bar.
func boardHead(s *todo.Sprint, counts map[todo.Column]int, active todo.Column, width int) []string {
	return []string{
		goalLine(s, width),
		progressLine(s, counts, width),
		ansi.Truncate("  "+tabBar(counts, active), width, ""),
	}
}

// goalLine renders the board head's goal line, width wide: the sprint's goal,
// or "—" when it has none, cut with "…".
func goalLine(s *todo.Sprint, width int) string {
	goal := todo.Printable(s.Goal)
	if goal == "" {
		goal = "—"
	}
	const goalLabel = "  Goal  "
	line := headerHintSty.Render(goalLabel) + plainStyle.Render(ansi.Truncate(goal, max(width-ansi.StringWidth(goalLabel), 0), "…"))
	return ansi.Truncate(line, width, "")
}

// progressLine renders the board head's progress line, width wide: the done
// tasks as a bar and a count, then whether the sprint can be closed. The bar
// shrinks, down to minBarCells, so the readiness reads in full; when it still
// does not fit, the readiness is cut with "…".
func progressLine(s *todo.Sprint, counts map[todo.Column]int, width int) string {
	total := 0
	for _, c := range todo.SprintColumns() {
		total += counts[c]
	}
	done := counts[todo.ColDone]

	// short is a shorter wording of the readiness, for a pane where the full
	// one would not fit beside the narrowest bar.
	ready, short, readyStyle := "", "", headerHintSty
	switch {
	case s.Status == todo.SprintDone:
		ready, short = "closed — c in the sidebar reopens it", "closed — c in sidebar reopens"
	case total == 0:
		ready = "no tasks yet"
	case done < total:
		ready = fmt.Sprintf("close blocked: %d not done", total-done)
	default:
		ready, short, readyStyle = "ready to close (c in the sidebar)", "ready to close — c in sidebar", doneStyle
	}

	const label = "  Done  "
	count := fmt.Sprintf(" %d/%d   ", done, total)
	fixed := ansi.StringWidth(label) + ansi.StringWidth(count)
	if short != "" && fixed+minBarCells+ansi.StringWidth(ready) > width {
		ready = short
	}
	cells := min(max(width-fixed-ansi.StringWidth(ready), minBarCells), barCells)
	filled := 0
	if total > 0 {
		filled = done * cells / total
	}
	bar := doneStyle.Render(strings.Repeat("█", filled)) + headerHintSty.Render(strings.Repeat("░", cells-filled))
	// What does not fit is the readiness's end (or, on a pane too narrow for
	// the bar, more): cut it with "…".
	return ansi.Truncate(headerHintSty.Render(label)+bar+count+readyStyle.Render(ready), width, "…")
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
		{active == screenPlan, "p", "Planning"},
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
