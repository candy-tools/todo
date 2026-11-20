package cmd

import (
	"strings"
	"testing"

	"github.com/candy-tools/todo/internal/todo"
	"github.com/candy-tools/todo/internal/todo/todotest"
)

func TestSettingsShowSetClear(t *testing.T) {
	path := todotest.Seed(t, "# Work\n")
	out, err := runCmd(t, "settings", "--file", path)
	if err != nil || strings.TrimSpace(out) != "ref-prefix: (none)" {
		t.Fatalf("show = %q, %v", out, err)
	}
	out, err = runCmd(t, "settings", "--file", path, "--ref-prefix", "todo")
	if err != nil || strings.TrimSpace(out) != "ref-prefix: TODO" {
		t.Fatalf("set = %q, %v", out, err)
	}
	doc, _, err := todo.NewStore(path).Load(t.Context())
	if err != nil || doc.RefPrefix != "TODO" {
		t.Fatalf("file prefix = %q, %v", doc.RefPrefix, err)
	}
	out, err = runCmd(t, "settings", "--file", path, "--json")
	if err != nil || strings.TrimSpace(out) != `{"refPrefix":"TODO"}` {
		t.Fatalf("json = %q, %v", out, err)
	}
	out, err = runCmd(t, "settings", "--file", path, "--ref-prefix", "")
	if err != nil || strings.TrimSpace(out) != "ref-prefix: (none)" {
		t.Fatalf("clear = %q, %v", out, err)
	}
	out, err = runCmd(t, "settings", "--file", path, "--json")
	if err != nil || strings.TrimSpace(out) != `{"refPrefix":""}` {
		t.Fatalf("json none = %q, %v", out, err)
	}
	_, err = runCmd(t, "settings", "--file", path, "--ref-prefix", "1x")
	if err == nil || codeOf(t, err) != exitUsage || !strings.Contains(err.Error(), todo.ErrInvalidPrefix.Error()) {
		t.Fatalf("invalid: %v", err)
	}
}

func TestAddCategoryPrefix(t *testing.T) {
	path := todotest.Seed(t, "")
	if _, err := runCmd(t, "add-category", "--file", path, "--title", "Work", "--prefix", "work"); err != nil {
		t.Fatal(err)
	}
	if _, err := runCmd(t, "add", "--file", path, "--parent-title", "Work", "--title", "t"); err != nil {
		t.Fatal(err)
	}
	if got := refOf(t, path, "t"); got != "WORK-001" {
		t.Errorf("ref = %q", got)
	}
	_, err := runCmd(t, "add-category", "--file", path, "--title", "Bad", "--prefix", "1x")
	if codeOf(t, err) != exitUsage {
		t.Errorf("invalid prefix: %v", err)
	}
}

func TestEditSetPrefix(t *testing.T) {
	path := todotest.Seed(t, "# Work\n\n- [ ] a\n")
	if _, err := runCmd(t, "edit", "--file", path, "--title", "Work", "--set-prefix", "wk"); err != nil {
		t.Fatal(err)
	}
	if _, err := runCmd(t, "add", "--file", path, "--parent-title", "Work", "--title", "t"); err != nil {
		t.Fatal(err)
	}
	if got := refOf(t, path, "t"); got != "WK-001" {
		t.Errorf("ref = %q", got)
	}
	_, err := runCmd(t, "edit", "--file", path, "--title", "a", "--set-prefix", "X")
	if codeOf(t, err) != exitWrongKind {
		t.Errorf("task: %v", err)
	}
}

func TestSprintPrefix(t *testing.T) {
	path := todotest.Seed(t, "# Work\n")
	if _, err := runCmd(t, "sprint", "add", "--file", path, "--name", "S", "--prefix", "spr"); err != nil {
		t.Fatal(err)
	}
	if _, err := runCmd(t, "add", "--file", path, "--parent-title", "Work", "--sprint", "S", "--title", "t"); err != nil {
		t.Fatal(err)
	}
	if got := refOf(t, path, "t"); got != "SPR-001" {
		t.Errorf("ref = %q", got)
	}
	out, err := runCmd(t, "sprint", "list", "--file", path, "--json")
	if err != nil || !strings.Contains(out, `"prefix": "SPR"`) {
		t.Errorf("list json = %q, %v", out, err)
	}
	if _, err := runCmd(t, "sprint", "edit", "--file", path, "--sprint", "S", "--set-prefix", ""); err != nil {
		t.Fatal(err)
	}
	out, _ = runCmd(t, "sprint", "list", "--file", path, "--json")
	if strings.Contains(out, "prefix") {
		t.Errorf("prefix not cleared: %q", out)
	}
}

func TestAddRefRefusedWithPrefix(t *testing.T) {
	path := todotest.Seed(t, "# Work\n")
	if _, err := runCmd(t, "settings", "--file", path, "--ref-prefix", "TODO"); err != nil {
		t.Fatal(err)
	}
	_, err := runCmd(t, "add", "--file", path, "--parent-title", "Work", "--title", "t", "--ref", "X-1")
	if codeOf(t, err) != exitUsage || !strings.Contains(err.Error(), todo.ErrRefGenerated.Error()) {
		t.Fatalf("err = %v", err)
	}
	doc, _, _ := todo.NewStore(path).Load(t.Context())
	if len(doc.ByTitle("t")) != 0 {
		t.Error("task was added")
	}
}

func TestListJSONPrefix(t *testing.T) {
	path := todotest.Seed(t, "# Work\n\n# Home\n")
	if _, err := runCmd(t, "edit", "--file", path, "--title", "Work", "--set-prefix", "WORK"); err != nil {
		t.Fatal(err)
	}
	out, err := runCmd(t, "list", "--file", path, "--json")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(out, `"prefix"`) != 1 || !strings.Contains(out, `"prefix": "WORK"`) {
		t.Errorf("out = %s", out)
	}
}
