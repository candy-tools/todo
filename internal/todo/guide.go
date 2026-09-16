package todo

import "strings"

// guideMarker is the sentinel that opens the app-managed guide block. Only a
// comment starting with this exact prefix is treated as the managed guide (and
// so stripped on load / rewritten on save); a user's own HTML comment is left
// alone.
const guideMarker = "<!-- todo:guide"

// guideComment is the app-managed guide written at the top of every saved file.
// It is an HTML comment, so it stays invisible when the markdown is rendered
// elsewhere but is plain to read in the raw file. It documents the format for
// whoever edits the file directly — human or agent — and points at the repo.
// The block is rewritten on every save, so it always reflects the current text.
const guideComment = `<!-- todo:guide — managed by todo; this block is rewritten on save. Docs: https://github.com/candy-tools/todo
This file is a todo list managed by "todo", a terminal TODO app:
https://github.com/candy-tools/todo

todo watches this file and reloads it automatically when it changes on disk, so
you — human or agent — can edit it directly in any editor. Keep to this format
so todo can parse what you write:

  # Heading           Headings ("#" to "######") are categories; they nest by
                      heading level.
  - [ ] Open task     A "- [ ]" line is an open task; "- [x]" marks it done.
  - [/] In progress   "- [/]" flags a task in progress, "- [>]" defers it.
  - [x] Done task     Tasks must live under a category heading.
    - [ ] Subtask     Indent by two spaces to nest a subtask under a task.
    Description text  An indented, non-checkbox line is the task's description.

Notes for editors:
- Text above the first heading (this block included) is preserved on save.
- todo rewrites the file into the canonical form above on every change, so any
  other free-form markdown placed between items is not kept.
-->`

// FileContent is the exact on-disk representation of the document: the managed
// guide block followed by the rendered markdown. It is what Save writes. The
// guide always leads, so a fresh or fully-emptied file still carries it.
func (d *Document) FileContent() string {
	body := d.Render()
	if body == "" {
		return guideComment + "\n"
	}
	return guideComment + "\n\n" + body
}

// guideRange reports the inclusive line range [start,end] of the managed guide
// block within lines (0-based), or ok=false when there is none. It is the scan
// Parse uses to skip the guide while keeping every other line's real index: the
// block runs from the line beginning with guideMarker through the next line
// containing the HTML-comment close.
func guideRange(lines []string) (start, end int, ok bool) {
	start = -1
	for i, l := range lines {
		if strings.HasPrefix(strings.TrimSpace(l), guideMarker) {
			start = i
			break
		}
	}
	if start == -1 {
		return 0, 0, false
	}
	end = start
	for j := start; j < len(lines); j++ {
		if strings.Contains(lines[j], "-->") {
			end = j
			break
		}
	}
	return start, end, true
}
