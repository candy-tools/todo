package todo_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/candy-tools/todo/internal/todo"
	"github.com/candy-tools/todo/internal/todo/todotest"
)

// parse returns the raw parse tree of src, dropping the warnings.
func parse(src string) *todo.Document {
	d, _ := todo.ParseMarkdown(src)
	return d
}

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
				case todo.Working:
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
	d := parse(sample)
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
	d := parse(sample)
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

func TestParsePreambleIsDroppedWithOneWarning(t *testing.T) {
	src := "My todos\n\nsome intro\n\n# Work\n\n- [ ] a\n"
	d, warnings := todo.ParseMarkdown(src)
	if len(warnings) != 1 || warnings[0] != "line 1: text before the first heading is not imported" {
		t.Errorf("warnings = %q", warnings)
	}
	if len(d.Roots) != 1 || d.Roots[0].Title != "Work" {
		t.Fatalf("expected one Work category, got %s", treeString(d))
	}
}

func TestParseDeepNesting(t *testing.T) {
	src := "- [ ] a\n  - [ ] b\n    - [ ] c\n"
	d := parse(src)
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
	// Tasks with no enclosing header are top-level roots, not dropped prose.
	d, warnings := todo.ParseMarkdown("- [ ] loose task\n")
	if len(warnings) != 0 {
		t.Errorf("a task line must not produce warnings: %q", warnings)
	}
	if len(d.Roots) != 1 || !d.Roots[0].IsTask() {
		t.Fatalf("expected one root task, got %s", treeString(d))
	}
}

func TestParseUppercaseX(t *testing.T) {
	d := parse("- [X] done\n")
	if !d.Roots[0].IsDone() {
		t.Errorf("[X] should parse as done")
	}
}

func TestParseInProgressAndDeferred(t *testing.T) {
	d := parse("- [/] a\n- [>] b\n")
	if len(d.Roots) != 2 {
		t.Fatalf("both extended markers should parse as tasks, got %d roots:\n%s", len(d.Roots), treeString(d))
	}
	if got := d.Roots[0].Status; got != todo.Working {
		t.Errorf("[/] should parse as Working, got %v", got)
	}
	if got := d.Roots[1].Status; got != todo.Deferred {
		t.Errorf("[>] should parse as Deferred, got %v", got)
	}
}

func TestParseEmpty(t *testing.T) {
	d, warnings := todo.ParseMarkdown("")
	if len(d.Roots) != 0 || len(warnings) != 0 {
		t.Errorf("empty source should give an empty document")
	}
}

func TestParseSkipsGuideAndKeepsWarningLineNumbers(t *testing.T) {
	// guide (1-2), blank (3), # Work (4), stray (5)
	src := "<!-- todo:guide x\n-->\n\n# Work\nstray\n"
	d, warnings := todo.ParseMarkdown(src)
	if len(d.Roots) != 1 || d.Roots[0].Title != "Work" || len(d.Roots[0].Children) != 0 {
		t.Fatalf("guide must be skipped, got:\n%s", treeString(d))
	}
	if len(warnings) != 1 || warnings[0] != `line 5: unrecognised line dropped: "stray"` {
		t.Errorf("warnings = %q", warnings)
	}
}

func TestParseFencedBlockOutsideTaskIsDroppedProse(t *testing.T) {
	// A `#` comment inside a fenced block that is not under a task must not
	// become a category: the whole block is dropped prose, one warning a line.
	src := "## Work\n\n- [ ] a\n\n```sh\nnpm ci\n# install deps\n- [ ] not a task\n```\n\n- [ ] b\n\n### Sub\n\n- [ ] c\n"
	d, warnings := todo.ParseMarkdown(src)
	want := `H2 Work
  [ ] a
  [ ] b
  H3 Sub
    [ ] c
`
	if got := treeString(d); got != want {
		t.Errorf("tree mismatch:\n got:\n%s\nwant:\n%s", got, want)
	}
	wantWarn := []string{
		"line 5: unrecognised line dropped: \"```sh\"",
		`line 6: unrecognised line dropped: "npm ci"`,
		`line 7: unrecognised line dropped: "# install deps"`,
		`line 8: unrecognised line dropped: "- [ ] not a task"`,
		"line 9: unrecognised line dropped: \"```\"",
	}
	if strings.Join(warnings, "\n") != strings.Join(wantWarn, "\n") {
		t.Errorf("warnings:\n got: %q\nwant: %q", warnings, wantWarn)
	}
}

func TestParseFencedBlockInDescription(t *testing.T) {
	// A fence indented under a task is part of its description, header- and
	// checkbox-looking lines included. A tilde fence is closed only by a tilde
	// fence at least as long, so the inner ``` and short ~~~ are content.
	src := "# Work\n\n- [ ] a\n  Run:\n  ~~~~\n  # install deps\n  - [ ] not a task\n  ```\n  ~~~\n  ~~~~\n  - [ ] sub\n- [ ] b\n"
	d, warnings := todo.ParseMarkdown(src)
	if len(warnings) != 0 {
		t.Errorf("unexpected warnings: %q", warnings)
	}
	want := `H1 Work
  [ ] a desc="Run:\n~~~~\n# install deps\n- [ ] not a task\n` + "```" + `\n~~~\n~~~~"
    [ ] sub
  [ ] b
`
	if got := treeString(d); got != want {
		t.Errorf("tree mismatch:\n got:\n%s\nwant:\n%s", got, want)
	}
}

