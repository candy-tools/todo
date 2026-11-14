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
