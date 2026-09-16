package cmd

import (
	"strings"
	"testing"
)

func TestAddTaskUnderCategory(t *testing.T) {
	path := writeTemp(t, "# Work\n\n- [ ] existing\n")
	if _, err := runCmd(t, "add", "--file", path, "--title", "fresh", "--parent-title", "Work"); err != nil {
		t.Fatalf("add: %v", err)
	}
	got := readFile(t, path)
	if !strings.Contains(got, "- [ ] fresh") {
		t.Errorf("new task missing:\n%s", got)
	}
}

func TestAddTaskGoesBeforeSubcategories(t *testing.T) {
	// The new task must land among Work's tasks, before its "## Sub" child, so
	// the file round-trips (a task after a subheader would re-parent on reload).
	path := writeTemp(t, "# Work\n\n- [ ] existing\n\n## Sub\n\n- [ ] s1\n")
	if _, err := runCmd(t, "add", "--file", path, "--title", "fresh", "--parent-title", "Work"); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, path)
	if strings.Index(got, "- [ ] fresh") > strings.Index(got, "## Sub") {
		t.Errorf("new task must precede the subcategory:\n%s", got)
	}
}

func TestAddSubtaskUnderTask(t *testing.T) {
	path := writeTemp(t, "# Work\n\n- [ ] parent\n")
	if _, err := runCmd(t, "add", "--file", path, "--title", "child", "--parent-title", "parent"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(readFile(t, path), "  - [ ] child") {
		t.Errorf("child should be nested under parent:\n%s", readFile(t, path))
	}
}

func TestAddWithStatusAndDesc(t *testing.T) {
	path := writeTemp(t, "# Work\n\n- [ ] a\n")
	if _, err := runCmd(t, "add", "--file", path, "--title", "b", "--parent-title", "Work", "--status", "progress", "--desc", "note"); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, path)
	if !strings.Contains(got, "- [/] b") || !strings.Contains(got, "note") {
		t.Errorf("want in-progress b with a description:\n%s", got)
	}
}

func TestAddErrors(t *testing.T) {
	path := writeTemp(t, "# Work\n\n- [ ] a\n")
	if _, err := runCmd(t, "add", "--file", path, "--parent-title", "Work"); codeOf(t, err) != exitUsage {
		t.Error("missing --title should be a usage error")
	}
	if _, err := runCmd(t, "add", "--file", path, "--title", "x", "--parent-title", "Work", "--status", "bogus"); codeOf(t, err) != exitUsage {
		t.Error("bad --status should be a usage error")
	}
	if _, err := runCmd(t, "add", "--file", path, "--title", "x"); codeOf(t, err) != exitUsage {
		t.Error("missing parent selector should be a usage error")
	}
}

func TestAddCategory(t *testing.T) {
	path := writeTemp(t, "# Work\n\n- [ ] a\n")
	if _, err := runCmd(t, "add-category", "--file", path, "--title", "Personal"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(readFile(t, path), "# Personal") {
		t.Errorf("top-level category missing:\n%s", readFile(t, path))
	}
}

func TestAddNestedCategory(t *testing.T) {
	path := writeTemp(t, "# Work\n\n- [ ] a\n")
	if _, err := runCmd(t, "add-category", "--file", path, "--title", "Backend", "--parent-title", "Work"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(readFile(t, path), "## Backend") {
		t.Errorf("nested category should render as an H2:\n%s", readFile(t, path))
	}
}
