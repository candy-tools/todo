package tui

import (
	"fmt"
	"strings"

	"github.com/candy-tools/todo/internal/todo"
)

// treeRow is one visible line of the task tree: an item and its nesting depth.
// The placeholder flag marks the synthetic "+ new category" row.
type treeRow struct {
	item        *todo.Item
	depth       int
	placeholder bool
}

// tree is the left-pane category/task tree: the document, per-item collapse
// state, and the cursor over the currently visible rows. rows is derived from
// doc + collapsed by rebuild and cached for cursor math and rendering. A
// synthetic placeholder row is always appended at root level so a new
// top-level category can always be added (otherwise a category's `c` only ever
// makes a subcategory, leaving no way to add a second root item).
type tree struct {
	doc         *todo.Document
	collapsed   map[*todo.Item]bool
	cursor      int
	rows        []treeRow
	placeholder *todo.Item // UI-only sentinel backing the "+ new category" row
	// offset is the index of the first row shown in the left-pane viewport — the
	// scroll position, kept decoupled from the cursor. The window follows the
	// cursor only when the cursor would leave it (see reconcileOffset), so moving
	// the cursor within the page moves the pointer, not the whole page.
	offset int
	// viewHeight is how many rows the left-pane viewport can show, set from the
	// terminal size. reconcileOffset needs it to know the window bounds.
	viewHeight int
	// filter is the active search query. When set, rebuild shows only the items
	// on a path to a match (see todo.VisibleItems), ignores the collapse state so
	// matches inside collapsed parents surface, and drops the placeholder row.
	filter string
	// grabbed is true while the item under the cursor is "picked up" for moving:
	// the arrow keys reorder/re-nest it instead of navigating, and its row is
	// drawn with the grabbed marker (see rowString).
	grabbed bool
}

func newTree(doc *todo.Document) tree {
	t := tree{doc: doc, collapsed: map[*todo.Item]bool{}, placeholder: &todo.Item{}}
	t.rebuild()
	return t
}

// rebuild recomputes the visible rows from the document and collapse state,
// clamping the cursor into range. Unfiltered it ends with the root-level
// placeholder row; while a filter is active it shows only the matching paths
// (collapse state ignored) and omits the placeholder. Call it after any
// structural change.
func (t *tree) rebuild() {
	t.rows = t.rows[:0]
	var visible map[*todo.Item]bool
	if t.filter != "" {
		visible = t.doc.VisibleItems(t.filter)
	}
	filtering := visible != nil
	var walk func(items []*todo.Item, depth int)
	walk = func(items []*todo.Item, depth int) {
		for _, it := range items {
			if filtering && !visible[it] {
				continue
			}
			t.rows = append(t.rows, treeRow{item: it, depth: depth})
			// While filtering, descend regardless of collapse so a match inside a
			// collapsed parent is still shown.
			if len(it.Children) > 0 && (filtering || !t.collapsed[it]) {
				walk(it.Children, depth+1)
			}
		}
	}
	walk(t.doc.Roots, 0)
	if !filtering {
		t.rows = append(t.rows, treeRow{item: t.placeholder, depth: 0, placeholder: true})
	}
	if t.cursor >= len(t.rows) {
		t.cursor = len(t.rows) - 1
	}
	if t.cursor < 0 {
		t.cursor = 0
	}
	t.reconcileOffset()
}

// setFilter applies a search query (empty clears it) and rebuilds, resetting the
// cursor to the first row so results start at the top.
func (t *tree) setFilter(q string) {
	t.filter = q
	t.cursor = 0
	t.rebuild()
}

// selected returns the item under the cursor. On the placeholder row it returns
// the sentinel — callers guard with onPlaceholder before treating it as a real
// item.
func (t *tree) selected() *todo.Item {
	if t.cursor < 0 || t.cursor >= len(t.rows) {
		return nil
	}
	return t.rows[t.cursor].item
}

