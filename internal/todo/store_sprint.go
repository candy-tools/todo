package todo

import "context"

// AddSprint adds an open sprint and returns its ID; it gets the next sprint
// reference (see Document.AddSprint). A name CleanTitle refuses fails it with
// ErrInvalidTitle.
func (s *Store) AddSprint(ctx context.Context, name, goal string) (string, WriteResult, error) {
	return s.add(ctx, func(d *Document) (string, error) { return d.AddSprint(name, goal) })
}

// EditSprint sets the fields of sprint id that c names, in the list as it is at
// write time. A missing sprint is ErrSprintNotFound; a name fails as for
// AddSprint. An edit that changes nothing writes nothing.
func (s *Store) EditSprint(ctx context.Context, id string, c SprintChanges) (WriteResult, error) {
	return s.write(ctx, func(d *Document) (bool, error) { return d.EditSprint(id, c) })
}

// DeleteSprint removes sprint id and all its tasks, as Document.DeleteSprint
// does. A sprint with tasks not done fails with a SprintNotDoneError; a
// missing one with ErrSprintNotFound.
func (s *Store) DeleteSprint(ctx context.Context, id string) (WriteResult, error) {
	return s.change(ctx, func(d *Document) error { return d.DeleteSprint(id) })
}

// Assign puts task taskID in sprint sprintID, as Document.Assign does, on the
// list as it is at write time. A task already there writes nothing.
func (s *Store) Assign(ctx context.Context, taskID, sprintID string) (WriteResult, error) {
	return s.write(ctx, func(d *Document) (bool, error) { return d.Assign(taskID, sprintID) })
}

// Unassign returns task taskID to the backlog, as Document.Unassign does. A
// task already there writes nothing.
func (s *Store) Unassign(ctx context.Context, taskID string) (WriteResult, error) {
	return s.write(ctx, func(d *Document) (bool, error) { return d.Unassign(taskID) })
}

// PruneSprint removes the fully-done tasks of sprint id and returns how many
// tasks went, found in the list as it is at write time. A missing sprint is
// ErrSprintNotFound; when nothing is done, nothing is written.
func (s *Store) PruneSprint(ctx context.Context, id string) (removed int, res WriteResult, err error) {
	res, err = s.write(ctx, func(d *Document) (bool, error) {
		if _, err := d.sprint(id); err != nil {
			return false, err
		}
		removed = d.PruneSprint(id)
		return removed > 0, nil
	})
	if err != nil {
		return 0, WriteResult{}, err
	}
	return removed, res, nil
}
