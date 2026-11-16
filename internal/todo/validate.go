package todo

import "fmt"

// invalidf returns an ErrInvalidList with the message format makes of args.
func invalidf(format string, args ...any) error {
	return fmt.Errorf("%w: "+format, append([]any{ErrInvalidList}, args...)...)
}

// validate checks l against the format's rules and returns the first failure,
// in this order: version, malformed IDs, duplicate IDs, missing category
// parents, parent cycles, missing task categories, unknown statuses, titles.
func (l *diskList) validate() error {
	if l.Version != fileVersion {
		return invalidf("unsupported format version %d", l.Version)
	}
	if err := l.validateIDs(); err != nil {
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
	return l.validateTitles()
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
		task := titled("task", t.Title, fmt.Sprintf("tasks[%d]", i))
		if !validID(t.ID) {
			return malformed(task, t.ID)
		}
		ids = append(ids, t.ID)
		for j, s := range t.Subtasks {
			if !validID(s.ID) {
				if s.Title == "" {
					return malformed(fmt.Sprintf("tasks[%d].subtasks[%d]", i, j), s.ID)
				}
				return malformed(fmt.Sprintf("subtask %q of %s", s.Title, task), s.ID)
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
