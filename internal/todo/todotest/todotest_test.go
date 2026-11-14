package todotest_test

import (
	"testing"

	"github.com/candy-tools/todo/internal/todo"
	"github.com/candy-tools/todo/internal/todo/todotest"
)

func TestParseRenderRoundTrip(t *testing.T) {
	md := "# A\n\n- [x] done\n  - [ ] sub\n\n## B\n\n- [/] one\n  note\n\n  more\n"
	if got := todotest.Render(todotest.Parse(t, md)); got != md {
		t.Fatalf("got:\n%s\nwant:\n%s", got, md)
	}
}

func TestParseMovesLooseTasksToUncategorized(t *testing.T) {
	d := todotest.Parse(t, "- [ ] loose\n")
	if len(d.Roots) != 1 || d.Roots[0].Title != "Uncategorized" {
		t.Fatalf("roots = %v, want a single Uncategorized category", d.Roots)
	}
}

func TestRenderEmptyDocument(t *testing.T) {
	if got := todotest.Render(&todo.Document{}); got != "" {
		t.Errorf("Render(empty) = %q, want empty", got)
	}
}

func TestSeedDumpRoundTrip(t *testing.T) {
	md := "# A\n\n- [/] one\n  note\n  - [ ] sub\n\n## B\n\n- [>] two\n"
	if got := todotest.Dump(t, todotest.Seed(t, md)); got != md {
		t.Fatalf("got:\n%s\nwant:\n%s", got, md)
	}
}
