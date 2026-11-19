package todo

import (
	"fmt"
)

// The sprint operations follow the rules of the list operations in ops.go:
// each validates first and leaves d untouched on an error.

// SprintChanges are the fields an EditSprint sets. A nil field is kept.
type SprintChanges struct {
	Name *string
	Goal *string
}

// sprintNotFound is the ErrSprintNotFound for a missing sprint id.
func sprintNotFound(id string) error {
	return fmt.Errorf("%w: id %s", ErrSprintNotFound, id)
}

// sprint returns the sprint id names, or ErrSprintNotFound naming id.
func (d *Document) sprint(id string) (*Sprint, error) {
	if s, ok := d.SprintByID(id); ok {
		return s, nil
	}
	return nil, sprintNotFound(id)
}

// checkSprintName cleans name and checks that no sprint other than self holds
// it, ignoring case. A name CleanTitle refuses is ErrInvalidTitle, a taken one
// ErrDuplicateSprint.
func (d *Document) checkSprintName(name string, self *Sprint) (string, error) {
	name, err := CleanTitle(name)
	if err != nil {
		return "", err
	}
	if other, ok := d.SprintByName(name); ok && other != self {
		return "", fmt.Errorf("%w: %q", ErrDuplicateSprint, name)
	}
	return name, nil
}

// AddSprint adds an open sprint named name with the given goal and returns its
// new ID. A name CleanTitle refuses is ErrInvalidTitle, and one another sprint
// holds (ignoring case) ErrDuplicateSprint.
func (d *Document) AddSprint(name, goal string) (string, error) {
	name, err := d.checkSprintName(name, nil)
	if err != nil {
		return "", err
	}
	id := newID(func(id string) bool { _, ok := d.SprintByID(id); return ok })
	d.Sprints = append(d.Sprints, &Sprint{ID: id, Name: name, Goal: goal})
	return id, nil
}

// EditSprint sets the fields of sprint id that c names. changed is false when
// every field c names already held its value. A missing sprint is
// ErrSprintNotFound; a name is checked as AddSprint checks it.
func (d *Document) EditSprint(id string, c SprintChanges) (changed bool, err error) {
	s, err := d.sprint(id)
	if err != nil {
		return false, err
	}
	var name string
	if c.Name != nil {
		if name, err = d.checkSprintName(*c.Name, s); err != nil {
			return false, err
		}
	}
	if c.Name != nil && s.Name != name {
		s.Name = name
		changed = true
	}
	if c.Goal != nil && s.Goal != *c.Goal {
		s.Goal = *c.Goal
		changed = true
	}
	return changed, nil
}

// CloseSprint marks sprint id done. While tasks or subtasks of it are not done
// it fails with a SprintNotDoneError.
func (d *Document) CloseSprint(id string) error {
	s, err := d.sprint(id)
	if err != nil {
		return err
	}
	if n := d.Unfinished(id); n > 0 {
		return &SprintNotDoneError{Remaining: n}
	}
	s.Status = SprintDone
	return nil
}

// ReopenSprint marks sprint id open again.
func (d *Document) ReopenSprint(id string) error {
	s, err := d.sprint(id)
	if err != nil {
		return err
	}
	s.Status = SprintOpen
	return nil
}

// DeleteSprint removes sprint id. A sprint with a done task is refused with
// ErrSprintHasDone. Its other tasks return to the backlog, Open; their
// subtasks keep their states.
func (d *Document) DeleteSprint(id string) error {
	if _, err := d.sprint(id); err != nil {
		return err
	}
	tasks := d.SprintTasks(id)
	for _, t := range tasks {
		if t.IsDone() {
			return ErrSprintHasDone
		}
	}
	for _, t := range tasks {
		t.Sprint = ""
		t.Status = Open
	}
	for i, s := range d.Sprints {
		if s.ID == id {
			d.Sprints = append(d.Sprints[:i], d.Sprints[i+1:]...)
			break
		}
	}
	return nil
}

// sprintTask returns the top-level task taskID names: a missing item is
// ErrNotFound, a category ErrNotTask and a subtask ErrTooDeep.
func (d *Document) sprintTask(taskID string) (*Item, error) {
	it, err := d.lookup(taskID)
	if err != nil {
		return nil, err
	}
	if it.Kind != Task {
		return nil, ErrNotTask
	}
	if it.IsSubtask() {
		return nil, ErrTooDeep
	}
	return it, nil
}

// closedSprint reports whether id names a done sprint.
func (d *Document) closedSprint(id string) bool {
	s, ok := d.SprintByID(id)
	return ok && s.Status == SprintDone
}

// Assign puts the top-level task taskID in sprint sprintID. A task from the
// backlog becomes New; one from another sprint keeps its state. changed is
// false, even for a done sprint, when the task is already there. A missing task is ErrNotFound and a
// missing sprint ErrSprintNotFound; a category is ErrNotTask and a subtask
// ErrTooDeep; a done target sprint, or a done sprint the task is in now, is
// ErrSprintClosed.
func (d *Document) Assign(taskID, sprintID string) (changed bool, err error) {
	it, err := d.sprintTask(taskID)
	if err != nil {
		return false, err
	}
	target, err := d.sprint(sprintID)
	if err != nil {
		return false, err
	}
	if it.Sprint == sprintID {
		return false, nil
	}
	if target.Status == SprintDone || d.closedSprint(it.Sprint) {
		return false, ErrSprintClosed
	}
	if it.Sprint == "" {
		it.Status = New
	}
	it.Sprint = sprintID
	return true, nil
}

// Unassign returns the top-level task taskID to the backlog, Open. A task
// already there gives changed false. A task in a done sprint is refused with
// ErrSprintClosed; a category is ErrNotTask and a subtask ErrTooDeep.
func (d *Document) Unassign(taskID string) (changed bool, err error) {
	it, err := d.sprintTask(taskID)
	if err != nil {
		return false, err
	}
	if it.Sprint == "" {
		return false, nil
	}
	if d.closedSprint(it.Sprint) {
		return false, ErrSprintClosed
	}
	it.Sprint = ""
	it.Status = Open
	return true, nil
}

// sprintDone returns the fully-done top-level tasks of sprint id.
func (d *Document) sprintDone(id string) []*Item {
	var out []*Item
	for _, it := range d.DoneSubtrees() {
		if it.Kind == Task && !it.IsSubtask() && it.Sprint == id {
			out = append(out, it)
		}
	}
	return out
}

// PruneSprint removes the fully-done top-level tasks of sprint id, as Prune
// does for the whole list, and returns how many tasks went. A done sprint may
// be pruned.
func (d *Document) PruneSprint(id string) int {
	n := 0
	for _, it := range d.sprintDone(id) {
		_, total := it.TaskCounts()
		n += 1 + total
		d.remove(it)
	}
	return n
}

// RemovableDoneIn returns how many tasks PruneSprint(id) would remove.
func (d *Document) RemovableDoneIn(id string) int {
	n := 0
	for _, it := range d.sprintDone(id) {
		_, total := it.TaskCounts()
		n += 1 + total
	}
	return n
}
