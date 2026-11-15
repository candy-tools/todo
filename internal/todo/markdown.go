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
	end    int  // a top-level block's last line index (its close, see fenceSpan)
}

// fenced advances the tracker past lines[i] and reports whether it is a fence
// line or fenced content. hasTask and taskIndent describe the current task: a
// block opened in its description ends with it (as a CommonMark list item
// closes its code block), so a dedented line is structure again, except a
// dedented run that closes the block, which is taken as its close. A top-level
// block runs to its span's end (see fenceSpan); unclosedEnd is that end when
// lines[i] opens a top-level block with no close, and -1 otherwise.
func (f *fenceTracker) fenced(lines []string, i int, hasTask bool, taskIndent int) (fenced bool, unclosedEnd int) {
	line := lines[i]
	inDesc := hasTask && leadingWhitespace(line) > taskIndent
	switch {
	case !f.open:
	case !f.inDesc:
		if f.open = i <= f.end; f.open {
			return true, -1
		}
	case !inDesc && strings.TrimSpace(line) != "":
		f.open = false
		if f.closes(line) {
			return true, -1
		}
	default:
		f.open = !f.closes(line)
		return true, -1
	}
	char, n, ok := openFence(line)
	if !ok {
		return false, -1
	}
	*f = fenceTracker{open: true, char: char, length: n, inDesc: inDesc}
	if inDesc {
		return true, -1
	}
	end, closed := fenceSpan(lines, i)
	f.end = end
	if closed {
		return true, -1
	}
	return true, end
}

// fenceSpan reports the index of the last line of the fenced block lines[start]
// opens: its closing line, or, when it has none (closed=false), the line just
// before the next top-level heading (EOF when none follows), so an unclosed
// fence does not swallow the categories and tasks after it (as commentSpan
// does for an unclosed HTML comment). lines[start] must open a fence.
func fenceSpan(lines []string, start int) (end int, closed bool) {
	if end, ok := fenceClose(lines, start); ok {
		return end, true
	}
	return nextHeading(lines, start) - 1, false
}

// fenceClose reports the index of the line closing the fenced block that
// lines[start] opens, if it opens one that has a close.
func fenceClose(lines []string, start int) (int, bool) {
	char, n, ok := openFence(lines[start])
	if !ok {
		return 0, false
	}
	f := fenceTracker{char: char, length: n}
	for j := start + 1; j < len(lines); j++ {
		if f.closes(lines[j]) {
			return j, true
		}
	}
	return 0, false
}

