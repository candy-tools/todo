// Package todo is the domain of the TODO app: the in-memory tree loaded from the
// JSON list file, the operations over it (toggle, cascade-complete, and the
// snapshot/restore that powers accidental-complete undo), the store itself, and
// the one-time markdown import. It has no dependency on the TUI.
package todo

import (
	"cmp"
	"fmt"
	"maps"
	"slices"
	"strings"
)

// Kind distinguishes a category from a task.
type Kind int

const (
	// Category groups sub-categories and tasks and carries no Status or
	// Description of its own. The markdown import reads one from a header.
	Category Kind = iota
	// Task carries a Status and a free-form Description, and may nest
	// sub-tasks. The markdown import reads one from a checkbox list item.
	Task
)

// Status is a task's workflow state. Open and Done are the classic checkbox
// states; the rest are kanban states. The list file stores a status by its name
// (see String), so the names are pinned; keep the numbers stable too, only
// append. Open, Working, Deferred and Done have a checkbox marker — `[ ]` `[/]`
// `[>]` `[x]` — which the markdown import reads; the others have none. Only
// Done reads as "complete": counts, pruning and the done-cascade all key off
// IsDone.
type Status int

const (
	Open     Status = 0 // `[ ]`
	Working  Status = 1 // `[/]`
	Deferred Status = 2 // `[>]`
	Done     Status = 3 // `[x]`
	New      Status = 4
	Ready    Status = 5
	Review   Status = 6
	Followup Status = 7
	WontDo   Status = 8 // a Backlog task that will not be done; it never counts as complete
)

// Column is a kanban column: a group of statuses. Columns are derived from a
// task's Status and never stored.
type Column int

const (
	ColBacklog Column = iota
	ColNext
	ColInProgress
	ColDone
)

var statusNames = map[Status]string{
	Open: "open", Working: "working", Deferred: "deferred", Done: "done",
	New: "new", Ready: "ready", Review: "review", Followup: "followup",
	WontDo: "wontdo",
}

// String returns the lowercase state name used by the CLI (JSON output and the
// `add --status` flag), or "status(N)" for an unknown value so it stays visible.
func (s Status) String() string {
	if n, ok := statusNames[s]; ok {
		return n
	}
	return fmt.Sprintf("status(%d)", int(s))
}

// Label returns the state's human-readable name for display, e.g. "won't do"
// for WontDo; for every other state it is the same as String.
func (s Status) Label() string {
	if s == WontDo {
		return "won't do"
	}
	return s.String()
}

// Marker returns the checkbox marker char for the status, e.g. "x" for Done, or
// "" for a kanban-only state with no marker. It is the inverse of
// statusFromMarker (see markdown.go); the todotest helpers use it to write a
// store back out as markdown.
func (s Status) Marker() string {
	switch s {
	case Open:
		return " "
	case Working:
		return "/"
	case Deferred:
		return ">"
	case Done:
		return "x"
	}
	return ""
}

// StatusFromName parses a status name into a Status; ok is false for an
// unknown name. "progress" is accepted as an alias of "working".
func StatusFromName(name string) (Status, bool) {
	if name == "progress" {
		return Working, true
	}
	for s, n := range statusNames {
		if n == name {
			return s, true
		}
	}
	return Open, false
}

// Column returns the kanban column the status belongs to; an unknown status maps
// to the backlog.
func (s Status) Column() Column {
	switch s {
	case New, Ready:
		return ColNext
	case Working, Review, Followup:
		return ColInProgress
	case Done:
		return ColDone
	}
	return ColBacklog
}

// SubtaskAllowed reports whether a subtask may be in this state: a subtask is
// only ever Open or Done. It does not limit which tasks hold subtasks; a task
// in any state (Working, for one) may.
func (s Status) SubtaskAllowed() bool { return s == Open || s == Done }

// Columns returns the four columns in board order.
func Columns() []Column { return []Column{ColBacklog, ColNext, ColInProgress, ColDone} }

// SprintColumns returns the columns of a sprint board, in board order: every
// column but the Backlog.
func SprintColumns() []Column { return []Column{ColNext, ColInProgress, ColDone} }

