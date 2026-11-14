// Package todo is the domain of the TODO app: the in-memory tree loaded from the
// SQLite store, the operations over it (toggle, cascade-complete, and the
// snapshot/restore that powers accidental-complete undo), the store itself, and
// the one-time markdown import. It has no dependency on the TUI.
package todo

import (
	"fmt"
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
// states; the rest are kanban states. The numbers are stored in the database, so
// they are pinned: never renumber them, only append. Open, Working, Deferred
// and Done have a checkbox marker — `[ ]` `[/]` `[>]` `[x]` — which the markdown
// import reads; the others have none. Only Done reads
// as "complete": counts, pruning and the done-cascade all key off IsDone.
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
	ID          uint // database node ID; 0 until the item is persisted
	Kind        Kind
	Title       string
	Level       int    // Category only: its depth among categories, 1 for a root category
	Status      Status // Task only
	Description string // Task only: free-form text shown in the details view
	Plan        string // Task only: path or reference of the plan for the task
	Children    []*Item
	Parent      *Item // nil for a root item
}

// Document is the in-memory TODO tree: its root items.
type Document struct {
	Roots []*Item
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

// NewTask builds a task item (with no children or parent set yet). The done
// flag maps to Done or Open; a caller wanting another state (Working, Deferred,
// …) sets Status afterwards.
func NewTask(title, description string, done bool) *Item {
	st := Open
	if done {
		st = Done
	}
	return &Item{Kind: Task, Title: title, Description: description, Status: st}
}

// AppendChild appends c as the last child of it, setting c.Parent.
func (it *Item) AppendChild(c *Item) {
	c.Parent = it
	it.Children = append(it.Children, c)
}

// AppendTask adds task as the last *task* child of it — before any
// subcategory children — and sets task.Parent. This preserves the invariant
// that a category's direct tasks precede its subcategories, which the move
// rules (crossUp/crossDown) and the displayed tree depend on. For a task
// parent (a subtask) there are no category children, so it simply appends.
func (it *Item) AppendTask(task *Item) {
	task.Parent = it
	pos := len(it.Children)
	for i, c := range it.Children {
		if c.Kind == Category {
			pos = i
			break
		}
	}
	it.Children = append(it.Children, nil)
	copy(it.Children[pos+1:], it.Children[pos:])
	it.Children[pos] = task
}

// AppendRoot appends it as the last top-level item of the document.
func (d *Document) AppendRoot(it *Item) {
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

// InsertAfter inserts sib immediately after ref among ref's peers, giving sib
// the same parent as ref. It is a no-op (returning false) if ref is not found.
func (d *Document) InsertAfter(ref, sib *Item) bool {
	list, idx, ok := d.siblings(ref)
	if !ok {
		return false
	}
	sib.Parent = ref.Parent
	list = append(list, nil)
	copy(list[idx+2:], list[idx+1:])
	list[idx+1] = sib
	if ref.Parent != nil {
		ref.Parent.Children = list
	} else {
		d.Roots = list
	}
	return true
}

// Remove detaches it from the tree. It is a no-op (returning false) if it is
// not found among its peers.
func (d *Document) Remove(it *Item) bool {
	list, idx, ok := d.siblings(it)
	if !ok {
		return false
	}
	list = append(list[:idx], list[idx+1:]...)
	if it.Parent != nil {
		it.Parent.Children = list
	} else {
		d.Roots = list
	}
	return true
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
// the document, and it is what Store.SetStatuses writes back.
type DoneStates map[uint]Status

// SnapshotDone captures the Status of it and all its descendant tasks, by ID.
func SnapshotDone(it *Item) DoneStates {
	s := DoneStates{it.ID: it.Status}
	for _, d := range it.Descendants() {
		s[d.ID] = d.Status
	}
	return s
}

// ApplyStatuses sets the in-memory Status of every item of d whose ID s
// records. IDs d does not hold (an item deleted since the snapshot) are skipped.
func (d *Document) ApplyStatuses(s DoneStates) {
	for _, it := range d.Enumerate() {
		if st, ok := s[it.ID]; ok {
			it.Status = st
		}
	}
}

// CascadeSetDone sets it and every descendant task to Done (done=true) or Open
// (done=false) — the behaviour where completing a parent completes all its
// children.
func CascadeSetDone(it *Item, done bool) {
	st := Open
	if done {
		st = Done
	}
	it.Status = st
	for _, d := range it.Descendants() {
		d.Status = st
	}
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

// Placement returns where it sits in the tree as database IDs: parentID is the
// parent's ID (0 for a root) and afterID is the ID of the sibling immediately
// before it (0 when it is the first among its peers).
func (d *Document) Placement(it *Item) (parentID, afterID uint) {
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
