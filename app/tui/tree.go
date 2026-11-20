package tui

import (
	"fmt"
	"strings"

	"github.com/candy-tools/todo/internal/todo"
	"github.com/charmbracelet/x/ansi"
)

// rowKind tells a tree row backed by an item from the synthetic ones.
type rowKind int

const (
	rowItem        rowKind = iota // a category or task of the document
	rowNewCategory                // the "+ new category" row
	rowAll                        // the sidebar's "All" row: no category picked
	rowHeader                     // a category drawn as a group header in a task list
	rowSection                    // a sprint board's section: its column's title and task count
)

// heading reports whether rows of kind k are headings — a category header or
// a board section — which the cursor never rests on.
func (k rowKind) heading() bool { return k == rowHeader || k == rowSection }

// paneKind is what a tree shows: today's mixed category/task tree, only the
// categories (the split view's sidebar), or only the tasks of one category
// with its subcategories as group headers (the split view's task list).
type paneKind int

const (
	paneMixed      paneKind = iota // categories and tasks in one tree
	paneCategories                 // the sidebar: All, the category tree, + new category
	paneTasks                      // the task list of the scope category
)

// treeRow is one visible line of the task tree: an item and its nesting depth,
// and what kind of row it is. label is a header row's text; col and count are
// a section row's column and how many top-level tasks it shows.
type treeRow struct {
	item  *todo.Item
	depth int
	kind  rowKind
	label string
	col   todo.Column
	count int
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
	// vs is what the tree shows the list through — the board tab, the
	// search query and the state filter — shared with the model and every
	// other tree (see viewState).
	vs *viewState
	// collapsed is the current view's fold map; it is the same map as
	// foldsByKey[view.key()]. Each view keeps its own folds and selection for
	// the session: foldsByKey and selByKey (the selected item's ID) hold them.
	collapsed   map[string]bool
	foldsByKey  map[viewKey]map[string]bool
	selByKey    map[viewKey]string
	shown       map[*todo.Item]bool // the items the view shows, set by rebuild (folds aside)
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
	// pane is what the tree shows (see paneKind); scope is the category whose
	// tasks a paneTasks tree lists, with its subcategories' (0 = every
	// category, the sidebar's All).
	pane  paneKind
	scope string
	// grabbed is true while the item under the cursor is "picked up" for moving:
	// the arrow keys reorder/re-nest it instead of navigating, and its row is
	// drawn with the grabbed marker (see rowString).
	grabbed bool
}

// newTree builds the tree for doc, shown through v.
func newTree(doc *todo.Document, v *viewState) tree {
	folds := map[string]bool{}
	t := tree{
		doc:         doc,
		vs:          v,
		collapsed:   folds,
		foldsByKey:  map[viewKey]map[string]bool{v.key(): folds},
		selByKey:    map[viewKey]string{},
		placeholder: &todo.Item{},
	}
	t.rebuild()
	return t
}

// newSidebar builds the split view's sidebar for doc, shown through v.
func newSidebar(doc *todo.Document, v *viewState) tree {
	t := newTree(doc, v)
	t.setPane(paneCategories)
	t.selectTop()
	return t
}

// useFolds makes the folds of view key k the tree's current ones.
func (t *tree) useFolds(k viewKey) {
	folds, ok := t.foldsByKey[k]
	if !ok {
		folds = map[string]bool{}
		t.foldsByKey[k] = folds
	}
	t.collapsed = folds
}

// filtering reports whether the rows are narrowed, by a search query or by
// the state filter. A filtered view ignores folds and drops the placeholder
// row.
func (t *tree) filtering() bool { return t.vs.filtering() }

// shownID returns a predicate that reports whether the tab showed the item
// with a given ID at the last rebuild, for a move step to skip the items it
// hides (see todo.Document.StepPlacement).
func (t *tree) shownID() func(id string) bool {
	ids := make(map[string]bool, len(t.shown))
	for it := range t.shown {
		ids[it.ID] = true
	}
	return func(id string) bool { return ids[id] }
}

