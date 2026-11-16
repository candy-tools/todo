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

// stepColumn returns the column delta tabs away from c, wrapping around the
// board in either direction.
func stepColumn(c todo.Column, delta int) todo.Column {
	cols := todo.Columns()
	for i, col := range cols {
		if col == c {
			return cols[((i+delta)%len(cols)+len(cols))%len(cols)]
		}
	}
	return todo.ColBacklog
}

// tabKey maps a main-view key to the tab it switches to: tab/shift+tab cycle
// from the current tab, 1–4 jump to a tab. ok is false for any other key.
func tabKey(key string, current todo.Column) (todo.Column, bool) {
	switch key {
	case "tab":
		return stepColumn(current, 1), true
	case "shift+tab":
		return stepColumn(current, -1), true
	}
	cols := todo.Columns()
	if len(key) == 1 && key[0] >= '1' && int(key[0]-'1') < len(cols) {
		return cols[key[0]-'1'], true
	}
	return 0, false
}

// tabBar renders the tab line under the header: every column's title and task
// count, the active one bracketed and accented, e.g.
// "Backlog 12   [Next 3]   In Progress 2   Done 40".
func tabBar(counts map[todo.Column]int, active todo.Column, width int) string {
	parts := make([]string, 0, len(todo.Columns()))
	for _, c := range todo.Columns() {
		label := fmt.Sprintf("%s %d", c.Title(), counts[c])
		if c == active {
			parts = append(parts, selectedRowStyle.Render("["+label+"]"))
		} else {
			parts = append(parts, headerHintSty.Render(label))
		}
	}
	return ansi.Truncate(" "+strings.Join(parts, "   "), width, "")
}

// allStatesBar renders the line that replaces the tab bar in the unified view,
// e.g. "All states · 12 tasks", so the header keeps its height.
func allStatesBar(total, width int) string {
	return ansi.Truncate(" "+selectedRowStyle.Render(fmt.Sprintf("All states · %d tasks", total)), width, "")
}
