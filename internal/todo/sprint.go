package todo

import (
	"fmt"
	"strings"
)

// SprintStatus is a sprint's state. The list file stores it by name (see
// String), so the names are pinned.
type SprintStatus int

const (
	// SprintOpen is a sprint that is being worked on.
	SprintOpen SprintStatus = 0
	// SprintDone is a closed sprint.
	SprintDone SprintStatus = 1
)

var sprintStatusNames = map[SprintStatus]string{SprintOpen: "open", SprintDone: "done"}

// String returns the sprint status's name, "open" or "done", or
// "sprintstatus(N)" for an unknown value so it stays visible.
func (s SprintStatus) String() string {
	if n, ok := sprintStatusNames[s]; ok {
		return n
	}
	return fmt.Sprintf("sprintstatus(%d)", int(s))
}

// SprintStatusFromName parses a sprint status name; ok is false for an unknown
// name.
func SprintStatusFromName(name string) (SprintStatus, bool) {
	for s, n := range sprintStatusNames {
		if n == name {
			return s, true
		}
	}
	return SprintOpen, false
}

// Sprint is a time-boxed group of tasks. A top-level task joins one by its
// Item.Sprint; its subtasks follow it.
type Sprint struct {
	ID     string
	Name   string
	Goal   string
	Status SprintStatus
	// Prefix is the reference prefix of the tasks created in the sprint (see
	// Document.PrefixFor); "" for none.
	Prefix string
}

// InSprint reports whether a task in this state belongs to a sprint: every
// state outside the backlog column does.
func (s Status) InSprint() bool { return s.Column() != ColBacklog }

// SprintID returns the ID of the sprint the task belongs to: its own Sprint,
// or, for a subtask, its parent's. It is "" for a task in the backlog.
func (it *Item) SprintID() string {
	if it.IsSubtask() {
		return it.Parent.Sprint
	}
	return it.Sprint
}

// SprintByID returns the sprint with the given ID.
func (d *Document) SprintByID(id string) (*Sprint, bool) {
	for _, s := range d.Sprints {
		if s.ID == id {
			return s, true
		}
	}
	return nil, false
}

// sprintNameKey is the form two sprint names are compared in: names are unique
// in a list ignoring case.
func sprintNameKey(name string) string { return strings.ToLower(strings.ToUpper(name)) }

// SprintByName returns the sprint with the given name, ignoring case.
func (d *Document) SprintByName(name string) (*Sprint, bool) {
	key := sprintNameKey(name)
	for _, s := range d.Sprints {
		if sprintNameKey(s.Name) == key {
			return s, true
		}
	}
	return nil, false
}

// ResolveSprint finds a sprint by key: an ID first, then a name.
func (d *Document) ResolveSprint(key string) (*Sprint, bool) {
	if s, ok := d.SprintByID(key); ok {
		return s, true
	}
	return d.SprintByName(key)
}

// OpenSprints returns the sprints that are not done, in creation order.
func (d *Document) OpenSprints() []*Sprint {
	var out []*Sprint
	for _, s := range d.Sprints {
		if s.Status == SprintOpen {
			out = append(out, s)
		}
	}
	return out
}

// SprintTasks returns the top-level tasks of the sprint, in tree order.
func (d *Document) SprintTasks(id string) []*Item {
	var out []*Item
	for _, it := range d.Enumerate() {
		if it.Kind == Task && !it.IsSubtask() && it.Sprint == id {
			out = append(out, it)
		}
	}
	return out
}

// WorkQueue returns the sprint's top-level tasks to work on: those in progress,
// in tree order, and those next, Ready before New and in tree order within each
// state. Done tasks, and tasks in a backlog state, are left out.
func (d *Document) WorkQueue(id string) (inProgress, next []*Item) {
	var fresh []*Item
	for _, it := range d.SprintTasks(id) {
		switch {
		case it.Status.Column() == ColInProgress:
			inProgress = append(inProgress, it)
		case it.Status == Ready:
			next = append(next, it)
		case it.Status.Column() == ColNext:
			fresh = append(fresh, it)
		}
	}
	return inProgress, append(next, fresh...)
}

// Unfinished returns how many tasks and subtasks of the sprint are not done.
func (d *Document) Unfinished(id string) int {
	n := 0
	for _, t := range d.SprintTasks(id) {
		if !t.IsDone() {
			n++
		}
		for _, c := range t.Descendants() {
			if c.Kind == Task && !c.IsDone() {
				n++
			}
		}
	}
	return n
}
