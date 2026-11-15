package todo

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	// headerRe matches a markdown header line: 1..6 leading '#', a space, then
	// the title (trimmed).
	headerRe = regexp.MustCompile(`^(#{1,6})[ \t]+(.*)$`)
	// checkboxRe matches a task line: optional indentation, a list bullet, a box
	// holding one status marker, then the title. Group 1 is the indentation,
	// group 2 the marker char, group 3 the title. The accepted marker set must
	// stay in sync with statusFromMarker: space (open), x/X (done), / (in
	// progress), > (deferred). Any other char fails the match, so a line the app
	// doesn't own is left as prose rather than mis-parsed as a task.
	checkboxRe = regexp.MustCompile(`^([ \t]*)[-*+][ \t]+\[([ xX/>])\][ \t]?(.*)$`)
	// fenceRe matches a code-fence line: optional indentation, then a run of 3+
	// backticks or 3+ tildes (group 1), then the rest of the line (group 2: the
	// info string when opening; only whitespace is allowed when closing).
	fenceRe = regexp.MustCompile("^[ \t]*(`{3,}|~{3,})(.*)$")
)

// fenceTracker follows fenced code blocks line by line, so the lines of a
// block are never read as headers or checkboxes.
type fenceTracker struct {
	open   bool
	char   byte // '`' or '~'
	length int  // the opening run's length; a closing run must be at least as long
	inDesc bool // the block opened inside the current task's description
}

// fenced advances the tracker past line and reports whether line is a fence
// line or fenced content. hasTask and taskIndent describe the current task: a
// block opened in its description ends with it (as a CommonMark list item
// closes its code block), so a dedented line is structure again.
func (f *fenceTracker) fenced(line string, hasTask bool, taskIndent int) bool {
	inDesc := hasTask && leadingWhitespace(line) > taskIndent
	if f.open && f.inDesc && !inDesc && strings.TrimSpace(line) != "" {
		f.open = false
	}
	if f.open {
		f.open = !f.closes(line)
		return true
	}
	char, n, ok := openFence(line)
	if ok {
		*f = fenceTracker{open: true, char: char, length: n, inDesc: inDesc}
	}
	return ok
}

// openFence reports whether line opens a fenced code block (CommonMark: 3+
// backticks or tildes; a backtick fence's info string may not hold a
// backtick), returning its char and run length.
func openFence(line string) (byte, int, bool) {
	m := fenceRe.FindStringSubmatch(line)
	if m == nil || (m[1][0] == '`' && strings.Contains(m[2], "`")) {
		return 0, 0, false
	}
	return m[1][0], len(m[1]), true
}

// closes reports whether line closes the open block: a run of its char at
// least as long, followed by nothing but whitespace.
func (f *fenceTracker) closes(line string) bool {
	m := fenceRe.FindStringSubmatch(line)
	return m != nil && m[1][0] == f.char && len(m[1]) >= f.length && strings.TrimSpace(m[2]) == ""
}

// statusFromMarker maps a checkbox marker char (checkboxRe's group 2) to a
// Status. It is the inverse of Status.Marker; the two must agree, and both with
// checkboxRe's accepted set.
func statusFromMarker(marker string) Status {
	switch marker {
	case "x", "X":
		return Done
	case "/":
		return Working
	case ">":
		return Deferred
	default: // " "
		return Open
	}
}

