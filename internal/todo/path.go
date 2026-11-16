package todo

// Path returns the titles from the root down to it (inclusive). The CLI's JSON
// output uses it to report each item's ancestry; selection and fold state are
// restored across a reload by item ID, not by title path.
func (it *Item) Path() []string {
	var p []string
	for n := it; n != nil; n = n.Parent {
		p = append([]string{n.Title}, p...)
	}
	return p
}
