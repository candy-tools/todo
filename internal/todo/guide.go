package todo

import "strings"

// guideMarker is the sentinel that opens the app-managed guide block that
// earlier versions wrote at the top of TODO.md. Only a comment starting with
// this exact prefix is treated as the managed guide (and so skipped silently on
// import); a user's own HTML comment is reported like other dropped text.
const guideMarker = "<!-- todo:guide"

// commentSpan reports whether lines[start] opens a block-level HTML comment (its
// trimmed text starts with "<!--") and, if so, the index of the comment's last
// line. The comment runs through the first line holding the "-->" close. An
// unclosed comment (closed=false) is taken to end just before the next
// top-level heading outside a fenced block (see nextHeading), or at EOF when
// none follows: its content (the guide's
// example `- [ ]` lines, say) is still not parsed, but the categories and tasks
// after it are not lost to a missing "-->".
func commentSpan(lines []string, start int) (end int, closed, ok bool) {
	trimmed := strings.TrimSpace(lines[start])
	if !strings.HasPrefix(trimmed, "<!--") {
		return 0, false, false
	}
	if strings.Contains(trimmed[len("<!--"):], "-->") {
		return start, true, true
	}
	for j := start + 1; j < len(lines); j++ {
		if strings.Contains(lines[j], "-->") {
			return j, true, true
		}
	}
	return nextHeading(lines, start) - 1, false, true
}
