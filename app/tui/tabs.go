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
// tab/shift+tab cycle from the current tab, 1–3 jump to a tab. ok is false for
// any other key.
func tabKey(key string, current todo.Column) (todo.Column, bool) {
	switch key {
	case "tab":
		return stepColumn(current, 1), true
	case "shift+tab":
		return stepColumn(current, -1), true
	}
	if cols := todo.SprintColumns(); len(key) == 1 && key[0] >= '1' && int(key[0]-'1') < len(cols) {
		return cols[key[0]-'1'], true
	}
	return 0, false
}

// boardBar renders a sprint board's header line: the sprint's name, marked
// when the sprint is done, and its goal when it has one, then the tab bar —
// every tab's title and the sprint's task count there, the active one
// bracketed and accented, e.g.
// "Sprint 13 · ship sprint support   [Next 3]   In Progress 2   Done 4".
// When the line is wider than width the goal is cut first, so the tab bar
// stays whole; the tab bar is cut only when even the name leaves no room.
func boardBar(name, goal string, done bool, counts map[todo.Column]int, active todo.Column, width int) string {
	head := todo.Printable(name)
	if done {
		head += " ✓ done"
	}
	tabs := tabBar(counts, active)
	const sep, goalSep = "   ", " · "
	line := " " + categoryStyle.Render(head)
	if goal = todo.Printable(goal); goal != "" {
		room := width - ansi.StringWidth(line) - ansi.StringWidth(goalSep) - ansi.StringWidth(sep) - ansi.StringWidth(tabs)
		if ansi.StringWidth(goal) > room {
			goal = ansi.Truncate(goal, room, "…")
		}
		// A goal cut to nothing but the ellipsis tells nothing: drop it.
		if room > 1 {
			line += headerHintSty.Render(goalSep) + plainStyle.Render(goal)
		}
	}
	return ansi.Truncate(line+sep+tabs, width, "")
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

// screenBar renders the header line of a screen without tabs, e.g.
// "Backlog · 12 tasks" or, in the unified view, "All states · 12 tasks", so
// the header keeps its height.
func screenBar(label string, width int) string {
	return ansi.Truncate(" "+selectedRowStyle.Render(label), width, "")
}
