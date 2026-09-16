package todo

// Enumerate returns every item in document order — pre-order, parents before
// children — which is the order `todo list` prints them. An item's 1-based
// position in this slice is its list "number": Enumerate()[n-1] is item n.
func (d *Document) Enumerate() []*Item {
	var out []*Item
	var walk func(items []*Item)
	walk = func(items []*Item) {
		for _, it := range items {
			out = append(out, it)
			walk(it.Children)
		}
	}
	walk(d.Roots)
	return out
}

// ByNumber returns the item at 1-based list position n (see Enumerate), or
// ok=false when n is out of range.
func (d *Document) ByNumber(n int) (*Item, bool) {
	items := d.Enumerate()
	if n < 1 || n > len(items) {
		return nil, false
	}
	return items[n-1], true
}

// ByLine returns the item whose source line (Item.Line) equals line, or
// ok=false when no item begins on that line.
func (d *Document) ByLine(line int) (*Item, bool) {
	for _, it := range d.Enumerate() {
		if it.Line == line {
			return it, true
		}
	}
	return nil, false
}

// ByTitle returns every item whose title equals title exactly (case-sensitive),
// in document order. Zero results means no such item; more than one means the
// title is ambiguous and the caller must disambiguate by number or line.
func (d *Document) ByTitle(title string) []*Item {
	var out []*Item
	for _, it := range d.Enumerate() {
		if it.Title == title {
			out = append(out, it)
		}
	}
	return out
}
