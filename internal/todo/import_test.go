package todo_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/candy-tools/todo/internal/todo"
	"github.com/candy-tools/todo/internal/todo/todotest"
)

func TestImportExampleRoundTrips(t *testing.T) {
	b, err := os.ReadFile("../../example.md")
	if err != nil {
		t.Fatal(err)
	}
	got := todotest.Dump(t, todotest.Seed(t, string(b)))
	want := todotest.Render(todotest.Parse(t, string(b)))
	if got == "" || got != want {
		t.Fatalf("round trip differs\n got:\n%s\nwant:\n%s", got, want)
	}
}

func TestImportSkipsGuide(t *testing.T) {
	src := guideSample + "\n\n# Main\n"
	_, rep, err := todo.ImportMarkdown([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if rep.Categories != 1 || rep.Tasks != 0 || len(rep.Warnings) != 0 {
		t.Fatalf("report = %+v; want 1 category, 0 tasks, no warnings", rep)
	}
}

func TestImportWarnings(t *testing.T) {
	doc, rep, err := todo.ImportMarkdown([]byte("intro\n# A\n\n- [?] odd\n  stray under cat\n"))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"line 1: text before the first heading is not imported",
		`line 4: unrecognised line dropped: "- [?] odd"`,
		`line 5: unrecognised line dropped: "stray under cat"`,
	}
	if strings.Join(rep.Warnings, "\n") != strings.Join(want, "\n") {
		t.Fatalf("warnings = %q; want %q", rep.Warnings, want)
	}
	if len(doc.Roots) != 1 || doc.Roots[0].Title != "A" {
		t.Fatalf("category A not imported: %+v", doc.Roots)
	}
}

func TestImportRootTasksGoToUncategorized(t *testing.T) {
	doc, rep, err := todo.ImportMarkdown([]byte("- [ ] loose\n# A\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Roots) != 2 || doc.Roots[0].Title != "A" || doc.Roots[1].Title != "Uncategorized" {
		t.Fatalf("roots = %+v; want [A, Uncategorized]", doc.Roots)
	}
	u := doc.Roots[1]
	if len(u.Children) != 1 || u.Children[0].Title != "loose" {
		t.Fatalf("Uncategorized children = %+v", u.Children)
	}
	if rep.Categories != 2 || rep.Tasks != 1 {
		t.Fatalf("report = %+v", rep)
	}
}

func TestImportEmptyFile(t *testing.T) {
	_, rep, err := todo.ImportMarkdown(nil)
	if err != nil || rep.Categories != 0 || rep.Tasks != 0 {
		t.Fatalf("got %+v, %v; want empty report, nil", rep, err)
	}
}

func TestImportRefusesNonEmpty(t *testing.T) {
	path := todotest.Seed(t, "# A\n\n- [ ] one\n")
	s, err := todo.OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	doc, _, _ := todo.ImportMarkdown([]byte("# B\n"))
	if err := s.Import(context.Background(), doc, false); !errors.Is(err, todo.ErrNotEmpty) {
		t.Fatalf("Import = %v; want ErrNotEmpty", err)
	}
	if got := todotest.Render(mustLoad(t, s)); !strings.Contains(got, "# A") || strings.Contains(got, "# B") {
		t.Fatalf("store changed on refused import:\n%s", got)
	}
}

func TestImportReplace(t *testing.T) {
	path := todotest.Seed(t, "# A\n\n- [ ] one\n")
	s, err := todo.OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	doc, _, _ := todo.ImportMarkdown([]byte("# B\n\n- [x] two\n  desc\n"))
	if err := s.Import(context.Background(), doc, true); err != nil {
		t.Fatal(err)
	}
	want := "# B\n\n- [x] two\n  desc\n"
	if got := todotest.Render(mustLoad(t, s)); got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestImportFileFailureLeavesNoDB(t *testing.T) {
	dir := t.TempDir()
	md := filepath.Join(dir, "TODO.md")
	bad := []byte{0xff, 0xfe}
	if err := os.WriteFile(md, bad, 0o600); err != nil {
		t.Fatal(err)
	}
	db := filepath.Join(dir, ".todo.db")
	if s, _, err := todo.ImportFile(db, md, false); err == nil {
		_ = s.Close()
		t.Fatal("ImportFile succeeded on invalid UTF-8")
	}
	for _, p := range []string{db, db + "-wal", db + "-shm"} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("%s exists after failed import (err=%v)", p, err)
		}
	}
	if got, _ := os.ReadFile(md); !bytes.Equal(got, bad) {
		t.Fatal("markdown file was modified")
	}
}

func TestImportFileNotEmptyKeepsExistingDB(t *testing.T) {
	db := todotest.Seed(t, "# A\n")
	md := filepath.Join(t.TempDir(), "TODO.md")
	if err := os.WriteFile(md, []byte("# B\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := todo.ImportFile(db, md, false); !errors.Is(err, todo.ErrNotEmpty) {
		t.Fatalf("err = %v; want ErrNotEmpty", err)
	}
	if got := todotest.Dump(t, db); got != "# A\n" {
		t.Fatalf("existing db changed: %q", got)
	}
}

func mustLoad(t *testing.T, s *todo.Store) *todo.Document {
	t.Helper()
	doc, _, err := s.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return doc
}
