package todo

import "strings"

// markerFromStatus maps a Status to its checkbox marker char, the inverse of
// statusFromMarker (see markdown.go); the two must stay in sync.
func markerFromStatus(s Status) string {
	switch s {
	case Done:
		return "x"
	case InProgress:
		return "/"
	case Deferred:
		return ">"
	default: // Open
		return " "
	}
}

// Render serializes the document back to canonical markdown: the preamble (if
// any), then each item. Headers get a blank line of breathing room on each
// side; task lists stay tight; a task's description follows its checkbox line
// indented, before any sub-tasks. The output always ends in a single newline.
func (d *Document) Render() string {
	var lines []string
	if d.Preamble != "" {
		lines = append(lines, strings.Split(d.Preamble, "\n")...)
	}
	lines = appendItems(lines, d.Roots, 0)
	lines = collapseBlanks(lines)
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n") + "\n"
}

// appendItems renders a sibling list. taskDepth is the task-nesting depth of a
// task in this list (0 directly under a category, +1 under each parent task).
// A blank line is emitted before every header; collapseBlanks later folds any
// doubled-up blanks and trims a leading one, so callers can be liberal here.
func appendItems(lines []string, items []*Item, taskDepth int) []string {
	for _, it := range items {
		if it.Kind == Category {
			lines = append(lines, "") // breathing room before the header
			lines = append(lines, strings.Repeat("#", it.Level)+" "+it.Title)
			lines = append(lines, "")
			lines = appendItems(lines, it.Children, 0)
			continue
		}
		indent := strings.Repeat("  ", taskDepth)
		box := "[" + markerFromStatus(it.Status) + "]"
		lines = append(lines, indent+"- "+box+" "+it.Title)
		if it.Description != "" {
			for _, dl := range strings.Split(it.Description, "\n") {
				if dl == "" {
					lines = append(lines, "")
				} else {
					lines = append(lines, indent+"  "+dl)
				}
			}
		}
		lines = appendItems(lines, it.Children, taskDepth+1)
	}
	return lines
}

// collapseBlanks removes a leading blank line and folds any run of consecutive
// blank lines down to one, so the header spacing rules above never produce
// doubled blanks.
func collapseBlanks(lines []string) []string {
	out := lines[:0:0]
	prevBlank := false
	for _, l := range lines {
		blank := strings.TrimSpace(l) == ""
		if blank && (prevBlank || len(out) == 0) {
			continue
		}
		out = append(out, l)
		prevBlank = blank
	}
	// Trim a single trailing blank the fold may have left.
	for len(out) > 0 && strings.TrimSpace(out[len(out)-1]) == "" {
		out = out[:len(out)-1]
	}
	return out
}
