package todo

import "fmt"

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
	// Ref is the sprint's reference, "<prefix>-<n>" (TODO-001), given when it
	// is created (see Document.SprintRefPrefix) and never changed. Unlike a
	// name, which several sprints may share, it names one sprint.
	Ref string
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

// sprintByRef returns the sprint whose reference equals ref ignoring case. The
// empty reference never matches.
func (d *Document) sprintByRef(ref string) (*Sprint, bool) {
	if ref == "" {
		return nil, false
	}
	key := refKey(ref)
	for _, s := range d.Sprints {
		if s.Ref != "" && refKey(s.Ref) == key {
			return s, true
		}
	}
	return nil, false
}

// ResolveSprint returns the sprints key names: the one with that ID, else the
// one with that reference (ignoring case), else every sprint with that name
// (ignoring case), in creation order. Names may repeat, so a name can match
// more than one sprint; nil when nothing matches.
func (d *Document) ResolveSprint(key string) []*Sprint {
	if s, ok := d.SprintByID(key); ok {
		return []*Sprint{s}
	}
	if s, ok := d.sprintByRef(key); ok {
		return []*Sprint{s}
	}
	var out []*Sprint
	for _, s := range d.Sprints {
		if refKey(s.Name) == refKey(key) {
			out = append(out, s)
		}
	}
	return out
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
