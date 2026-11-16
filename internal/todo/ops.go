package todo

import (
	"fmt"
	"maps"
	"slices"
)

// The list operations below are the in-memory form of the store's writes: each
// one applies the store's rules on the Document it is called on, and the store
// runs it on the list as the file holds it at write time. Every operation
// validates everything first and changes d only when it succeeds, so on an
// error d is exactly as it was. An ID of "" names the root as a parent and
// "place first" as an afterID.

// itemIndex maps every non-empty item ID of a document to its item.
type itemIndex map[string]*Item

// index returns the itemIndex of d.
func (d *Document) index() itemIndex {
	ix := itemIndex{}
	for _, it := range d.Enumerate() {
		if it.ID != "" {
			ix[it.ID] = it
		}
	}
	return ix
}

// taken reports whether id is already used by an item of the index.
func (ix itemIndex) taken(id string) bool {
	_, ok := ix[id]
	return ok
}

// notFound is the ErrNotFound for a missing item id.
func notFound(id string) error {
	return fmt.Errorf("%w: id %s", ErrNotFound, id)
}

// lookup returns the item id names, or ErrNotFound naming id.
func (d *Document) lookup(id string) (*Item, error) {
	if it, ok := d.ByID(id); ok {
		return it, nil
	}
	return nil, notFound(id)
}

// parentItem returns the item parentID names, nil for the root (""), or
// ErrNotFound naming parentID.
func (d *Document) parentItem(parentID string) (*Item, error) {
	if parentID == "" {
		return nil, nil
	}
	return d.lookup(parentID)
}

// children returns parent's children, or d's roots when parent is nil.
func (d *Document) children(parent *Item) []*Item {
	if parent == nil {
		return d.Roots
	}
	return parent.Children
}

// insertAt inserts it at index pos among parent's children (d's roots when
// parent is nil) and sets it.Parent.
func (d *Document) insertAt(parent *Item, pos int, it *Item) {
	it.Parent = parent
	list := slices.Insert(d.children(parent), pos, it)
	if parent == nil {
		d.Roots = list
	} else {
		parent.Children = list
	}
}

// firstCategory returns the index of the first category in list, len(list)
// when it holds none: the slot after its last task.
func firstCategory(list []*Item) int {
	for i, it := range list {
		if it.Kind == Category {
			return i
		}
	}
	return len(list)
}

// slot returns the index, among parent's children other than self (the item
// being moved; nil for an add), at which an item of kind k placed after the
// sibling afterID goes. "" places a task first and a category first among the
// subcategories, after every task; a task anchor does the same for a category,
// so tasks always stay ahead of categories. An anchor that is not such a child
// of parent (self included), or a category anchor for a task, is ErrConflict.
func (d *Document) slot(parent *Item, k Kind, afterID string, self *Item) (int, error) {
	peers := slices.DeleteFunc(slices.Clone(d.children(parent)), func(c *Item) bool { return c == self })
	if afterID == "" {
		if k == Category {
			return firstCategory(peers), nil
		}
		return 0, nil
	}
	i := slices.IndexFunc(peers, func(c *Item) bool { return c.ID == afterID })
	switch {
	case i < 0:
		return 0, ErrConflict
	case peers[i].Kind == Task && k == Category:
		return firstCategory(peers), nil
	case peers[i].Kind == Category && k == Task:
		return 0, ErrConflict
	}
	return i + 1, nil
}

// checkPlacement reports whether an item of kind k may sit under parent (nil
// for the root): a task needs a parent (ErrWrongParent), a category cannot sit
// under a task (ErrWrongParent), and a task cannot sit under a subtask
// (ErrTooDeep).
func checkPlacement(k Kind, parent *Item) error {
	if parent == nil {
		if k == Task {
			return ErrWrongParent
		}
		return nil
	}
	if parent.Kind != Task {
		return nil
	}
	if k == Category {
		return ErrWrongParent
	}
	if parent.IsSubtask() {
		return ErrTooDeep
	}
	return nil
}

// categoryLevel is the Level of category it where it sits: one more than the
// number of categories above it.
func categoryLevel(it *Item) int {
	level := 1
	for p := it.Parent; p != nil; p = p.Parent {
		if p.Kind == Category {
			level++
		}
	}
	return level
}

