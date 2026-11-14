package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/candy-tools/todo/internal/todo/todotest"
)

func writeMD(t *testing.T, md string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "in.md")
	if err := os.WriteFile(p, []byte(md), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestImportCmd(t *testing.T) {
	db := filepath.Join(t.TempDir(), "x.db")
	md := writeMD(t, "# Work\n\n- [ ] a\n- [x] b\n")
	out, err := runCmd(t, "import", md, "-f", db)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if want := "imported 1 categories, 2 tasks from " + md + "\n"; out != want {
		t.Errorf("out = %q, want %q", out, want)
	}
	if got := todotest.Dump(t, db); !contains(got, "- [x] b") {
		t.Errorf("imported content wrong:\n%s", got)
	}
}

func TestImportCmdRefusesNonEmpty(t *testing.T) {
	db := todotest.Seed(t, "# Old\n")
	_, err := runCmd(t, "import", writeMD(t, "# New\n"), "-f", db)
	if codeOf(t, err) != exitGeneric || !contains(err.Error(), "--force") {
		t.Errorf("want exit 1 mentioning --force, got %v", err)
	}
}

func TestImportCmdForce(t *testing.T) {
	db := todotest.Seed(t, "# Old\n")
	if _, err := runCmd(t, "import", writeMD(t, "# New\n"), "-f", db, "--force"); err != nil {
		t.Fatalf("import --force: %v", err)
	}
	got := todotest.Dump(t, db)
	if contains(got, "Old") || !contains(got, "# New") {
		t.Errorf("content should be replaced:\n%s", got)
	}
}