// onPlaceholder reports whether the cursor is on the "+ new category" row.
func (t *tree) onPlaceholder() bool {
	return t.cursor >= 0 && t.cursor < len(t.rows) && t.rows[t.cursor].placeholder
}

// selectItem moves the cursor onto it, if it is currently visible, scrolling the
// viewport as needed to keep it on screen.
func (t *tree) selectItem(it *todo.Item) {
	for i, r := range t.rows {
		if r.item == it {
			t.cursor = i
			t.reconcileOffset()
			return
		}
	}
}

// revealItem expands every ancestor of it (so a collapsed fold can't hide it),
// rebuilds, and moves the cursor onto it. Used when returning from a filtered
// view — where the collapse state is ignored — so the focused item stays selected
// even when it lives inside a fold.
func (t *tree) revealItem(it *todo.Item) {
	for p := it.Parent; p != nil; p = p.Parent {
		delete(t.collapsed, p)
	}
	t.rebuild()
	t.selectItem(it)
}

// selectTop moves the cursor to the first row and scrolls the viewport back to
// the top — the fallback when a prior selection no longer exists.
func (t *tree) selectTop() {
	t.cursor = 0
	t.reconcileOffset()
}

// navStep is how many rows Page Up / Page Down jump per press: a fixed stride
// that travels faster than the single-row ↑/↓, independent of the window height.
const navStep = 10

func (t *tree) moveUp()   { t.moveBy(-1) }
func (t *tree) moveDown() { t.moveBy(1) }

// pageUp / pageDown jump navStep rows at once (clamped to the ends), for quick
// travel through a long list.
func (t *tree) pageUp()   { t.moveBy(-navStep) }
func (t *tree) pageDown() { t.moveBy(navStep) }

// moveBy shifts the cursor by delta rows (negative up, positive down), clamped
// to the visible range, then scrolls the viewport just enough to follow it.
func (t *tree) moveBy(delta int) {
	t.cursor += delta
	if last := len(t.rows) - 1; t.cursor > last {
		t.cursor = last
	}
	if t.cursor < 0 {
		t.cursor = 0
	}
	t.reconcileOffset()
}

// reconcileOffset scrolls the viewport just enough to keep the cursor visible,
// leaving it put otherwise. This is what decouples the scroll position from the
// cursor: the window shifts only when the cursor moves out of it (rather than
// pinning the cursor to an edge and dragging the whole page along), so stepping
// the cursor within the page moves the pointer alone.
func (t *tree) reconcileOffset() {
	h := t.viewHeight
	if h < 1 {
		h = 1
	}
	switch {
	case t.cursor < t.offset: // cursor left above the window: scroll up to it
		t.offset = t.cursor
	case t.cursor >= t.offset+h: // cursor left below the window: scroll down to it
		t.offset = t.cursor - h + 1
	}
	// Keep the window within the list: don't scroll past the last row (which would
	// leave blank space below), nor above the first.
	if maxOffset := len(t.rows) - h; t.offset > maxOffset {
		t.offset = maxOffset
	}
	if t.offset < 0 {
		t.offset = 0
	}
}

// toggleFold flips the collapse state of the selected item (no-op on a leaf).
func (t *tree) toggleFold() {
	it := t.selected()
	if it == nil || len(it.Children) == 0 {
		return
	}
	t.collapsed[it] = !t.collapsed[it]
	t.rebuild()
}

// collapse folds an expanded parent; on a leaf or already-folded item it jumps
// to the parent instead — the familiar file-tree Left-arrow behaviour.
func (t *tree) collapse() {
	it := t.selected()
	if it == nil {
		return
	}
	if len(it.Children) > 0 && !t.collapsed[it] {
		t.collapsed[it] = true
		t.rebuild()
		return
	}
	if it.Parent != nil {
		t.selectItem(it.Parent)
	}
}

// expand unfolds a folded parent (no-op otherwise).
func (t *tree) expand() {
	it := t.selected()
	if it == nil {
		return
	}
	if len(it.Children) > 0 && t.collapsed[it] {
		t.collapsed[it] = false
		t.rebuild()
	}
}