// nextHeading returns the index of the first top-level heading after
// lines[start], skipping closed fenced blocks, or len(lines) when none follows.
func nextHeading(lines []string, start int) int {
	for j := start + 1; j < len(lines); j++ {
		if end, ok := fenceClose(lines, j); ok {
			j = end
			continue
		}
		if headerRe.MatchString(lines[j]) {
			return j
		}
	}
	return len(lines)
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

// lineTracker follows the fenced code blocks and HTML comments of the lines
// being parsed. A commented line is never a header or checkbox: the example
// lines of the app-managed guide block that older TODO.md files carry are
// skipped outright, and any other comment's lines are kept or dropped like
// fenced content.
type lineTracker struct {
	fences       fenceTracker
	commentEnd   int  // index of the open comment's last line; -1 before any
	commentGuide bool // the open comment is the managed guide block
}

// structural advances past lines[i] and reports whether it may be a header or
// checkbox (it is neither fenced nor commented) and whether it belongs to the
// managed guide. hasTask and taskIndent describe the current task (see
// fenceTracker.fenced). unclosed is called with what is unclosed and its last
// index when lines[i] opens a top-level code fence or an HTML comment that has
// no close (see fenceSpan and commentSpan).
func (t *lineTracker) structural(lines []string, i int, hasTask bool, taskIndent int, unclosed func(what string, end int)) (structural, guide bool) {
	if i <= t.commentEnd {
		return false, t.commentGuide
	}
	if fenced, end := t.fences.fenced(lines, i, hasTask, taskIndent); fenced {
		if end >= 0 {
			unclosed("code fence", end)
		}
		return false, false
	}
	end, closed, ok := commentSpan(lines, i)
	if !ok {
		return true, false
	}
	t.commentEnd = end
	t.commentGuide = strings.HasPrefix(strings.TrimSpace(lines[i]), guideMarker)
	if !closed {
		unclosed("HTML comment", end)
	}
	return false, t.commentGuide
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
// prose; the same holds for lines inside an HTML comment. It also reports what
// it did not keep: one warning, `line N: <reason>` with the 1-based source
// line, for the first non-blank line of text before the first node, one for
// every line of prose it drops, one for each unclosed HTML comment, and one for
// each change it makes to a title so the store accepts it (see importTitle).
func parseMarkdown(src string) (*Document, []string) {
	doc := &Document{}
	var warnings []string
	warn := func(lineNo int, format string, args ...any) {
		warnings = append(warnings, fmt.Sprintf("line %d: ", lineNo)+fmt.Sprintf(format, args...))
	}
	// title makes a parsed title one the store accepts, warning for each change.
	title := func(lineNo int, raw string) string {
		return importTitle(raw, func(note string) { warn(lineNo, "%s", note) })
	}
	preambleWarned := false
	lines := strings.Split(src, "\n")

	var catStack []*Item // open categories, by increasing Level
	type taskEntry struct {
		indent int
		item   *Item
	}
	var taskStack []taskEntry // open tasks, by increasing indentation
	var curTask *Item         // most recent task; receives description lines
	var curTaskIndent int
	var descBuf []string // buffered description lines for curTask
	// Fenced and commented lines are never headers or checkboxes (see
	// lineTracker). Lines are skipped rather than stripped up front, so every
	// line keeps its real index in the warnings.
	tracker := lineTracker{commentEnd: -1}
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
		lineNo := i + 1
		structural, guide := tracker.structural(lines, i, curTask != nil, curTaskIndent, func(what string, end int) {
			warn(lineNo, "unclosed %s: lines %d-%d are not imported", what, lineNo, end+1)
		})
		if guide {
			continue // the managed guide block: skip but keep counting lines
		}
		var hm, cm []string // header and checkbox matches; none for fenced or commented lines
		if structural {
			hm, cm = headerRe.FindStringSubmatch(line), checkboxRe.FindStringSubmatch(line)
		}
		if m := hm; m != nil {
			flushDesc()
			curTask = nil
			taskStack = nil
			level := len(m[1])
			cat := &Item{Kind: Category, Level: level, Title: title(lineNo, m[2])}
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
			indent := leadingWhitespace(m[1])
			task := &Item{
				Kind:   Task,
				Title:  title(lineNo, m[3]),
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

// tabStop is the column multiple a tab advances to (the CommonMark tab stop).
const tabStop = 4

// leadingWhitespace measures the indentation of s in columns: a space is one,
// a tab advances to the next multiple of tabStop.
func leadingWhitespace(s string) int {
	col := 0
	for _, c := range s {
		switch c {
		case ' ':
			col++
		case '\t':
			col += tabStop - col%tabStop
		default:
			return col
		}
	}
	return col
}

// stripIndent removes n columns of indentation from s, which must be indented
// at least that far. Indentation holding a tab is rewritten as spaces, so what
// remains keeps its width wherever a tab straddled column n.
func stripIndent(s string, n int) string {
	ws := len(s) - len(strings.TrimLeft(s, " \t"))
	if !strings.Contains(s[:ws], "\t") {
		return s[n:]
	}
	return strings.Repeat(" ", leadingWhitespace(s)-n) + s[ws:]
}

// dedent strips the smallest common indentation (in columns, see
// leadingWhitespace) from the buffered description lines (blank lines ignored for the measurement, emitted empty)
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
			out[i] = stripIndent(l, min)
		}
	}
	return strings.Join(out, "\n")
}
