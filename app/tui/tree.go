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

// tree is the left-pane category/task tree: the document, the board column
// (tab) it shows, per-item collapse state (keyed by item ID, so it survives a
// reload), and the cursor over the currently visible rows. rows is derived from
// doc + column + collapsed by rebuild and cached for cursor math and rendering.
// On the Backlog tab a synthetic placeholder row is appended at root level so a
// new top-level category can always be added (otherwise a category's `c` only
// ever makes a subcategory, leaving no way to add a second root item).
type tree struct {
	doc *todo.Document
	// column is the board tab shown: only its tasks (with their subtasks and
	// category paths) are rows. See setColumn.
	column todo.Column
	// collapsed is the active tab's fold map; it is the same map as
	// foldsByCol[column]. Each tab keeps its own folds and selection for the
	// session: foldsByCol and selByCol (the selected item's ID) hold them per
	// column.
	collapsed   map[uint]bool
	foldsByCol  map[todo.Column]map[uint]bool
	selByCol    map[todo.Column]uint
	shown       map[*todo.Item]bool // the items the tab shows, set by rebuild (folds aside)
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
	// filter is the active search query. It is kept across tab switches. When
	// set, rebuild shows only the tab's items on a path to a match (see
	// visibleItems), ignores the collapse state so matches inside collapsed
	// parents surface, and drops the placeholder row.
	filter string
	// grabbed is true while the item under the cursor is "picked up" for moving:
	// the arrow keys reorder/re-nest it instead of navigating, and its row is
	// drawn with the grabbed marker (see rowString).
	grabbed bool
}

// newTree builds the tree for doc on the Backlog tab, the one the app opens on.
func newTree(doc *todo.Document) tree {
	folds := map[uint]bool{}
	t := tree{
		doc:         doc,
		column:      todo.ColBacklog,
		collapsed:   folds,
		foldsByCol:  map[todo.Column]map[uint]bool{todo.ColBacklog: folds},
		selByCol:    map[todo.Column]uint{},
		placeholder: &todo.Item{},
	}
	t.rebuild()
	return t
}

// searching reports whether a search query filters the rows. A blank query
// (spaces only) filters nothing, as in todo.VisibleItems.
func (t *tree) searching() bool { return strings.TrimSpace(t.filter) != "" }

// shownBefore counts the items the tab showed at the last rebuild that come
// before it in document order. A move that changes the count has carried it
// past a shown row.
func (t *tree) shownBefore(it *todo.Item) int {
	n := 0
	for _, x := range t.doc.Enumerate() {
		if x == it {
			break
		}
		if t.shown[x] {
			n++
		}
	}
	return n
}

// visibleItems returns the items the current tab shows: the column's items
// (todo.ColumnItems) and, while searching, only those on a path to a match that
// is itself in the tab. Matching only in-tab items keeps a search from showing
// a Backlog category whose sole match is a task in another column.
func (t *tree) visibleItems() map[*todo.Item]bool {
	inTab := t.doc.ColumnItems(t.column)
	if !t.searching() {
		return inTab
	}
	vis := t.doc.VisibleWhere(func(it *todo.Item) bool { return inTab[it] && it.Matches(t.filter) })
	for it := range vis {
		if !inTab[it] {
			delete(vis, it)
		}
	}
	return vis
}

// foldable reports whether it has a child the tab shows (folded or not), so it
// can fold. A Backlog category whose tasks all sit in other columns has none.
func (t *tree) foldable(it *todo.Item) bool {
	for _, c := range it.Children {
		if t.shown[c] {
			return true
		}
	}
	return false
}