// visibleItems returns the items the current view shows: the column's items
// (on a sprint board, the board's sprint's only) and, while searching, only
// those on a path to a match that is itself in the view (see
// todo.Document.ColumnMatches), narrowed to the state filter when it is on for
// the view (see todo.Document.KeepStates).
func (t *tree) visibleItems() map[*todo.Item]bool {
	cols, sprint := t.vs.columns(), t.vs.scopeSprint()
	vis := t.doc.ColumnMatches(t.vs.filter, sprint, cols...)
	if active := t.vs.activeStates(); len(active) > 0 {
		keep := map[todo.Status]bool{}
		for _, st := range active {
			keep[st] = true
		}
		vis = t.doc.KeepStates(vis, keep, sprint, cols...)
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
	vis := t.visibleItems()
	switch t.pane {
	case paneCategories:
		t.rebuildCategories(vis)
	case paneTasks:
		t.rebuildTasks(vis)
	case paneMixed:
		if t.vs.onBoard() && t.vs.sprint != "" {
			t.rebuildBoard(vis)
			break
		}
		t.shown = vis
		t.walk(t.doc.Roots, 0, nil)
		if t.vs.editsCategories() && !t.filtering() {
			t.rows = append(t.rows, treeRow{item: t.placeholder, depth: 0, kind: rowNewCategory})
		}
	}
	if t.cursor >= len(t.rows) {
		t.cursor = len(t.rows) - 1
	}
	if t.cursor < 0 {
		t.cursor = 0
	}
	t.settle(1)
	t.reconcileOffset()
}

// walk appends a row for every shown item of items, at depth, descending into
// unfolded ones — or every one while filtering, so a match inside a collapsed
// parent is still shown. keep, when set, limits which items get a row (their
// children are still walked when they are kept).
func (t *tree) walk(items []*todo.Item, depth int, keep func(*todo.Item) bool) {
	for _, it := range items {
		if !t.shown[it] || (keep != nil && !keep(it)) {
			continue
		}
		t.rows = append(t.rows, treeRow{item: it, depth: depth})
		if len(it.Children) > 0 && (t.filtering() || !t.collapsed[it.ID]) {
			t.walk(it.Children, depth+1, keep)
		}
	}
}

// rebuildCategories lays out the sidebar: the All row, the categories of the
// visible set as a foldable tree, and — where categories are edited and
// nothing is filtered — the "+ new category" row. Only categories count as
// shown, so grab-moving a category steps past one shown category at a time.
func (t *tree) rebuildCategories(vis map[*todo.Item]bool) {
	t.shown = map[*todo.Item]bool{}
	for it := range vis {
		if it.Kind == todo.Category {
			t.shown[it] = true
		}
	}
	t.rows = append(t.rows, treeRow{kind: rowAll})
	t.walk(t.doc.Roots, 0, nil)
	if t.vs.editsCategories() && !t.filtering() {
		t.rows = append(t.rows, treeRow{item: t.placeholder, depth: 0, kind: rowNewCategory})
	}
}

// scopeItem returns the scope category, or nil for All (or a scope that is no
// longer in the document).
func (t *tree) scopeItem() *todo.Item {
	if t.scope == "" {
		return nil
	}
	it, ok := t.doc.ByID(t.scope)
	if !ok || it.Kind != todo.Category {
		return nil
	}
	return it
}

// rebuildTasks lays out the task list of the scope category: its own tasks,
// then each subcategory (recursively) as a header row followed by its tasks.
// A header's label is the category's path below the scope; a category with
// no shown task in its subtree gets no header. Only items inside the scope
// count as shown, so a task moved out of it has left the pane.
func (t *tree) rebuildTasks(vis map[*todo.Item]bool) {
	roots := t.doc.Roots
	if sc := t.scopeItem(); sc != nil {
		roots = sc.Children
	}
	t.shown = map[*todo.Item]bool{}
	var mark func(items []*todo.Item)
	mark = func(items []*todo.Item) {
		for _, it := range items {
			if vis[it] {
				t.shown[it] = true
			}
			mark(it.Children)
		}
	}
	mark(roots)
	t.layTasks(roots, t.scopeItem(), nil, nil)
}

// layTasks appends the rows of the shown tasks among items, owned by
// category owner (nil at the root) at path below the scope, and of those in
// its subcategories, recursively; keep, when set, limits which top-level
// tasks get rows. The rows keep document order, where a category's
// subcategories precede its tasks, so a category's header goes right before
// its own run of tasks, labelled with its path from the scope. The scope's
// own tasks need their header only after other rows, to tell them apart. It
// returns how many top-level tasks it laid out.
func (t *tree) layTasks(items []*todo.Item, owner *todo.Item, path []string, keep func(*todo.Item) bool) (n int) {
	headed := false
	for _, it := range items {
		if !t.shown[it] {
			continue
		}
		if it.Kind == todo.Category {
			n += t.layTasks(it.Children, it, append(path[:len(path):len(path)], it.Title), keep)
			continue
		}
		if keep != nil && !keep(it) {
			continue
		}
		if !headed && owner != nil && (len(path) > 0 || len(t.rows) > 0) {
			label := owner.Title
			if len(path) > 0 {
				label = strings.Join(path, " › ")
			}
			t.rows = append(t.rows, treeRow{item: owner, kind: rowHeader, label: label})
		}
		headed = true
		n++
		t.walk([]*todo.Item{it}, 0, nil)
	}
	return n
}

// rebuildBoard lays out a sprint board: for each of its columns, a section
// row, then that column's top-level tasks of the shown set (with their
// subtasks) grouped under their category paths, as the split view's task list
// groups them. Every section shows, an empty one as its row alone, so the
// board keeps its shape — unless no section holds a task: then there are no
// rows, and view says the sprint is empty (or that nothing matches). A
// category can head a run of tasks in several
// sections, so categories are never rows of their own here: only tasks are
// selected and folded.
func (t *tree) rebuildBoard(vis map[*todo.Item]bool) {
	t.shown = vis
	tasks := 0
	for _, c := range todo.SprintColumns() {
		at := len(t.rows)
		t.rows = append(t.rows, treeRow{kind: rowSection, col: c})
		t.rows[at].count = t.layTasks(t.doc.Roots, nil, nil, func(it *todo.Item) bool { return it.Column() == c })
		tasks += t.rows[at].count
	}
	if tasks == 0 {
		t.rows = t.rows[:0]
	}
}

// sectionAt returns the column of the board section holding row i: the
// nearest section row at or above it. ok is false off a board.
func (t *tree) sectionAt(i int) (c todo.Column, ok bool) {
	for i = min(i, len(t.rows)-1); i >= 0; i-- {
		if t.rows[i].kind == rowSection {
			return t.rows[i].col, true
		}
	}
	return 0, false
}

// showSectionAt puts the cursor on row i, the first task of a board section,
// with the window starting at the section's headings (see reconcileOffset),
// as far as the rows below fill it.
func (t *tree) showSectionAt(i int) {
	t.cursor, t.offset = i, i
	t.reconcileOffset()
}

// sectionStart returns the index of the first task row of board section c,
// or false when the section is empty or the tree shows no board.
func (t *tree) sectionStart(c todo.Column) (int, bool) {
	in := false
	for i, r := range t.rows {
		switch {
		case r.kind == rowSection:
			in = r.col == c
		case in && !r.kind.heading():
			return i, true
		}
	}
	return 0, false
}

// settle moves the cursor off a heading row — headings are never selected —
// onto the nearest other row in direction dir (±1), or the other way when
// there is none that way. With no other row at all it stays.
func (t *tree) settle(dir int) {
	if t.cursor < 0 || t.cursor >= len(t.rows) || !t.rows[t.cursor].kind.heading() {
		return
	}
	for _, d := range []int{dir, -dir} {
		for i := t.cursor; i >= 0 && i < len(t.rows); i += d {
			if !t.rows[i].kind.heading() {
				t.cursor = i
				return
			}
		}
	}
}

// setPane switches what the tree shows and rebuilds.
func (t *tree) setPane(p paneKind) {
	t.pane = p
	t.rebuild()
}

// setScope points a task-list tree at category id (0 = All) and rebuilds,
// keeping the selected task when the new scope shows it, else selecting the
// first task. A no-op when the scope is unchanged.
func (t *tree) setScope(id string) {
	if id == t.scope {
		return
	}
	sel := t.selectedID()
	t.scope = id
	t.rebuild()
	if !t.selectID(sel) {
		t.selectTop()
	}
}

// refreshSidebar rebuilds the sidebar keeping the selected category (or the
// "+ new category" row) when it is still shown, else falling back to All —
// after a tab switch, a filter change or a reload took it away.
func (t *tree) refreshSidebar() {
	id, onPH := t.selectedID(), t.onPlaceholder()
	t.rebuild()
	if t.selectID(id) {
		return
	}
	if onPH {
		for i, r := range t.rows {
			if r.kind == rowNewCategory {
				t.cursor = i
				t.reconcileOffset()
				return
			}
		}
	}
	t.selectTop()
}

// selected returns the item under the cursor. On the placeholder row it returns
// the sentinel — callers guard with onPlaceholder before treating it as a real
// item.
// The All and heading rows have no selectable item: it returns nil there.
func (t *tree) selected() *todo.Item {
	if t.cursor < 0 || t.cursor >= len(t.rows) {
		return nil
	}
	if r := t.rows[t.cursor]; r.kind == rowAll || r.kind.heading() {
		return nil
	}
	return t.rows[t.cursor].item
}

// onKind reports whether the cursor is on a row of kind k.
func (t *tree) onKind(k rowKind) bool {
	return t.cursor >= 0 && t.cursor < len(t.rows) && t.rows[t.cursor].kind == k
}

// onPlaceholder reports whether the cursor is on the "+ new category" row.
func (t *tree) onPlaceholder() bool { return t.onKind(rowNewCategory) }

// onAll reports whether the cursor is on the sidebar's "All" row.
func (t *tree) onAll() bool { return t.onKind(rowAll) }

// selectItem moves the cursor onto it, if it is currently visible, scrolling the
// viewport as needed to keep it on screen.
func (t *tree) selectItem(it *todo.Item) {
	for i, r := range t.rows {
		if r.kind == rowItem && r.item == it {
			t.cursor = i
			t.reconcileOffset()
			return
		}
	}
}

// selectID moves the cursor onto the visible item with the given ID, scrolling
// the viewport as needed; it reports whether one was found. ID 0 (an item not in
// the store, such as the placeholder sentinel) never matches.
func (t *tree) selectID(id string) bool {
	if id == "" {
		return false
	}
	for i, r := range t.rows {
		if r.kind == rowItem && r.item.ID == id {
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
func (t *tree) adopt(doc *todo.Document, focusID string, onPlaceholder bool) {
	t.doc = doc
	present := map[string]bool{}
	for _, it := range doc.Enumerate() {
		present[it.ID] = true
	}
	for _, folds := range t.foldsByKey {
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
	if onPlaceholder && !t.filtering() {
		for i, r := range t.rows {
			if r.kind == rowNewCategory {
				t.cursor = i
				t.reconcileOffset()
				return
			}
		}
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
	t.settle(1)
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
	if delta < 0 {
		t.settle(-1)
	} else {
		t.settle(1)
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
	// The headings right above the window name what the cursor is in: bring
	// them into view while the cursor still fits.
	for t.offset > 0 && t.rows[t.offset-1].kind.heading() && t.cursor < t.offset-1+h {
		t.offset--
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

// msgFoldsOffWhileSearching is the footer hint for z during a search, which
// shows every match unfolded.
const msgFoldsOffWhileSearching = "folds are off while searching"

// msgFoldInSidebar is the footer hint for z in the split view's task list,
// whose category headers do not fold.
const msgFoldInSidebar = "fold categories in the sidebar"

// msgNoFoldOnBoard is the footer hint for z on a sprint board, whose category
// headers do not fold.
const msgNoFoldOnBoard = "categories do not fold on the board"

// toggleFoldLevel folds every category at the level of the selection — the
// selected category and its sibling categories, or for a task those of its
// category — when any of them is unfolded, and unfolds them all otherwise.
// When the fold hides the selected row, the selection moves to the folded
// category holding it. It returns the footer message: the reason it did
// nothing while searching, else "".
func (t *tree) toggleFoldLevel() string {
	sel := t.selected()
	if sel == nil {
		return ""
	}
	switch {
	case t.pane == paneTasks:
		return msgFoldInSidebar
	case t.vs.onBoard():
		return msgNoFoldOnBoard
	}
	if t.filtering() {
		return msgFoldsOffWhileSearching
	}
	anchor := sel.EnclosingCategory()
	if anchor == nil {
		return ""
	}
	peers := t.doc.Roots
	if anchor.Parent != nil {
		peers = anchor.Parent.Children
	}
	var level []*todo.Item
	fold := false
	for _, c := range peers {
		if c.Kind == todo.Category && t.foldable(c) {
			level = append(level, c)
			fold = fold || !t.collapsed[c.ID]
		}
	}
	for _, c := range level {
		t.collapsed[c.ID] = fold
	}
	t.rebuild()
	if t.selected() != sel {
		t.selectItem(anchor)
	}
	return ""
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
		empty := "nothing in " + t.vs.title()
		if t.filtering() {
			empty = "no matches in " + t.vs.title()
		}
		if t.pane == paneTasks {
			empty = "no tasks here"
			if t.filtering() {
				empty = "no matches here"
			}
		}
		return "  " + helpTextStyle.Render(empty)
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
		if t.rows[i].kind == rowSection {
			b.WriteString(sectionRow(t.rows[i], width))
			continue
		}
		b.WriteString(t.rowString(t.rows[i], i == t.cursor))
	}
	return b.String()
}

// sectionRow renders a board section's row, width wide: a heavy rule in the
// column's colour carrying its title and how many top-level tasks it shows,
// e.g. "━━ In Progress 3 ━━━━━━━━".
func sectionRow(r treeRow, width int) string {
	st := columnStyle(r.col)
	lead, label, count := "━━ ", r.col.Title(), fmt.Sprintf(" %d ", r.count)
	used := ansi.StringWidth(lead) + ansi.StringWidth(label) + ansi.StringWidth(count)
	line := st.Render(lead) + st.Bold(true).Render(label) + helpTextStyle.Render(count) +
		st.Render(strings.Repeat("━", max(width-used, 0)))
	return ansi.Truncate(line, width, "")
}

// rowString renders one row. A selected row is drawn wholly in the accent
// colour behind a "❯ " gutter; unselected rows keep their per-token styling
// (purple categories, green ticks, struck-through done titles) behind a
// matching two-space gutter so columns line up. A task that has a description
// carries a trailing "≡" marker.
func (t *tree) rowString(r treeRow, selected bool) string {
	switch r.kind {
	case rowAll:
		if selected {
			return selectedRowStyle.Render("❯ All")
		}
		return "  " + categoryStyle.Render("All")
	case rowHeader:
		return "  " + helpTextStyle.Render("── ") + categoryStyle.Render(todo.Printable(r.label))
	}
	if r.kind == rowNewCategory {
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
		if t.collapsed[r.item.ID] && !t.filtering() {
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
		title := todo.Printable(r.item.Title)
		// The category's overall progress (sprint boards draw no category
		// rows, see rebuildBoard).
		counts := ""
		if done, total := r.item.TaskCounts(); total > 0 {
			counts = fmt.Sprintf(" (%d/%d)", done, total)
		}
		if selected {
			return selStyle.Render(gutter+indent+fold) +
				highlight(title, t.vs.filter, selStyle) + selStyle.Render(counts)
		}
		return "  " + indent + helpTextStyle.Render(fold) +
			highlight(title, t.vs.filter, categoryStyle) + helpTextStyle.Render(counts)
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
	state, warn, note := taskTrailer(r.item, t.sprintTag(r.item))
	if selected {
		return selStyle.Render(gutter+indent+fold+glyph+" ") + highlight(todo.Printable(r.item.DisplayTitle()), t.vs.filter, selStyle) + selStyle.Render(state+warn+note)
	}
	return "  " + indent + helpTextStyle.Render(fold) + boxStyle.Render(glyph) + " " + highlight(todo.Printable(r.item.DisplayTitle()), t.vs.filter, titleStyle) +
		helpTextStyle.Render(state) + deferStyle.Render(warn) + helpTextStyle.Render(note)
}

// sprintTag is the sprint name a row of top-level task it shows next to its
// state: only in the unified view, which mixes every sprint's tasks, and only
// for a task in a sprint. The name is made safe to draw and cut to
// maxNameWidth, as in the Sprints workspace's sidebar. It is "" otherwise.
func (t *tree) sprintTag(it *todo.Item) string {
	if !t.vs.all || it.IsSubtask() || it.Sprint == "" {
		return ""
	}
	s, ok := t.doc.SprintByID(it.Sprint)
	if !ok {
		return ""
	}
	return ansi.Truncate(todo.Printable(s.Name), maxNameWidth, "…")
}

// taskTrailer returns the unstyled pieces drawn after a task's title: the
// state label, then sprint (the task's sprint name, when the row shows one;
// see sprintTag) and subtask progress (top-level tasks only), the no-plan
// warning, and the ≡ marker. Each piece is empty or starts with a space.
func taskTrailer(it *todo.Item, sprint string) (state, warn, note string) {
	if !it.IsSubtask() {
		state = " " + it.Status.Label()
		if sprint != "" {
			state += " · " + sprint
		}
		if done, total := it.TaskCounts(); total > 0 {
			state += fmt.Sprintf(" (%d/%d)", done, total)
		}
	}
	if it.NeedsPlan() {
		warn = " ⚠ no plan"
	}
	// A task with a description gets a trailing ≡ marker, so the single-panel list
	// still shows at a glance which items have details to open (enter).
	if it.Description != "" {
		note = " ≡"
	}
	return state, warn, note
}