// String returns the column's short name: backlog, next, progress or done.
func (c Column) String() string {
	switch c {
	case ColBacklog:
		return "backlog"
	case ColNext:
		return "next"
	case ColInProgress:
		return "progress"
	case ColDone:
		return "done"
	}
	return fmt.Sprintf("column(%d)", int(c))
}

// Title returns the column's display title.
func (c Column) Title() string {
	switch c {
	case ColBacklog:
		return "Backlog"
	case ColNext:
		return "Next"
	case ColInProgress:
		return "In Progress"
	case ColDone:
		return "Done"
	}
	return c.String()
}

// ColumnFromName parses a column's short name; ok is false for an unknown name.
func ColumnFromName(name string) (Column, bool) {
	for _, c := range Columns() {
		if c.String() == name {
			return c, true
		}
	}
	return ColBacklog, false
}

// States returns the statuses that belong to the column, in cycle order.
func (c Column) States() []Status {
	switch c {
	case ColNext:
		return []Status{New, Ready}
	case ColInProgress:
		return []Status{Working, Review, Followup}
	case ColDone:
		return []Status{Done}
	}
	return []Status{Open, Deferred, WontDo}
}

// FirstState returns the state a task takes when it enters the column.
func (c Column) FirstState() Status { return c.States()[0] }

// StatusAfterMove returns the status a task takes when it is re-parented.
// Becoming a subtask keeps Done and otherwise resets to Open. A subtask becoming
// a task keeps Done and otherwise takes oldParent's status (Working if the old
// parent was itself Done).
func StatusAfterMove(cur Status, toSubtask bool, oldParent Status) Status {
	if cur == Done {
		return Done
	}
	if toSubtask {
		return Open
	}
	if oldParent == Done {
		return Working
	}
	return oldParent
}

// Item is one node of the TODO tree. Categories hold sub-categories and tasks;
// tasks hold sub-tasks. Only tasks use Status and Description; only categories
// use Level.
type Item struct {
	ID          string // item ID; empty until the item is persisted
	Kind        Kind
	Title       string
	Level       int    // Category only: its depth among categories, 1 for a root category
	Status      Status // Task only
	Description string // Task only: free-form text shown in the details view
	Plan        string // Task only: path or reference of the plan for the task
	// Ref is a top-level task's free-form reference, such as "APP-001", shown
	// before its title (see DisplayTitle); "" when it has none. No two tasks
	// of a list hold the same one, ignoring case; a subtask holds none.
	Ref string
	// Prefix is a category's reference prefix, "" for none: the tasks created
	// in it, and in its subcategories without one, get references from it (see
	// Document.PrefixFor).
	Prefix string
	// Sprint is a top-level task's sprint ID, "" for the backlog. A subtask
	// follows its task (see SprintID) and holds none.
	Sprint   string
	Children []*Item
	Parent   *Item // nil for a root item
}

// Document is the in-memory TODO tree: its root items, and the view the list
// is shown in.
type Document struct {
	Roots []*Item
	// Sprints are the list's sprints, in creation order.
	Sprints []*Sprint
	// View is how the list is shown, kept in the list file so every TUI
	// session opens on it.
	View View
	// Comment is the file's "_comment" note, kept as the file holds it ("" for
	// none). A write to a file with no note sets todo's own (fileComment).
	Comment string
	// RefPrefix is the list's reference prefix, "" for none: the last resort
	// of Document.PrefixFor.
	RefPrefix string
	// RefLast is the last number handed out per reference prefix, so a number
	// is never reused after its task goes; nil when none was.
	RefLast map[string]int
}

// View is how the TUI shows the list. The zero View is the default: the
// board's tabs, without the split view.
type View struct {
	// Unified shows every task, in any state, in one list instead of the
	// board's tabs (the TUI's "k").
	Unified bool
	// Split shows the categories in a sidebar beside a task list (the TUI's
	// "v").
	Split bool
	// Screen is the screen the TUI opens on.
	Screen Screen
	// Sprint is the ID of the sprint the sprint screen shows; it may name a
	// sprint that no longer exists, which the TUI treats as no sprint.
	Sprint string
}

// Screen is a screen of the TUI. The list file stores it by name (see
// String), so the names are pinned.
type Screen int

