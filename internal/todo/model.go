// Package todo is the pure domain of the TODO app: the in-memory tree parsed
// from a markdown file, the operations over it (toggle, cascade-complete, and
// the snapshot/restore that powers accidental-complete undo), and the
// markdown (de)serialization. It has no dependency on the TUI — everything
// here is testable in isolation.
package todo

// Kind distinguishes a category (a markdown header) from a task (a checkbox
// list item).
type Kind int

const (
	// Category is a markdown header (`#`..`######`); it groups sub-categories
	// and tasks and carries no Done/Description of its own.
	Category Kind = iota
	// Task is a checkbox list item (`- [ ]` / `- [x]`); it carries a Status
	// and a free-form Description, and may nest sub-tasks.
	Task
)

// Status is a task's completion state. Open and Done are the classic checkbox
// states; InProgress and Deferred are extra flags. Each maps to an extended
// checkbox marker on disk — `[ ]` `[/]` `[>]` `[x]` — parsed and rendered by
// this package. Only Done reads as "complete": counts, pruning and the
// done-cascade all key off IsDone, so InProgress and Deferred count as not done.
type Status int

const (
	Open       Status = iota // `[ ]`
	InProgress               // `[/]`
	Deferred                 // `[>]`
	Done                     // `[x]`
)

// String returns the lowercase status name used by the CLI (JSON output and
// the `add --status` flag): open, progress, deferred, done.
func (s Status) String() string {
	switch s {
	case InProgress:
		return "progress"
	case Deferred:
		return "deferred"
	case Done:
		return "done"
	default:
		return "open"
	}
}

// Marker returns the checkbox marker char for the status, e.g. "x" for Done. It
// is markerFromStatus exposed for the CLI's list table; the two stay in sync.
func (s Status) Marker() string { return markerFromStatus(s) }

// Item is one node of the TODO tree. Categories nest by header level and hold
// sub-categories and tasks; tasks nest by list indentation and hold sub-tasks.
// Only tasks use Status and Description; only categories use Level.
type Item struct {
	Kind        Kind
	Title       string
	Level       int    // Category only: header level 1..6
	Line        int    // 1-based source line of the header/checkbox; 0 until parsed
	Status      Status // Task only
	Description string // Task only: free-form text shown in the details view
	Children    []*Item
	Parent      *Item // nil for a root item
}

// Document is a parsed TODO file: its root items plus any verbatim text that
// appeared before the first header or task (preserved across a save so a title
// or intro paragraph at the top of the file is never lost).
type Document struct {
	Preamble string
	Roots    []*Item
}

// IsTask reports whether the item is a task (rather than a category).
func (it *Item) IsTask() bool { return it.Kind == Task }

// IsDone reports whether the task is complete. It is the successor to the old
// Done bool: the done-cascade, counts and pruning all key off it, so a task that
// is InProgress or Deferred reads as not done.
func (it *Item) IsDone() bool { return it.Status == Done }

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
// flag maps to Done or Open; a caller wanting InProgress/Deferred sets Status
// afterwards.
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
// that a category's direct tasks precede its subcategories, which is what keeps
// the markdown round-tripping: a task written after a `##` subheader would
// otherwise re-parent under that subheader when the file is reloaded. For a
// task parent (a subtask) there are no category children, so it simply appends.
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

// DoneStates records the Status of a set of tasks, so a cascade-complete can be
// reverted to the exact prior state — including a child that was InProgress or
// Deferred. It is the in-memory snapshot behind the "I marked the parent done by
// accident" undo.
type DoneStates map[*Item]Status

// SnapshotDone captures the Status of it and all its descendant tasks.
func SnapshotDone(it *Item) DoneStates {
	s := DoneStates{it: it.Status}
	for _, d := range it.Descendants() {
		s[d] = d.Status
	}
	return s
}

// RestoreDone re-applies a snapshot's recorded statuses.
func RestoreDone(s DoneStates) {
	for it, st := range s {
		it.Status = st
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

// RemovableDone reports how many tasks RemoveDone would delete — every task
// whose whole subtree is done. It does not modify the document, so callers can
// size a confirmation prompt before committing to the removal.
func (d *Document) RemovableDone() int {
	n := 0
	var walk func(items []*Item)
	walk = func(items []*Item) {
		for _, it := range items {
			if fullyDone(it) {
				n++
			}
			walk(it.Children)
		}
	}
	walk(d.Roots)
	return n
}

// RemoveDone deletes every completed task from the document and returns how many
// tasks were removed. A done task is dropped together with its subtree, but only
// when that whole subtree is done: a done task that still holds an unfinished
// sub-task is kept (the prune recurses into it, so done tasks nested deeper are
// still removed). This mirrors single-item delete, which also removes a subtree,
// while never discarding unfinished work.
func (d *Document) RemoveDone() int {
	removed := 0
	var prune func(items []*Item) []*Item
	prune = func(items []*Item) []*Item {
		kept := items[:0]
		for _, it := range items {
			if fullyDone(it) {
				_, total := it.TaskCounts()
				removed += 1 + total // this task plus its (all-done) sub-tasks
				continue             // drop the whole subtree
			}
			it.Children = prune(it.Children)
			kept = append(kept, it)
		}
		return kept
	}
	d.Roots = prune(d.Roots)
	return removed
}