// view renders the visible window of rows to fit width x height, starting from
// the persistent scroll offset (maintained by reconcileOffset) rather than
// re-deriving it from the cursor, so the page doesn't slide when the cursor
// moves within it. The offset is clamped here defensively in case this height
// differs from the one it was reconciled against (e.g. before the first resize).
func (t *tree) view(width, height int) string {
	if height < 1 {
		height = 1
	}
	start := t.offset
	if start > t.cursor {
		start = t.cursor
	}
	if start < t.cursor-height+1 {
		start = t.cursor - height + 1
	}
	if maxStart := len(t.rows) - height; start > maxStart {
		start = maxStart
	}
	if start < 0 {
		start = 0
	}
	end := start + height
	if end > len(t.rows) {
		end = len(t.rows)
	}
	var b strings.Builder
	for i := start; i < end; i++ {
		if i > start {
			b.WriteByte('\n')
		}
		b.WriteString(t.rowString(t.rows[i], i == t.cursor))
	}
	return b.String()
}

// rowString renders one row. A selected row is drawn wholly in the accent
// colour behind a "❯ " gutter; unselected rows keep their per-token styling
// (purple categories, green ticks, struck-through done titles) behind a
// matching two-space gutter so columns line up. A task that has a description
// carries a trailing "≡" marker.
func (t *tree) rowString(r treeRow, selected bool) string {
	if r.placeholder {
		if selected {
			return selectedRowStyle.Render("❯ + new category")
		}
		return "  " + helpTextStyle.Render("+ new category")
	}

	indent := strings.Repeat("  ", r.depth)
	fold := "  "
	if len(r.item.Children) > 0 {
		fold = "▾ "
		// A filtered view expands everything, so never show a collapsed caret.
		if t.collapsed[r.item] && t.filter == "" {
			fold = "▸ "
		}
	}

	// A selected row is drawn whole in the accent colour behind a "❯ " gutter;
	// while it is grabbed for moving it switches to the reverse-video grabbed
	// style and a "⇕ " gutter so the picked-up item is unmistakable.
	selStyle, gutter := selectedRowStyle, "❯ "
	if selected && t.grabbed {
		selStyle, gutter = grabbedRowStyle, "⇕ "
	}

	if r.item.Kind == todo.Category {
		title := r.item.Title
		counts := ""
		if done, total := r.item.TaskCounts(); total > 0 {
			counts = fmt.Sprintf(" (%d/%d)", done, total)
		}
		if selected {
			return selStyle.Render(gutter+indent+fold) +
				highlight(title, t.filter, selStyle) + selStyle.Render(counts)
		}
		return "  " + indent + helpTextStyle.Render(fold) +
			highlight(title, t.filter, categoryStyle) + helpTextStyle.Render(counts)
	}

	// Task. The glyph and its colour follow the status: open is an empty circle,
	// in progress a half-filled circle, deferred a coloured pause, done a filled
	// green circle with a struck-through dim title.
	glyph, boxStyle, titleStyle := "○", plainStyle, plainStyle
	switch r.item.Status {
	case todo.InProgress:
		glyph, boxStyle = "◐", progStyle
	case todo.Deferred:
		glyph, boxStyle = "⏸", deferStyle
	case todo.Done:
		glyph, boxStyle, titleStyle = "●", doneStyle, doneTitleStyle
	}
	// A task with a description gets a trailing ≡ marker, so the single-panel list
	// still shows at a glance which items have details to open (enter).
	note := ""
	if r.item.Description != "" {
		note = " ≡"
	}
	if selected {
		return selStyle.Render(gutter+indent+fold+glyph+" ") + highlight(r.item.Title, t.filter, selStyle) + selStyle.Render(note)
	}
	return "  " + indent + helpTextStyle.Render(fold) + boxStyle.Render(glyph) + " " + highlight(r.item.Title, t.filter, titleStyle) + helpTextStyle.Render(note)
}
