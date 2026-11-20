package todo

import (
	"fmt"
	"maps"
	"slices"
)

// invalidf returns an ErrInvalidList with the message format makes of args.
func invalidf(format string, args ...any) error {
	return fmt.Errorf("%w: "+format, append([]any{ErrInvalidList}, args...)...)
}

// validate checks l against the format's rules and returns the first failure,
// in this order: version, malformed IDs, duplicate IDs, sprints, missing
// category parents, parent cycles, missing task categories, unknown statuses,
// missing task sprints, sprint membership, titles, references, reference
// prefixes. A file older
// than version 6 is upgraded before (see upgradeSprints), so its tasks in a
// sprint state pass the membership checks.
func (l *diskList) validate() error {
	if l.Version != fileVersion {
		return invalidf("unsupported format version %d", l.Version)
	}
	if err := l.validateIDs(); err != nil {
		return err
	}
	if l.View != nil {
		if _, ok := screenFromName(l.View.Screen); !ok && l.View.Screen != "" {
			return invalidf("view: unknown screen %q", l.View.Screen)
		}
	}
	if err := l.validateSprints(); err != nil {
		return err
	}
	parents, err := l.validateCategories()
	if err != nil {
		return err
	}
	for _, t := range l.Tasks {
		if _, ok := parents[t.Category]; !ok {
			return invalidf("task %s: category %q does not exist", t.ID, t.Category)
		}
	}
	for _, t := range l.Tasks {
		if _, ok := statusFromDisk(t.Status); !ok {
			return invalidf("task %s: unknown status %q", t.ID, t.Status)
		}
	}
	if err := l.validateTaskSprints(); err != nil {
		return err
	}
	if err := l.validateMembership(); err != nil {
		return err
	}
	if err := l.validateTitles(); err != nil {
		return err
	}
	if err := l.validateRefs(); err != nil {
		return err
	}
	return l.validatePrefixes()
}

// validateSprints checks every sprint: a well formed ID that no other sprint
// has, a name CleanTitle accepts unchanged (names may repeat), and a known
// status, in that order over all sprints.
func (l *diskList) validateSprints() error {
	ids := make(map[string]bool, len(l.Sprints))
	for i, s := range l.Sprints {
		if !validID(s.ID) {
			return invalidf("%s: malformed id %q: %s", titled("sprint", s.Name, fmt.Sprintf("sprints[%d]", i)), s.ID, idRule)
		}
		if ids[s.ID] {
			return invalidf("duplicate sprint id %s", s.ID)
		}
		ids[s.ID] = true
	}
	for _, s := range l.Sprints {
		clean, err := CleanTitle(s.Name)
		if err != nil {
			return invalidf("sprint %s: %v", s.ID, err)
		}
		if clean != s.Name {
			return invalidf("sprint %s: name has leading or trailing whitespace", s.ID)
		}
	}
	for _, s := range l.Sprints {
		if _, ok := SprintStatusFromName(s.Status); !ok {
			return invalidf("sprint %s: unknown status %q", s.ID, s.Status)
		}
	}
	return nil
}

// validateTaskSprints checks every task that names a sprint names one the list
// has.
func (l *diskList) validateTaskSprints() error {
	have := make(map[string]bool, len(l.Sprints))
	for _, s := range l.Sprints {
		have[s.ID] = true
	}
	for _, t := range l.Tasks {
		if t.Sprint != "" && !have[t.Sprint] {
			return invalidf("task %s: sprint %q does not exist", t.ID, t.Sprint)
		}
	}
	return nil
}

// validateMembership checks the membership rule on every task, in file order:
// a task in a sprint state has a sprint and one in a backlog state has none,
// and a done sprint holds only done tasks with done subtasks. The statuses
// and sprints must already be valid.
func (l *diskList) validateMembership() error {
	done := make(map[string]bool, len(l.Sprints))
	for _, s := range l.Sprints {
		done[s.ID] = s.Status == SprintDone.String()
	}
	for _, t := range l.Tasks {
		st, _ := statusFromDisk(t.Status)
		switch {
		case st.InSprint() && t.Sprint == "":
			return invalidf("task %s: status %q needs a sprint", t.ID, t.Status)
		case !st.InSprint() && t.Sprint != "":
			return invalidf("task %s: backlog status %q in sprint %s", t.ID, t.Status, t.Sprint)
		case !done[t.Sprint]:
			continue
		case st != Done:
			return invalidf("sprint %s is done but task %s is not", t.Sprint, t.ID)
		}
		for _, s := range t.Subtasks {
			if !s.Done {
				return invalidf("sprint %s is done but subtask %s of task %s is not", t.Sprint, s.ID, t.ID)
			}
		}
	}
	return nil
}

// validateRefs checks every task and sprint reference is one CleanRef accepts
// unchanged and that no two of them, tasks and sprints together, hold the
// same one, ignoring case. Tasks are checked first, then sprints.
func (l *diskList) validateRefs() error {
	seen := map[string]string{} // refKey to the task or sprint holding it
	check := func(owner, ref string) error {
		if ref == "" {
			return nil
		}
		clean, err := CleanRef(ref)
		if err != nil {
			return invalidf("%s: %v", owner, err)
		}
		if clean != ref {
			return invalidf("%s: reference has leading or trailing whitespace", owner)
		}
		if other, ok := seen[refKey(ref)]; ok {
			return invalidf("%s: reference %q is already used by %s", owner, ref, other)
		}
		seen[refKey(ref)] = owner
		return nil
	}
	for _, t := range l.Tasks {
		if err := check("task "+t.ID, t.Ref); err != nil {
			return err
		}
	}
	for _, s := range l.Sprints {
		if err := check("sprint "+s.ID, s.Ref); err != nil {
			return err
		}
	}
	return nil
}