const (
	// ScreenBacklog is the backlog, the screen todo opens on.
	ScreenBacklog Screen = 0
	// ScreenSprints is the list of sprints.
	ScreenSprints Screen = 1
	// ScreenSprint is one sprint's board.
	ScreenSprint Screen = 2
)

var screenNames = map[Screen]string{ScreenBacklog: "backlog", ScreenSprints: "sprints", ScreenSprint: "sprint"}

// String returns the screen's name, or "screen(N)" for an unknown value so it
// stays visible.
func (s Screen) String() string {
	if n, ok := screenNames[s]; ok {
		return n
	}
	return fmt.Sprintf("screen(%d)", int(s))
}

// screenFromName parses a screen name; ok is false for an unknown name.
func screenFromName(name string) (Screen, bool) {
	for s, n := range screenNames {
		if n == name {
			return s, true
		}
	}
	return ScreenBacklog, false
}

// IsTask reports whether the item is a task (rather than a category).
func (it *Item) IsTask() bool { return it.Kind == Task }

// IsDone reports whether the task is complete. It is the successor to the old
// Done bool: the done-cascade, counts and pruning all key off it, so a task that
// is Working or Deferred reads as not done.
func (it *Item) IsDone() bool { return it.Status == Done }

// IsSubtask reports whether it is a task nested under another task.
func (it *Item) IsSubtask() bool {
	return it.Kind == Task && it.Parent != nil && it.Parent.Kind == Task
}

// Column returns the board column task it shows in: its status's column, or,
// for a subtask, its task's. It is for tasks only: a category sits in no
// column, and for one it returns ColBacklog, which a caller must not take as
// the category's place on the board.
func (it *Item) Column() Column {
	if it.IsSubtask() {
		return it.Parent.Status.Column()
	}
	if it.Kind != Task {
		return ColBacklog
	}
	return it.Status.Column()
}

// NeedsPlan reports whether it is a Ready task that has no plan yet.
func (it *Item) NeedsPlan() bool {
	return it.Kind == Task && it.Status == Ready && strings.TrimSpace(it.Plan) == ""
}

// EnclosingCategory returns the nearest category at or above it: it itself when
// it is a category, otherwise its closest category ancestor. It returns nil
// only for a task with no category ancestor, which does not occur in a parsed
// tree (every task lives under a category).
func (it *Item) EnclosingCategory() *Item {
	for n := it; n != nil; n = n.Parent {
		if n.Kind == Category {
			return n
		}
	}
	return nil
}

// AppendChild appends c as the last child of it, setting c.Parent.
func (it *Item) AppendChild(c *Item) {
	c.Parent = it
	it.Children = append(it.Children, c)
}

// Clone returns a deep copy of the document: every item is copied, with its
// Parent pointing into the copy, so changing one never shows in the other.
func (d *Document) Clone() *Document {
	c := &Document{Roots: make([]*Item, len(d.Roots)), View: d.View, Comment: d.Comment, RefPrefix: d.RefPrefix, RefLast: maps.Clone(d.RefLast)}
	for i, it := range d.Roots {
		c.Roots[i] = it.clone(nil)
	}
	for _, s := range d.Sprints {
		sc := *s
		c.Sprints = append(c.Sprints, &sc)
	}
	return c
}

// clone deep-copies it and its subtree under parent.
func (it *Item) clone(parent *Item) *Item {
	c := *it
	c.Parent = parent
	c.Children = make([]*Item, len(it.Children))
	for i, ch := range it.Children {
		c.Children[i] = ch.clone(&c)
	}
	return &c
}

// categoriesFirst reorders every category's children so its subcategories
// precede its tasks, keeping the order within each kind: the document order,
// as a file explorer lists folders before files.
func (d *Document) categoriesFirst() {
	for _, it := range d.Enumerate() {
		if it.Kind == Category {
			slices.SortStableFunc(it.Children, func(a, b *Item) int { return cmp.Compare(kindRank(a), kindRank(b)) })
		}
	}
}

// kindRank orders a category before a task.
func kindRank(it *Item) int {
	if it.Kind == Category {
		return 0
	}
	return 1
}