// place inserts it at pos among parent's children and, for a category,
// recomputes the Level of it and its subcategories.
func (d *Document) place(parent *Item, pos int, it *Item) {
	d.insertAt(parent, pos, it)
	if it.Kind == Category {
		relevel(it, categoryLevel(it))
	}
}

// addNew gives it a fresh ID, places it at pos under parent and returns the ID.
func (d *Document) addNew(parent *Item, pos int, it *Item) string {
	it.ID = newID(d.index().taken)
	d.place(parent, pos, it)
	return it.ID
}

// AddCategory adds a category titled title under parentID ("" for a root),
// after the sibling afterID ("" places it first among the categories), and
// returns its new ID. An afterID that is a task places it first among the
// subcategories, after every task. Under a task it fails with ErrWrongParent,
// a missing parent with ErrNotFound, an afterID that is not a child of the
// parent with ErrConflict, and a title CleanTitle refuses with its error.
func (d *Document) AddCategory(title, parentID, afterID string) (string, error) {
	return d.addCategory(title, parentID, func(parent *Item) (int, error) {
		return d.slot(parent, Category, afterID, nil)
	})
}

// AppendCategory adds a category like AddCategory, as the last child of
// parentID ("" for the root), after its tasks and subcategories.
func (d *Document) AppendCategory(title, parentID string) (string, error) {
	return d.addCategory(title, parentID, func(parent *Item) (int, error) {
		return len(d.children(parent)), nil
	})
}

// addCategory is AddCategory with the slot under the parent worked out by at.
func (d *Document) addCategory(title, parentID string, at func(parent *Item) (int, error)) (string, error) {
	title, err := CleanTitle(title)
	if err != nil {
		return "", err
	}
	parent, err := d.parentItem(parentID)
	if err != nil {
		return "", err
	}
	if err := checkPlacement(Category, parent); err != nil {
		return "", err
	}
	pos, err := at(parent)
	if err != nil {
		return "", err
	}
	return d.addNew(parent, pos, &Item{Kind: Category, Title: title}), nil
}

// AddTask adds a task with fields f and status st under parentID, after the
// sibling afterID ("" places it first), and returns its new ID. At the root it
// fails with ErrWrongParent, under a subtask with ErrTooDeep, and under a task
// (making a subtask) with a state other than Open or Done with
// ErrSubtaskStatus. A missing parent is ErrNotFound; an afterID that is not a
// child of the parent, or is a category, is ErrConflict; a title CleanTitle
// refuses fails it with that error.
func (d *Document) AddTask(f Fields, st Status, parentID, afterID string) (string, error) {
	return d.addTask(f, st, parentID, func(parent *Item) (int, error) {
		return d.slot(parent, Task, afterID, nil)
	})
}

// AppendTask adds a task like AddTask, after the last task child of parentID
// and before any subcategory.
func (d *Document) AppendTask(f Fields, st Status, parentID string) (string, error) {
	return d.addTask(f, st, parentID, func(parent *Item) (int, error) {
		return firstCategory(parent.Children), nil
	})
}

// addTask is AddTask with the slot under the parent worked out by at, which is
// only called with a non-nil parent.
func (d *Document) addTask(f Fields, st Status, parentID string, at func(parent *Item) (int, error)) (string, error) {
	title, err := CleanTitle(f.Title)
	if err != nil {
		return "", err
	}
	parent, err := d.parentItem(parentID)
	if err != nil {
		return "", err
	}
	if err := checkPlacement(Task, parent); err != nil {
		return "", err
	}
	if parent.Kind == Task && !st.SubtaskAllowed() {
		return "", ErrSubtaskStatus
	}
	pos, err := at(parent)
	if err != nil {
		return "", err
	}
	it := &Item{Kind: Task, Title: title, Status: st, Description: f.Description, Plan: f.Plan}
	return d.addNew(parent, pos, it), nil
}

