package cmd

import (
	"strings"
	"testing"
)

func TestRmRemovesSubtree(t *testing.T) {
	path := writeTemp(t, "# Work\n\n- [ ] parent\n  - [ ] child\n- [ ] keep\n")
	if _, err := runCmd(t, "rm", "--file", path, "--title", "parent"); err != nil {
		t.Fatalf("rm: %v", err)
	}
	got := readFile(t, path)
	if strings.Contains(got, "parent") || strings.Contains(got, "child") {
		t.Errorf("parent and its child should be gone:\n%s", got)
	}
	if !strings.Contains(got, "- [ ] keep") {
		t.Errorf("sibling should remain:\n%s", got)
	}
}

func TestPruneRemovesCompleted(t *testing.T) {
	path := writeTemp(t, "# Work\n\n- [x] done1\n- [ ] open1\n- [x] done2\n")
	out, err := runCmd(t, "prune", "--file", path)
	if err != nil {
		t.Fatalf("prune: %v", err)
	}
	got := readFile(t, path)
	if strings.Contains(got, "done1") || strings.Contains(got, "done2") {
		t.Errorf("completed tasks should be pruned:\n%s", got)
	}
	if !strings.Contains(got, "- [ ] open1") {
		t.Errorf("open task should remain:\n%s", got)
	}
	if !strings.Contains(out, "2") {
		t.Errorf("prune should report the count removed, got %q", out)
	}
}