func TestParseUnclosedDescriptionFenceEndsWithTheTask(t *testing.T) {
	// A fence left open in a description ends with the description: the
	// dedented lines after it are parsed as structure again.
	src := "# Work\n\n- [ ] a\n  ```\n  # comment\n# Next\n\n- [ ] b\n"
	d, warnings := todo.ParseMarkdown(src)
	if len(warnings) != 0 {
		t.Errorf("unexpected warnings: %q", warnings)
	}
	want := "H1 Work\n  [ ] a desc=\"```\\n# comment\"\nH1 Next\n  [ ] b\n"
	if got := treeString(d); got != want {
		t.Errorf("tree mismatch:\n got:\n%s\nwant:\n%s", got, want)
	}
}

func TestParseDedentedClosingFenceClosesDescriptionFence(t *testing.T) {
	// A column-0 fence that would close the description's open block closes
	// it rather than opening a new top-level block, so nothing after it is
	// swallowed. The closing line itself is reported as dropped.
	src := "# Work\n- [ ] a\n  ```\n  code\n```\n- [ ] b\n# Home\n- [ ] c\n"
	d, warnings := todo.ParseMarkdown(src)
	want := "H1 Work\n  [ ] a desc=\"```\\ncode\"\n  [ ] b\nH1 Home\n  [ ] c\n"
	if got := treeString(d); got != want {
		t.Errorf("tree mismatch:\n got:\n%s\nwant:\n%s", got, want)
	}
	wantWarn := []string{"line 5: unrecognised line dropped: \"```\""}
	if strings.Join(warnings, "\n") != strings.Join(wantWarn, "\n") {
		t.Errorf("warnings:\n got: %q\nwant: %q", warnings, wantWarn)
	}
}

func TestParseUnclosedTopLevelFenceEndsAtNextHeadingAndWarns(t *testing.T) {
	// A top-level fence with no close ends just before the next column-0
	// heading (like an unclosed HTML comment), with a warning, so the
	// categories and tasks after it are not lost.
	src := "# Work\n- [ ] a\n~~~\n- [ ] b\n# Home\n- [ ] c\n"
	d, warnings := todo.ParseMarkdown(src)
	want := "H1 Work\n  [ ] a\nH1 Home\n  [ ] c\n"
	if got := treeString(d); got != want {
		t.Errorf("tree mismatch:\n got:\n%s\nwant:\n%s", got, want)
	}
	wantWarn := []string{
		"line 3: unclosed code fence: lines 3-4 are not imported",
		`line 3: unrecognised line dropped: "~~~"`,
		`line 4: unrecognised line dropped: "- [ ] b"`,
	}
	if strings.Join(warnings, "\n") != strings.Join(wantWarn, "\n") {
		t.Errorf("warnings:\n got: %q\nwant: %q", warnings, wantWarn)
	}
}

func TestParseFencedDescriptionRoundTrips(t *testing.T) {
	// import → render → import keeps a description holding a fenced block,
	// header- and checkbox-looking lines included, and the tree around it.
	src := "# Work\n\n- [ ] a\n  ```sh\n  # install deps\n  - [ ] not a task\n  ```\n  - [ ] sub\n\n## Sub\n\n- [ ] b\n"
	first := parse(src)
	again := parse(todotest.Render(first))
	if got, want := treeString(again), treeString(first); got != want {
		t.Errorf("round trip changed the tree:\n got:\n%s\nwant:\n%s", got, want)
	}
	if desc := first.Roots[0].Children[0].Description; desc != "```sh\n# install deps\n- [ ] not a task\n```" {
		t.Errorf("description = %q", desc)
	}
}

func TestParseTabIndentIsFourColumns(t *testing.T) {
	// A tab advances to the next multiple of 4 columns (the CommonMark tab
	// stop): a two-space task after a tab-indented one is its sibling, not its
	// child, and tab- and space-indented description lines dedent together.
	src := "# Work\n\n\t- [ ] a\n  - [ ] b\n- [ ] c\n\tfirst\n    second\n  \tthird\n"
	d, warnings := todo.ParseMarkdown(src)
	if len(warnings) != 0 {
		t.Errorf("unexpected warnings: %q", warnings)
	}
	want := "H1 Work\n  [ ] a\n  [ ] b\n  [ ] c desc=\"first\\nsecond\\nthird\"\n"
	if got := treeString(d); got != want {
		t.Errorf("tree mismatch:\n got:\n%s\nwant:\n%s", got, want)
	}
}