// Edit sets the fields of item id that c names: its title and, for a task, its
// description and plan; a category ignores c.Description and c.Plan. Fields c
// leaves nil are kept. changed is false when every field applied already held
// its value, or c names no field the item uses. A missing id is ErrNotFound and
// a title CleanTitle refuses fails it with that error.
func (d *Document) Edit(id string, c Changes) (changed bool, err error) {
	var title string
	if c.Title != nil {
		if title, err = CleanTitle(*c.Title); err != nil {
			return false, err
		}
	}
	it, err := d.lookup(id)
	if err != nil {
		return false, err
	}
	set := func(field *string, v *string) {
		if v != nil && *field != *v {
			*field = *v
			changed = true
		}
	}
	if c.Title != nil {
		set(&it.Title, &title)
	}
	if it.Kind == Task {
		set(&it.Description, c.Description)
		set(&it.Plan, c.Plan)
	}
	return changed, nil
}

// SetStatuses sets the status of every task in states, or of none: an ID d
// does not hold fails it with ErrNotFound, a category with ErrNotTask, and a
// subtask given a state other than Open or Done with ErrSubtaskStatus.
func (d *Document) SetStatuses(states DoneStates) error {
	return d.setStatuses(states, d.index())
}

// setStatuses is SetStatuses on the index ix of d.
func (d *Document) setStatuses(states DoneStates, ix itemIndex) error {
	ids := slices.Sorted(maps.Keys(states))
	for _, id := range ids {
		if it, ok := ix[id]; ok && it.Kind != Task {
			return ErrNotTask
		}
	}
	for _, id := range ids {
		if !ix.taken(id) {
			return notFound(id)
		}
	}
	for _, id := range ids {
		if !states[id].SubtaskAllowed() && ix[id].IsSubtask() {
			return ErrSubtaskStatus
		}
	}
	for id, st := range states {
		ix[id].Status = st
	}
	return nil
}

// CascadeStatus sets task id to st and every item below it to Done when st is
// Done and to Open otherwise: a board state never reaches a subtask. A missing
// id is ErrNotFound, a category ErrNotTask, and a subtask given a state other
// than Open or Done ErrSubtaskStatus.
func (d *Document) CascadeStatus(id string, st Status) error {
	it, err := d.lookup(id)
	if err != nil {
		return err
	}
	if it.Kind != Task {
		return ErrNotTask
	}
	if !st.SubtaskAllowed() && it.IsSubtask() {
		return ErrSubtaskStatus
	}
	below := Open
	if st == Done {
		below = Done
	}
	it.Status = st
	for _, c := range it.Descendants() {
		c.Status = below
	}
	return nil
}

// Complete sets task id to Done. With cascade every item below it is set to
// Done too (see CascadeStatus); without it the subtasks keep their states.
func (d *Document) Complete(id string, cascade bool) error {
	if cascade {
		return d.CascadeStatus(id, Done)
	}
	return d.SetStatuses(DoneStates{id: Done})
}

// Reopen sets task id to st and treats its subtasks as o says: o.Cascade opens
// them all (see CascadeStatus), o.Restore gives them their snapshot states back
// (see ReopenStates), and with neither they keep theirs. o.Restore undoes a
// complete, so it gives back only what that complete left: an entry is
// applied only to an item that is still below task id and still Done, and
// skipped for one d no longer holds, that moved elsewhere or whose state
// changed since (say, another writer reopened it). Setting both fails with
// ErrRestoreAndCascade before anything else is checked; otherwise it fails as
// SetStatuses or CascadeStatus does.
func (d *Document) Reopen(id string, st Status, o ReopenOptions) error {
	if o.Restore != nil && o.Cascade {
		return ErrRestoreAndCascade
	}
	if o.Cascade {
		return d.CascadeStatus(id, st)
	}
	ix := d.index()
	below := map[*Item]bool{}
	if it, ok := ix[id]; ok {
		for _, c := range it.Descendants() {
			below[c] = true
		}
	}
	restore := make(DoneStates, len(o.Restore))
	for rid, rst := range o.Restore {
		if it, ok := ix[rid]; ok && below[it] && it.Status == Done {
			restore[rid] = rst
		}
	}
	return d.setStatuses(ReopenStates(id, st, restore), ix)
}

