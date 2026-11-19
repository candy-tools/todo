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

// boardColumns are a sprint board's tabs, in order.
var boardColumns = []todo.Column{todo.ColNext, todo.ColInProgress, todo.ColDone}

// stepColumn returns the board tab delta tabs away from c, wrapping around the
// board in either direction.
func stepColumn(c todo.Column, delta int) todo.Column {
	n := len(boardColumns)
	for i, col := range boardColumns {
		if col == c {
			return boardColumns[((i+delta)%n+n)%n]
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
	if len(key) == 1 && key[0] >= '1' && int(key[0]-'1') < len(boardColumns) {
		return boardColumns[key[0]-'1'], true
	}
	return 0, false
}

// boardBar renders a sprint board's header line: the sprint's name, marked
// when the sprint is done, then the tab bar — every tab's title and the
// sprint's task count there, the active one bracketed and accented, e.g.
// "Sprint 13 · [Next 3]   In Progress 2   Done 4".
func boardBar(name string, done bool, counts map[todo.Column]int, active todo.Column, width int) string {
	head := todo.Printable(name)
	if done {
		head += " ✓ done"
	}
	parts := make([]string, 0, len(boardColumns))
	for _, c := range boardColumns {
		label := fmt.Sprintf("%s %d", c.Title(), counts[c])
		if c == active {
			parts = append(parts, selectedRowStyle.Render("["+label+"]"))
		} else {
			parts = append(parts, headerHintSty.Render(label))
		}
	}
	return ansi.Truncate(" "+categoryStyle.Render(head)+headerHintSty.Render(" · ")+strings.Join(parts, "   "), width, "")
}

// screenBar renders the header line of a screen without tabs, e.g.
// "Backlog · 12 tasks" or, in the unified view, "All states · 12 tasks", so
// the header keeps its height.
func screenBar(label string, width int) string {
	return ansi.Truncate(" "+selectedRowStyle.Render(label), width, "")
}
