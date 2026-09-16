package todo_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/candy-tools/todo/internal/todo"
)

// treeString renders a document's structure into a compact, indentation-based
// form for readable assertions: "H<level> <title>" for categories and
// "[ ]/[x] <title>" (plus desc=…) for tasks.
func treeString(d *todo.Document) string {
	var b strings.Builder
	var walk func(items []*todo.Item, depth int)
	walk = func(items []*todo.Item, depth int) {
		for _, it := range items {
			b.WriteString(strings.Repeat("  ", depth))
			if it.Kind == todo.Category {
				fmt.Fprintf(&b, "H%d %s\n", it.Level, it.Title)
			} else {
				box := " "
				switch it.Status {
				case todo.InProgress:
					box = "/"
				case todo.Deferred:
					box = ">"
				case todo.Done:
					box = "x"
				}
				fmt.Fprintf(&b, "[%s] %s", box, it.Title)
				if it.Description != "" {
					fmt.Fprintf(&b, " desc=%q", it.Description)
				}
				b.WriteString("\n")
			}
			walk(it.Children, depth+1)
		}
	}
	walk(d.Roots, 0)
	return b.String()
}

const sample = `# Work

- [ ] Ship v1.0 release
  The release notes, tag, and
  announcement blog post.
  - [x] Write changelog
  - [ ] Cut the git tag
- [x] Fix login bug

## Backend

- [ ] Migrate DB

# Personal

- [ ] Renew passport
`

func TestParse(t *testing.T) {
	d := todo.Parse(sample)
	got := treeString(d)
	want := `H1 Work
  [ ] Ship v1.0 release desc="The release notes, tag, and\nannouncement blog post."
    [x] Write changelog
    [ ] Cut the git tag
  [x] Fix login bug
  H2 Backend
    [ ] Migrate DB
H1 Personal
  [ ] Renew passport
`
	if got != want {
		t.Errorf("tree mismatch:\n got:\n%s\nwant:\n%s", got, want)
	}
}

func TestParseParentPointers(t *testing.T) {
	d := todo.Parse(sample)
	work := d.Roots[0]
	if work.Parent != nil {
		t.Errorf("root category should have nil parent")
	}
	ship := work.Children[0]
	if ship.Parent != work {
		t.Errorf("Ship's parent should be Work")
	}
	changelog := ship.Children[0]
	if changelog.Parent != ship {
		t.Errorf("subtask parent should be its task")
	}
	backend := work.Children[2]
	if backend.Kind != todo.Category || backend.Parent != work {
		t.Errorf("Backend should be an H2 child of Work, got kind=%v parent=%v", backend.Kind, backend.Parent)
	}
}

func TestParsePreamble(t *testing.T) {
	src := "My todos\n\nsome intro\n\n# Work\n\n- [ ] a\n"
	d := todo.Parse(src)
	if d.Preamble != "My todos\n\nsome intro" {
		t.Errorf("preamble = %q", d.Preamble)
	}
	if len(d.Roots) != 1 || d.Roots[0].Title != "Work" {
		t.Fatalf("expected one Work category, got %s", treeString(d))
	}
}

func TestParseDeepNesting(t *testing.T) {
	src := "- [ ] a\n  - [ ] b\n    - [ ] c\n"
	d := todo.Parse(src)
	a := d.Roots[0]
	b := a.Children[0]
	c := b.Children[0]
	if a.Title != "a" || b.Title != "b" || c.Title != "c" {
		t.Fatalf("nesting wrong:\n%s", treeString(d))
	}
	if len(c.Children) != 0 {
		t.Errorf("c should be a leaf")
	}
}

func TestParseTasksBeforeAnyHeader(t *testing.T) {
	// Tasks with no enclosing header are top-level roots, not preamble.
	d := todo.Parse("- [ ] loose task\n")
	if d.Preamble != "" {
		t.Errorf("a task line must not become preamble: %q", d.Preamble)
	}
	if len(d.Roots) != 1 || !d.Roots[0].IsTask() {
		t.Fatalf("expected one root task, got %s", treeString(d))
	}
}

func TestParseUppercaseX(t *testing.T) {
	d := todo.Parse("- [X] done\n")
	if !d.Roots[0].IsDone() {
		t.Errorf("[X] should parse as done")
	}
}

func TestParseInProgressAndDeferred(t *testing.T) {
	d := todo.Parse("- [/] a\n- [>] b\n")
	if len(d.Roots) != 2 {
		t.Fatalf("both extended markers should parse as tasks, got %d roots:\n%s", len(d.Roots), treeString(d))
	}
	if got := d.Roots[0].Status; got != todo.InProgress {
		t.Errorf("[/] should parse as InProgress, got %v", got)
	}
	if got := d.Roots[1].Status; got != todo.Deferred {
		t.Errorf("[>] should parse as Deferred, got %v", got)
	}
}

func TestParseEmpty(t *testing.T) {
	d := todo.Parse("")
	if len(d.Roots) != 0 || d.Preamble != "" {
		t.Errorf("empty source should give an empty document")
	}
}

func TestParseTracksLinesWithoutGuide(t *testing.T) {
	// # Work (1), blank (2), - [ ] a (3)
	d := todo.Parse("# Work\n\n- [ ] a\n")
	if got := d.Roots[0].Line; got != 1 {
		t.Errorf("Work header line = %d, want 1", got)
	}
	if got := d.Roots[0].Children[0].Line; got != 3 {
		t.Errorf("task a line = %d, want 3", got)
	}
}

func TestParseTracksLinesCountingGuide(t *testing.T) {
	// guide (1-2), blank (3), # Work (4), blank (5), - [ ] a (6), "  - [x] b" (7)
	src := "<!-- todo:guide x\n-->\n\n# Work\n\n- [ ] a\n  - [x] b\n"
	d := todo.Parse(src)
	work := d.Roots[0]
	a := work.Children[0]
	b := a.Children[0]
	if work.Line != 4 || a.Line != 6 || b.Line != 7 {
		t.Errorf("lines = work:%d a:%d b:%d, want 4/6/7", work.Line, a.Line, b.Line)
	}
	if d.Preamble != "" {
		t.Errorf("guide must still be stripped from the preamble, got %q", d.Preamble)
	}
}