// SetState sets task id to st, deciding from the state the task is in now what
// that means, so one call serves every state change:
//
//   - into Done (st is Done) it completes the task, as Complete does;
//   - out of Done (the task is Done and st is not) it reopens it, as Reopen
//     does, giving back o.Restore's states to the tasks the complete left
//     done;
//   - any other change just sets st; o.Restore is not used, as there is no
//     complete to undo.
//
// With o.Cascade every task below it is set too, to Done when st is Done and
// to Open otherwise (see CascadeStatus), whichever case applies; without it the
// subtasks keep their states (but for a restore). changed is false when every
// status already held its value. Setting both o.Restore and o.Cascade fails
// with ErrRestoreAndCascade before anything else is checked; a missing id is
// ErrNotFound, a category ErrNotTask, and a subtask given a state other than
// Open or Done ErrSubtaskStatus.
func (d *Document) SetState(id string, st Status, o StateOptions) (changed bool, err error) {
	if o.Restore != nil && o.Cascade {
		return false, ErrRestoreAndCascade
	}
	it, err := d.lookup(id)
	if err != nil {
		return false, err
	}
	before := d.statuses()
	switch {
	case o.Cascade:
		err = d.CascadeStatus(id, st)
	case it.Status == Done && st != Done:
		err = d.Reopen(id, st, ReopenOptions{Restore: o.Restore})
	default:
		err = d.SetStatuses(DoneStates{id: st})
	}
	if err != nil {
		return false, err
	}
	return !maps.Equal(before, d.statuses()), nil
}

// statuses returns the status of every task of d, by ID.
func (d *Document) statuses() DoneStates {
	s := DoneStates{}
	for _, it := range d.Enumerate() {
		if it.Kind == Task {
			s[it.ID] = it.Status
		}
	}
	return s
}

// Move places item id, with its subtree, under parentID after the sibling
// afterID, by the placement rules of AddCategory and AddTask. A task moved to
// the root or a category moved under a task fails with ErrWrongParent; a task
// moved under a subtask, or a task with children moved under a task, with
// ErrTooDeep; a move into the item itself or its subtree with ErrInvalidMove.
// A missing item or parent is ErrNotFound, and an afterID that is not a fitting
// child of the parent (the item itself included) ErrConflict. A task that
// becomes, stays or stops being a subtask takes its status from
// StatusAfterMove; a task moved between categories keeps its status. A moved
// category's subtree is re-levelled.
func (d *Document) Move(id, parentID, afterID string) error {
	it, err := d.lookup(id)
	if err != nil {
		return err
	}
	parent, err := d.parentItem(parentID)
	if err != nil {
		return err
	}
	if err := checkPlacement(it.Kind, parent); err != nil {
		return err
	}
	toSubtask := parent != nil && parent.Kind == Task
	if it.Kind == Task && toSubtask && len(it.Children) > 0 {
		return ErrTooDeep
	}
	for p := parent; p != nil; p = p.Parent {
		if p == it {
			return ErrInvalidMove
		}
	}
	pos, err := d.slot(parent, it.Kind, afterID, it)
	if err != nil {
		return err
	}
	if it.IsSubtask() || (it.Kind == Task && toSubtask) {
		oldParent := Open
		if it.IsSubtask() {
			oldParent = it.Parent.Status
		}
		it.Status = StatusAfterMove(it.Status, toSubtask, oldParent)
	}
	d.Remove(it)
	d.place(parent, pos, it)
	return nil
}

// Delete removes item id together with its subtree. A missing id is
// ErrNotFound.
func (d *Document) Delete(id string) error {
	it, err := d.lookup(id)
	if err != nil {
		return err
	}
	d.Remove(it)
	return nil
}

// Prune removes every fully-done task subtree (see DoneSubtrees) and returns
// how many tasks went, counted as RemovableDone counts them.
func (d *Document) Prune() int {
	n := 0
	for _, it := range d.DoneSubtrees() {
		_, total := it.TaskCounts()
		n += 1 + total
		d.Remove(it)
	}
	return n
}

// AssignIDs gives every item whose ID is empty a fresh ID, unique in d, and
// keeps every existing ID.
func (d *Document) AssignIDs() {
	ix := d.index()
	for _, it := range d.Enumerate() {
		if it.ID == "" {
			it.ID = newID(ix.taken)
			ix[it.ID] = it
		}
	}
}
