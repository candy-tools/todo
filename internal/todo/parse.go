package todo

import (
	"os"
	"path/filepath"
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
)

// statusFromMarker maps a checkbox marker char (checkboxRe's group 2) to a
// Status. It is the inverse of markerFromStatus (see render.go); the two must
// agree, and both with checkboxRe's accepted set.
func statusFromMarker(marker string) Status {
	switch marker {
	case "x", "X":
		return Done
	case "/":
		return InProgress
	case ">":
		return Deferred
	default: // " "
		return Open
	}
}

// Load reads and parses the TODO file at path. A missing file is not an error:
// it yields an empty document, so `todo new-file.md` starts a fresh list that
// is written on the first save.
func Load(path string) (*Document, error) {
	b, err := os.ReadFile(path) //nolint:gosec // path is a user-provided CLI argument; reading it is the app's purpose
	if err != nil {
		if os.IsNotExist(err) {
			return &Document{}, nil
		}
		return nil, err
	}
	return Parse(string(b)), nil
}

// EnsureFile bootstraps path as a fresh, empty TODO file (the managed guide
// block and no tasks) when it does not already exist, so running todo creates
// the file up front rather than only writing it on the first edit.
func EnsureFile(path string) error {
	if _, err := os.Stat(path); err == nil {
		return nil // already exists — leave it (and any tasks in it) untouched
	} else if !os.IsNotExist(err) {
		return err // stat failed for another reason (e.g. permissions)
	}
	return (&Document{}).Save(path)
}

// Save writes the document back to path in canonical markdown form, led by the
// app-managed guide block (see FileContent). It writes to a temporary file in
// the same directory and renames it over the target: the rename is atomic, so a
// concurrent reader — notably the app's own change-poll (see the tui package) —
// never observes a half-written file.
func (d *Document) Save(path string) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".todo-*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	if _, err := f.WriteString(d.FileContent()); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	// CreateTemp makes the file 0600; todo files are meant to be user-readable.
	if err := os.Chmod(tmp, 0o644); err != nil { //nolint:gosec // 0644 is intentional: todo files are meant to be readable and greppable
		_ = os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, path)
}

// Parse turns markdown source into a Document. Headers become nested
// categories; `- [ ]` items become tasks nested by indentation; indented
// non-checkbox text under a task becomes that task's description. Text before
// the first header/task is kept verbatim as the preamble; stray non-indented
// prose elsewhere is dropped (the app owns the file format).
func Parse(src string) *Document {
	doc := &Document{}
	lines := strings.Split(src, "\n")
	// The app-managed guide block is skipped below rather than stripped up
	// front, so its lines still count toward every other line's real index
	// (see guideRange and Item.Line). It must still be excluded from parsing
	// itself: it is an HTML comment whose own example lines (`- [ ]`, `#`)
	// would otherwise be mis-parsed as real tasks and headers.
	gStart, gEnd, hasGuide := guideRange(lines)

	var catStack []*Item // open categories, by increasing Level
	type taskEntry struct {
		indent int
		item   *Item
	}
	var taskStack []taskEntry // open tasks, by increasing indentation
	var curTask *Item         // most recent task; receives description lines
	var curTaskIndent int
	var descBuf []string  // buffered description lines for curTask
	var preamble []string // verbatim lines before the first node
	seenNode := false

	flushDesc := func() {
		if curTask != nil {
			for len(descBuf) > 0 && strings.TrimSpace(descBuf[len(descBuf)-1]) == "" {
				descBuf = descBuf[:len(descBuf)-1]
			}
			if len(descBuf) > 0 {
				curTask.Description = dedent(descBuf)
			}
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
		if m := headerRe.FindStringSubmatch(line); m != nil {
			flushDesc()
			curTask = nil
			taskStack = nil
			level := len(m[1])
			cat := &Item{Kind: Category, Level: level, Title: strings.TrimRight(m[2], " \t"), Line: lineNo}
			for len(catStack) > 0 && catStack[len(catStack)-1].Level >= level {
				catStack = catStack[:len(catStack)-1]
			}
			attach(cat)
			catStack = append(catStack, cat)
			seenNode = true
			continue
		}
		if m := checkboxRe.FindStringSubmatch(line); m != nil {
			flushDesc()
			indent := len(m[1])
			task := &Item{
				Kind:   Task,
				Title:  strings.TrimRight(m[3], " \t"),
				Status: statusFromMarker(m[2]),
				Line:   lineNo,
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
		// Neither a header nor a checkbox.
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
		if !seenNode {
			preamble = append(preamble, line)
		}
		// Otherwise: stray prose between items after a task closed — dropped.
	}
	flushDesc()
	doc.Preamble = strings.Trim(strings.Join(preamble, "\n"), "\n")
	return doc
}

// inGuide reports whether line index i falls inside the managed guide block
// [start,end].
func inGuide(i, start, end int, has bool) bool { return has && i >= start && i <= end }

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
