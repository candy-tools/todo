package todo_test

import (
	"strings"
	"testing"

	"github.com/candy-tools/todo/internal/todo"
)

// guideSample is a representative managed guide block as older TODO.md files
// carry it: it starts with the recognised marker and ends with the HTML-comment
// close. The exact prose is not significant; the parser only has to recognise
// and skip it.
const guideSample = `<!-- todo:guide — managed by todo; this block is rewritten on save
This is a todo list for https://github.com/candy-tools/todo.
- [ ] example task inside the guide
# Example heading inside the guide
-->`

func TestParseSkipsManagedGuide(t *testing.T) {
	src := guideSample + "\n\n# Work\n\n- [ ] a\n"
	d, warnings := todo.ParseMarkdown(src)
	if len(warnings) != 0 {
		t.Errorf("the managed guide must not produce warnings, got %q", warnings)
	}
	if len(d.Roots) != 1 || d.Roots[0].Title != "Work" || len(d.Roots[0].Children) != 1 {
		t.Fatalf("expected only the Work category with task a, got:\n%s", treeString(d))
	}
}

// A user's own HTML comment (not the managed marker) is not the guide, so it is
// reported as dropped text before the first heading.
func TestParseDoesNotTreatForeignCommentAsGuide(t *testing.T) {
	src := "<!-- just my note -->\n\n# Work\n\n- [ ] a\n"
	_, warnings := todo.ParseMarkdown(src)
	if len(warnings) != 1 || !strings.HasPrefix(warnings[0], "line 1:") {
		t.Errorf("a non-managed comment must be reported, got %q", warnings)
	}
}

// An unclosed managed guide (truncated or hand-edited, no `-->`) is taken to end
// just before the next top-level heading: the guide's own example lines (the
// indented `- [ ]` samples) are not imported, yet every real category and task
// below is kept, and the import warns about the unclosed comment. Skipping to
// EOF, as CommonMark would, would silently drop the whole list.
func TestParseUnclosedGuideEndsAtNextHeadingAndWarns(t *testing.T) {
	src := "<!-- todo:guide — managed by todo\n" +
		"  # Heading           Headings are categories.\n" +
		"  - [ ] Open task     A \"- [ ]\" line is an open task.\n" +
		"    - [ ] Subtask     Indent to nest.\n" +
		"\n# Work\n\n- [ ] a\n"
	d, warnings := todo.ParseMarkdown(src)
	if got, want := treeString(d), "H1 Work\n  [ ] a\n"; got != want {
		t.Fatalf("tree:\n got %q\nwant %q", got, want)
	}
	if len(warnings) != 1 || !strings.HasPrefix(warnings[0], "line 1:") || !strings.Contains(warnings[0], "unclosed") {
		t.Errorf("want one line-1 warning about the unclosed comment, got %q", warnings)
	}
}

// A user's own multi-line HTML comment hides its content the same way: the
// `- [ ]` and `#` lines inside it are not tasks or categories.
func TestParseMultiLineCommentContentIsNotParsed(t *testing.T) {
	src := "# Work\n\n- [ ] a\n<!-- old items\n- [ ] commented out\n# Commented heading\n-->\n- [ ] b\n"
	d, _ := todo.ParseMarkdown(src)
	if got, want := treeString(d), "H1 Work\n  [ ] a\n  [ ] b\n"; got != want {
		t.Fatalf("tree:\n got %q\nwant %q", got, want)
	}
}

// An unclosed comment with no heading after it runs to EOF: its lines are
// dropped (reported as such), never imported as tasks.
func TestParseUnclosedCommentAtEndRunsToEOF(t *testing.T) {
	src := "# Work\n\n- [ ] a\n<!-- note\n- [ ] hidden\n"
	d, warnings := todo.ParseMarkdown(src)
	if got, want := treeString(d), "H1 Work\n  [ ] a\n"; got != want {
		t.Fatalf("tree:\n got %q\nwant %q", got, want)
	}
	if len(warnings) == 0 || !strings.Contains(warnings[0], "line 4: unclosed HTML comment: lines 4-6") {
		t.Errorf("want a line-4 unclosed-comment warning first, got %q", warnings)
	}
}