// validatePrefixes checks every stored reference prefix, the list's, the
// sprints' and the categories', is one CleanPrefix returns unchanged and not
// empty, and every counter is at least 1 under such a prefix; counters are
// checked in prefix order. The sprints' prefix setting must not be the list's
// or a category's. A version 7 sprint's own prefix is ignored.
func (l *diskList) validatePrefixes() error {
	check := func(owner, p string) error {
		if clean, err := CleanPrefix(p); err != nil || clean != p || p == "" {
			return invalidf("%s: invalid reference prefix %q", owner, p)
		}
		return nil
	}
	if err := l.Refs.validate(check); err != nil {
		return err
	}
	sprint := ""
	if l.Refs != nil {
		sprint = l.Refs.SprintPrefix
	}
	for _, c := range l.Categories {
		if c.Prefix != "" {
			if err := check("category "+c.ID, c.Prefix); err != nil {
				return err
			}
			if c.Prefix == sprint {
				return invalidf("category %s: reference prefix %s is also the sprint reference prefix", c.ID, c.Prefix)
			}
		}
	}
	return nil
}

// validate checks the prefix and the counters of the refs section with check,
// which words a bad prefix for its owner; r may be nil. Counters are checked in
// prefix order.
func (r *diskRefs) validate(check func(owner, p string) error) error {
	if r == nil {
		return nil
	}
	if r.Prefix != "" {
		if err := check("refs", r.Prefix); err != nil {
			return err
		}
	}
	if r.SprintPrefix != "" {
		if err := check("refs", r.SprintPrefix); err != nil {
			return err
		}
		if r.SprintPrefix == r.Prefix {
			return invalidf("refs: reference prefix %s is both the list's and the sprint reference prefix", r.Prefix)
		}
	}
	for _, p := range slices.Sorted(maps.Keys(r.Last)) {
		if err := check("refs", p); err != nil {
			return err
		}
		if r.Last[p] < 1 {
			return invalidf("refs: counter for %s must be at least 1", p)
		}
	}
	return nil
}

// idRule says what a valid ID looks like (see validID).
var idRule = fmt.Sprintf("an id is %d characters from 0-9 and a-z except i, l, o and u", idLen)

// validateIDs checks every ID is well formed, then that none repeats. A
// malformed ID is reported with the item that holds it, named by title (or by
// its JSON path when it has none), since its ID cannot name it.
func (l *diskList) validateIDs() error {
	malformed := func(owner, id string) error {
		return invalidf("%s: malformed id %q: %s", owner, id, idRule)
	}
	var ids []string
	for i, c := range l.Categories {
		if !validID(c.ID) {
			return malformed(titled("category", c.Title, fmt.Sprintf("categories[%d]", i)), c.ID)
		}
		ids = append(ids, c.ID)
	}
	for i, t := range l.Tasks {
		// The name is built only for a malformed ID: this runs on every load
		// and write.
		task := func() string { return titled("task", t.Title, fmt.Sprintf("tasks[%d]", i)) }
		if !validID(t.ID) {
			return malformed(task(), t.ID)
		}
		ids = append(ids, t.ID)
		for j, s := range t.Subtasks {
			if !validID(s.ID) {
				if s.Title == "" {
					return malformed(fmt.Sprintf("tasks[%d].subtasks[%d]", i, j), s.ID)
				}
				return malformed(fmt.Sprintf("subtask %q of %s", s.Title, task()), s.ID)
			}
			ids = append(ids, s.ID)
		}
	}
	seen := make(map[string]bool, len(ids))
	for _, id := range ids {
		if seen[id] {
			return invalidf("duplicate id %s", id)
		}
		seen[id] = true
	}
	return nil
}

// validateCategories checks every parent exists and the parents form no cycle.
// It returns the category ID to parent ID map.
func (l *diskList) validateCategories() (map[string]string, error) {
	parents := make(map[string]string, len(l.Categories))
	for _, c := range l.Categories {
		parents[c.ID] = c.Parent
	}
	for _, c := range l.Categories {
		if _, ok := parents[c.Parent]; c.Parent != "" && !ok {
			return nil, invalidf("category %s: parent %q does not exist", c.ID, c.Parent)
		}
	}
	for _, c := range l.Categories {
		steps := 0
		for p := c.Parent; p != ""; p = parents[p] {
			if steps++; steps > len(parents) {
				return nil, invalidf("category %s: parent cycle", c.ID)
			}
		}
	}
	return parents, nil
}

// validateTitles checks every title, in file order.
func (l *diskList) validateTitles() error {
	for _, c := range l.Categories {
		if err := checkTitle("category", c.ID, c.Title); err != nil {
			return err
		}
	}
	for _, t := range l.Tasks {
		if err := checkTitle("task", t.ID, t.Title); err != nil {
			return err
		}
		for _, s := range t.Subtasks {
			if err := checkTitle("subtask", s.ID, s.Title); err != nil {
				return err
			}
		}
	}
	return nil
}

// titled names an item by kind and title, as `task "Buy milk"`, or by its
// JSON path when it has no title.
func titled(kind, title, path string) string {
	if title == "" {
		return path
	}
	return fmt.Sprintf("%s %q", kind, title)
}

// checkTitle fails when CleanTitle rejects title or would change it. CleanTitle
// changes a title it accepts only by trimming the whitespace around it.
func checkTitle(kind, id, title string) error {
	clean, err := CleanTitle(title)
	if err != nil {
		return invalidf("%s %s: %v", kind, id, err)
	}
	if clean != title {
		return invalidf("%s %s: title has leading or trailing whitespace", kind, id)
	}
	return nil
}