// appendRoot appends it as the last top-level item of the document.
func (d *Document) appendRoot(it *Item) {
	it.Parent = nil
	d.Roots = append(d.Roots, it)
}

// siblings returns the slice that holds it among its peers (its parent's
// Children, or the document roots for a top-level item) together with it's
// index in that slice. found is false if it is not present (defensive).
func (d *Document) siblings(it *Item) (list []*Item, idx int, found bool) {
	if it.Parent != nil {
		list = it.Parent.Children
	} else {
		list = d.Roots
	}
	for i, s := range list {
		if s == it {
			return list, i, true
		}
	}
	return list, -1, false
}

// remove detaches it from the tree; it does nothing when it is not found
// among its peers.
func (d *Document) remove(it *Item) {
	list, idx, ok := d.siblings(it)
	if !ok {
		return
	}
	list = append(list[:idx], list[idx+1:]...)
	if it.Parent != nil {
		it.Parent.Children = list
	} else {
		d.Roots = list
	}
}

// Descendants returns every item nested under it, depth-first, excluding it.
// For a task these are its sub-tasks (recursively); the cascade and snapshot
// helpers below build on it.
func (it *Item) Descendants() []*Item {
	var out []*Item
	var walk func(n *Item)
	walk = func(n *Item) {
		for _, c := range n.Children {
			out = append(out, c)
			walk(c)
		}
	}
	walk(it)
	return out
}

// TaskCounts returns how many descendant tasks of it are done and how many
// there are in total (it itself is not counted).
func (it *Item) TaskCounts() (done, total int) {
	for _, c := range it.Descendants() {
		if c.Kind == Task {
			total++
			if c.IsDone() {
				done++
			}
		}
	}
	return done, total
}

// DoneStates records the Status of a set of tasks by item ID, so a
// cascade-complete can be reverted to the exact prior state — including the
// task's own board state, such as Working or Review. It is the in-memory snapshot behind the "I
// marked the parent done by accident" undo; keyed by ID, it outlives a reload of
// the document, and it is what StateOptions.Restore writes back.
type DoneStates map[string]Status

// SnapshotDone captures the Status of it and all its descendant tasks, by ID.
func SnapshotDone(it *Item) DoneStates {
	s := DoneStates{it.ID: it.Status}
	for _, d := range it.Descendants() {
		s[d.ID] = d.Status
	}
	return s
}

// fullyDone reports whether it is a task that is done and whose every nested
// task is also done. Only such a subtree is safe to prune wholesale: a done
// task that still holds an unfinished sub-task is *not* fully done, so removing
// it would discard work the user hasn't finished.
func fullyDone(it *Item) bool {
	if !it.IsTask() || !it.IsDone() {
		return false
	}
	done, total := it.TaskCounts()
	return done == total
}

// placement returns where it sits in the tree as item IDs: parentID is the
// parent's ID ("" for a root) and afterID is the ID of the sibling immediately
// before it ("" when it is the first among its peers).
func (d *Document) placement(it *Item) (parentID, afterID string) {
	if it.Parent != nil {
		parentID = it.Parent.ID
	}
	if list, idx, ok := d.siblings(it); ok && idx > 0 {
		afterID = list[idx-1].ID
	}
	return parentID, afterID
}

// DoneSubtrees returns the outermost fully-done tasks (see fullyDone) in
// pre-order. It does not descend into a returned item, but does descend into
// every other item, so done tasks nested under a kept task are still found.
func (d *Document) DoneSubtrees() []*Item {
	var out []*Item
	var walk func(items []*Item)
	walk = func(items []*Item) {
		for _, it := range items {
			if fullyDone(it) {
				out = append(out, it)
				continue
			}
			walk(it.Children)
		}
	}
	walk(d.Roots)
	return out
}

// RemovableDone reports how many tasks removing the completed ones would delete
// — every task whose whole subtree is done (a done task that still holds an
// unfinished sub-task is kept). It does not modify the document, so callers can
// size a confirmation prompt before committing to the removal.
func (d *Document) RemovableDone() int {
	n := 0
	for _, it := range d.DoneSubtrees() {
		_, total := it.TaskCounts()
		n += 1 + total
	}
	return n
}
