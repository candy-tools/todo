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

// stepColumn returns the board column delta columns away from c, wrapping
// around the board in either direction.
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

// tabKey maps a main-view key to the sprint board section it jumps to: ]
// and [ step from current, the section the cursor is in, 1–3 name a section.
// ok is false for any other key.
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

// jumpSection moves the board's cursor onto the first task of section c (see
// tabKey), scrolling the section to the top of the pane as far as the board
// allows; dir is the way ] (1) or [ (-1) steps, 0 for 1–3. A step skips the
// empty sections, wrapping around the board, and does nothing when every
// other one is empty; 1–3 on an empty section says so in the footer.
func (m *model) jumpSection(c todo.Column, dir int) {
	for range todo.SprintColumns() {
		if i, ok := m.tree.sectionStart(c); ok {
			m.tree.showSectionAt(i)
			return
		}
		if dir == 0 {
			m.status = "nothing in " + c.Title()
			return
		}
		c = stepColumn(c, dir)
	}
}

// boardColumn is the column of the board section the cursor is in, where a
// new task goes (see newTaskPlacement); Next when the tree shows no board.
func (m *model) boardColumn() todo.Column {
	if c, ok := m.tree.sectionAt(m.tree.cursor); ok {
		return c
	}
	return todo.ColNext
}

// boardChrome is how many lines the board pane's head takes above the tree:
// the goal, the progress and the rule that sets them apart from the tasks.
const boardChrome = 3

// barCells is the width of the progress bar in the board head, and
// minBarCells the narrowest it shrinks to so the readiness fits beside it.
const (
	barCells    = 20
	minBarCells = 8
)

// boardHead renders the lines above a sprint's tree, rows of them (at most
// boardChrome), width wide: its goal, its progress with whether it can be
// closed, and a rule under them. A short pane gives up the goal first, then
// the rule: the progress goes last.
func boardHead(s *todo.Sprint, counts map[todo.Column]int, width, rows int) []string {
	goal, progress := goalLine(s, width), progressLine(s, counts, width)
	switch {
	case rows >= boardChrome:
		return []string{goal, progress, headRule(width)}
	case rows == 2:
		return []string{progress, headRule(width)}
	case rows == 1:
		return []string{progress}
	}
	return nil
}

// headRule renders the rule under the board head, width wide.
func headRule(width int) string {
	return helpTextStyle.Render(strings.Repeat("─", max(width, 0)))
}

// goalLine renders the board head's goal line, width wide: the sprint's goal,
// or "—" when it has none, cut with "…".
func goalLine(s *todo.Sprint, width int) string {
	goal := todo.Printable(s.Goal)
	if goal == "" {
		goal = "—"
	}
	const goalLabel = "  Goal  "
	line := headLabelStyle.Render(goalLabel) + plainStyle.Bold(true).Render(ansi.Truncate(goal, max(width-ansi.StringWidth(goalLabel), 0), "…"))
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
	return ansi.Truncate(headLabelStyle.Render(label)+bar+count+readyStyle.Render(ready), width, "…")
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