// parseMarkdown turns markdown source into a Document, for the one-time import.
// Headers become nested categories; `- [ ]` items become tasks nested by
// indentation; indented non-checkbox text under a task becomes that task's
// description. Lines inside a ``` or ~~~ fenced code block are never headers
// or checkboxes: indented under a task they are description, otherwise dropped
// prose. It also reports what it did not keep: one warning,
// `line N: <reason>` with the 1-based source line, for the first non-blank line
// of text before the first node and one for every line of prose it drops.
func parseMarkdown(src string) (*Document, []string) {
	doc := &Document{}
	var warnings []string
	warn := func(lineNo int, format string, args ...any) {
		warnings = append(warnings, fmt.Sprintf("line %d: ", lineNo)+fmt.Sprintf(format, args...))
	}
	preambleWarned := false
	lines := strings.Split(src, "\n")
	// The app-managed guide block that older TODO.md files carry is skipped
	// below rather than stripped up front, so its lines still count toward every
	// other line's real index in the warnings (see guideRange). It must be
	// excluded from parsing itself: it is an HTML comment whose own example
	// lines (`- [ ]`, `#`) would otherwise be mis-parsed as real tasks and
	// headers.
	gStart, gEnd, hasGuide := guideRange(lines)

	var catStack []*Item // open categories, by increasing Level
	type taskEntry struct {
		indent int
		item   *Item
	}
	var taskStack []taskEntry // open tasks, by increasing indentation
	var curTask *Item         // most recent task; receives description lines
	var curTaskIndent int
	var descBuf []string // buffered description lines for curTask
	var fences fenceTracker
	seenNode := false

	flushDesc := func() {
		if curTask != nil {
			curTask.Description = description(descBuf)
		}
		descBuf = nil
	}

	attach := func(it *Item) {
		if len(catStack) > 0 {
			catStack[len(catStack)-1].AppendChild(it)
		} else {
			doc.AppendRoot(it)
		}
	}

	for i, line := range lines {
		if inGuide(i, gStart, gEnd, hasGuide) {
			continue // the managed guide block: skip but keep counting lines
		}
		lineNo := i + 1
		var hm, cm []string // header and checkbox matches; none for fenced lines
		if !fences.fenced(line, curTask != nil, curTaskIndent) {
			hm, cm = headerRe.FindStringSubmatch(line), checkboxRe.FindStringSubmatch(line)
		}
		if m := hm; m != nil {
			flushDesc()
			curTask = nil
			taskStack = nil
			level := len(m[1])
			cat := &Item{Kind: Category, Level: level, Title: strings.TrimRight(m[2], " \t")}
			for len(catStack) > 0 && catStack[len(catStack)-1].Level >= level {
				catStack = catStack[:len(catStack)-1]
			}
			attach(cat)
			catStack = append(catStack, cat)
			seenNode = true
			continue
		}
		if m := cm; m != nil {
			flushDesc()
			indent := len(m[1])
			task := &Item{
				Kind:   Task,
				Title:  strings.TrimRight(m[3], " \t"),
				Status: statusFromMarker(m[2]),
			}
			for len(taskStack) > 0 && taskStack[len(taskStack)-1].indent >= indent {
				taskStack = taskStack[:len(taskStack)-1]
			}
			if len(taskStack) > 0 {
				taskStack[len(taskStack)-1].item.AppendChild(task)
			} else {
				attach(task)
			}
			taskStack = append(taskStack, taskEntry{indent: indent, item: task})
			curTask = task
			curTaskIndent = indent
			seenNode = true
			continue
		}
		// Neither a header nor a checkbox (or a fence line or fenced content).
		if curTask != nil {
			if strings.TrimSpace(line) == "" {
				descBuf = append(descBuf, "")
				continue
			}
			if leadingWhitespace(line) > curTaskIndent {
				descBuf = append(descBuf, line)
				continue
			}
			// A dedented non-blank line closes the current task's description.
			flushDesc()
			curTask = nil
		}
		trimmed := strings.TrimSpace(line)
		if !seenNode {
			if trimmed != "" && !preambleWarned {
				preambleWarned = true
				warn(lineNo, "text before the first heading is not imported")
			}
		} else if trimmed != "" {
			// Stray prose between items after a task closed — dropped.
			warn(lineNo, "unrecognised line dropped: %q", trimmed)
		}
	}
	flushDesc()
	return doc, warnings
}

// inGuide reports whether line index i falls inside the managed guide block
// [start,end].
func inGuide(i, start, end int, has bool) bool { return has && i >= start && i <= end }

// description turns the buffered description lines into the task description:
// trailing blank lines are dropped and the rest dedented. No lines (or only
// blank ones) give "".
func description(lines []string) string {
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) == 0 {
		return ""
	}
	return dedent(lines)
}

// leadingWhitespace counts the leading space/tab characters of s (each as one).
func leadingWhitespace(s string) int {
	return len(s) - len(strings.TrimLeft(s, " \t"))
}

// dedent strips the smallest common leading-whitespace prefix from the buffered
// description lines (blank lines ignored for the measurement, emitted empty)
// and joins them with newlines.
func dedent(lines []string) string {
	min := -1
	for _, l := range lines {
		if strings.TrimSpace(l) == "" {
			continue
		}
		if n := leadingWhitespace(l); min == -1 || n < min {
			min = n
		}
	}
	if min <= 0 {
		return strings.Join(lines, "\n")
	}
	out := make([]string, len(lines))
	for i, l := range lines {
		if strings.TrimSpace(l) == "" {
			out[i] = ""
		} else {
			out[i] = l[min:]
		}
	}
	return strings.Join(out, "\n")
}
