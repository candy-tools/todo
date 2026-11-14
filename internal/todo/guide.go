package todo

import "strings"

// guideMarker is the sentinel that opens the app-managed guide block that
// earlier versions wrote at the top of TODO.md. Only a comment starting with
// this exact prefix is treated as the managed guide (and so skipped on import);
// a user's own HTML comment is left alone.
const guideMarker = "<!-- todo:guide"

// guideRange reports the inclusive line range [start,end] of the managed guide
// block within lines (0-based), or ok=false when there is none. It is the scan
// parseMarkdown uses to skip the guide while keeping every other line's real
// index: the block runs from the line beginning with guideMarker through the
// next line containing the HTML-comment close.
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