// rebuild recomputes the visible rows from the document, the tab and the
// collapse state, clamping the cursor into range. Without a search, the
// Backlog tab ends with the root-level placeholder row; while searching only
// the matching paths show (collapse state ignored) and the placeholder is
// omitted. An empty tab has no rows at all. Call it after any structural
// change.
func (t *tree) rebuild() {
	t.rows = t.rows[:0]
	t.shown = t.visibleItems()
	searching := t.searching()
	var walk func(items []*todo.Item, depth int)
	walk = func(items []*todo.Item, depth int) {
		for _, it := range items {
			if !t.shown[it] {
				continue
			}
			t.rows = append(t.rows, treeRow{item: it, depth: depth})
			// While searching, descend regardless of collapse so a match inside a
			// collapsed parent is still shown.
			if len(it.Children) > 0 && (searching || !t.collapsed[it.ID]) {
				walk(it.Children, depth+1)
			}
		}
	}
	walk(t.doc.Roots, 0)
	if t.column == todo.ColBacklog && !searching {
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

// selectID moves the cursor onto the visible item with the given ID, scrolling
// the viewport as needed; it reports whether one was found. ID 0 (an item not in
// the store, such as the placeholder sentinel) never matches.
func (t *tree) selectID(id uint) bool {
	if id == 0 {
		return false
	}
	for i, r := range t.rows {
		if !r.placeholder && r.item.ID == id {
			t.cursor = i
			t.reconcileOffset()
			return true
		}
	}
	return false
}

// adopt swaps in doc, a fresh load of the same list, and rebuilds. Every tab's
// folds carry over by ID for the items doc still holds; the tab, the viewport,
// the filter and the grab state are kept. The cursor moves onto focusID when it
// is visible in the tab, else back onto the placeholder row when onPlaceholder
// (and no search hides it), else it keeps its index, clamped into range — so
// a task that left the tab hands the cursor to the nearest remaining row.
func (t *tree) adopt(doc *todo.Document, focusID uint, onPlaceholder bool) {
	t.doc = doc
	present := map[uint]bool{}
	for _, it := range doc.Enumerate() {
		present[it.ID] = true
	}
	for _, folds := range t.foldsByCol {
		for id := range folds {
			if !present[id] {
				delete(folds, id)
			}
		}
	}
	t.rebuild()
	if t.selectID(focusID) {
		return
	}
	if onPlaceholder && !t.searching() {
		t.cursor = len(t.rows) - 1
		t.reconcileOffset()
	}
}

// revealItem expands every ancestor of it (so a collapsed fold can't hide it),
// rebuilds, and moves the cursor onto it. Used when returning from a filtered
// view — where the collapse state is ignored — so the focused item stays selected
// even when it lives inside a fold.
func (t *tree) revealItem(it *todo.Item) {
	for p := it.Parent; p != nil; p = p.Parent {
		delete(t.collapsed, p.ID)
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
	if it == nil || !t.foldable(it) {
		return
	}
	t.collapsed[it.ID] = !t.collapsed[it.ID]
	t.rebuild()
}

// collapse folds an expanded parent; on a leaf or already-folded item it jumps
// to the parent instead — the familiar file-tree Left-arrow behaviour.
func (t *tree) collapse() {
	it := t.selected()
	if it == nil {
		return
	}
	if t.foldable(it) && !t.collapsed[it.ID] {
		t.collapsed[it.ID] = true
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
	if t.foldable(it) && t.collapsed[it.ID] {
		t.collapsed[it.ID] = false
		t.rebuild()
	}
}

// view renders the visible window of rows to fit width x height, starting from
// the persistent scroll offset (maintained by reconcileOffset) rather than
// re-deriving it from the cursor, so the page doesn't slide when the cursor
// moves within it. The offset is clamped here defensively in case this height
// differs from the one it was reconciled against (e.g. before the first resize).
// An empty tab renders a single dim "nothing in <tab>" line, or "no matches in
// <tab>" while a search finds nothing there.
func (t *tree) view(width, height int) string {
	if len(t.rows) == 0 {
		empty := "nothing in "
		if t.searching() {
			empty = "no matches in "
		}
		return "  " + helpTextStyle.Render(empty+t.column.Title())
	}
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
	if t.foldable(r.item) {
		fold = "▾ "
		// A filtered view expands everything, so never show a collapsed caret.
		if t.collapsed[r.item.ID] && !t.searching() {
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
		// Backlog shows the category's overall progress; the other tabs count
		// the category's tasks in that tab.
		counts := fmt.Sprintf(" (%d)", r.item.CountInColumn(t.column))
		if t.column == todo.ColBacklog {
			counts = ""
			if done, total := r.item.TaskCounts(); total > 0 {
				counts = fmt.Sprintf(" (%d/%d)", done, total)
			}
		}
		if selected {
			return selStyle.Render(gutter+indent+fold) +
				highlight(title, t.filter, selStyle) + selStyle.Render(counts)
		}
		return "  " + indent + helpTextStyle.Render(fold) +
			highlight(title, t.filter, categoryStyle) + helpTextStyle.Render(counts)
	}

	// Task. The glyph and its colour follow the status (see statusGlyph); a done
	// task's title is struck through and dim. A top-level task names its state
	// after the title, followed by its subtask progress when it has subtasks,
	// and a ready task without a plan carries a warning.
	subtask := r.item.IsSubtask()
	glyph, boxStyle := statusGlyph(r.item.Status, subtask)
	titleStyle := plainStyle
	if r.item.IsDone() {
		titleStyle = doneTitleStyle
	}
	state := ""
	if !subtask {
		state = " " + r.item.Status.String()
		if done, total := r.item.TaskCounts(); total > 0 {
			state += fmt.Sprintf(" (%d/%d)", done, total)
		}
	}
	warn := ""
	if r.item.NeedsPlan() {
		warn = " ⚠ no plan"
	}
	// A task with a description gets a trailing ≡ marker, so the single-panel list
	// still shows at a glance which items have details to open (enter).
	note := ""
	if r.item.Description != "" {
		note = " ≡"
	}
	if selected {
		return selStyle.Render(gutter+indent+fold+glyph+" ") + highlight(r.item.Title, t.filter, selStyle) + selStyle.Render(state+warn+note)
	}
	return "  " + indent + helpTextStyle.Render(fold) + boxStyle.Render(glyph) + " " + highlight(r.item.Title, t.filter, titleStyle) +
		helpTextStyle.Render(state) + deferStyle.Render(warn) + helpTextStyle.Render(note)
}
