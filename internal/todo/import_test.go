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
	// The store keeps subtasks one level deep and Open or Done, so the import
	// flattens what it parsed.
	parsed := todotest.Parse(t, string(b))
	parsed.FlattenSubtasks()
	want := todotest.Render(parsed)
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

func TestImportFileFailureLeavesNoFile(t *testing.T) {
	dir := t.TempDir()
	md := filepath.Join(dir, "TODO.md")
	bad := []byte{0xff, 0xfe}
	if err := os.WriteFile(md, bad, 0o600); err != nil {
		t.Fatal(err)
	}
	list := filepath.Join(dir, ".todo.json")
	if s, _, err := todo.ImportFile(list, md, false); err == nil {
		_ = s.Close()
		t.Fatal("ImportFile succeeded on invalid UTF-8")
	}
	if left, _ := filepath.Glob(filepath.Join(dir, ".todo*")); len(left) > 0 {
		t.Errorf("files left after a failed import: %v", left)
	}
	if got, _ := os.ReadFile(md); !bytes.Equal(got, bad) {
		t.Fatal("markdown file was modified")
	}
}

func TestImportFileNotEmptyKeepsExistingList(t *testing.T) {
	list := todotest.Seed(t, "# A\n")
	md := filepath.Join(t.TempDir(), "TODO.md")
	if err := os.WriteFile(md, []byte("# B\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := todo.ImportFile(list, md, false); !errors.Is(err, todo.ErrNotEmpty) {
		t.Fatalf("err = %v; want ErrNotEmpty", err)
	}
	if got := todotest.Dump(t, list); got != "# A\n" {
		t.Fatalf("existing list changed: %q", got)
	}
}

// ImportFile creates the list file when it is missing, and returns a store on
// it whose LastWrite is the import.
func TestImportFileCreatesList(t *testing.T) {
	dir := t.TempDir()
	md := filepath.Join(dir, "TODO.md")
	if err := os.WriteFile(md, []byte("# A\n\n- [ ] one\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	list := filepath.Join(dir, ".todo.json")
	s, rep, err := todo.ImportFile(list, md, false)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	if s.Path() != list || rep.Categories != 1 || rep.Tasks != 1 || rep.Backup != "" {
		t.Fatalf("ImportFile = %s, %+v; want a store on %s and 1 category, 1 task, no backup", s.Path(), rep, list)
	}
	if got := todotest.Render(s.LastWrite().Doc); got != "# A\n\n- [ ] one\n" {
		t.Fatalf("LastWrite().Doc renders:\n%s", got)
	}
	if got := todotest.Dump(t, list); got != "# A\n\n- [ ] one\n" {
		t.Fatalf("list holds:\n%s", got)
	}
}

func TestImportFlattensDeepMarkdown(t *testing.T) {
	doc, _, err := todo.ImportMarkdown([]byte("- [ ] T\n  - [/] a\n    - [ ] a1\n"))
	if err != nil {
		t.Fatal(err)
	}
	s, err := todo.OpenStore(filepath.Join(t.TempDir(), ".todo.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	if err := s.Import(context.Background(), doc, false); err != nil {
		t.Fatal(err)
	}
	got := mustLoad(t, s)
	T := find(got, "T")
	if T == nil {
		t.Fatal("T not imported")
	}
	if names := titles(T.Children); strings.Join(names, ",") != "a,a1" {
		t.Errorf("T's children = %v, want [a a1]", names)
	}
	for _, c := range T.Children {
		if len(c.Children) != 0 {
			t.Errorf("%s has %d children, want none", c.Title, len(c.Children))
		}
	}
	if st := find(got, "a").Status; st != todo.Open {
		t.Errorf("a status = %v, want %v", st, todo.Open)
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

func TestImportFileReplaceBacksUpNonEmptyStore(t *testing.T) {
	db := todotest.Seed(t, "# Old\n\n- [ ] keep me\n")
	md := filepath.Join(t.TempDir(), "TODO.md")
	if err := os.WriteFile(md, []byte("# New\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	s, rep, err := todo.ImportFile(db, md, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddCategory(context.Background(), "After", "", ""); err != nil {
		t.Fatalf("write after the import: %v", err)
	}
	_ = s.Close()
	if want := todo.BackupPath(db, 0); rep.Backup != want {
		t.Fatalf("Backup = %q, want %q", rep.Backup, want)
	}
	if _, err := os.Stat(rep.Backup); err != nil {
		t.Fatalf("backup after a successful import: %v", err)
	}
	if got := todotest.Dump(t, rep.Backup); got != "# Old\n\n- [ ] keep me\n" {
		t.Fatalf("backup holds:\n%s", got)
	}
	if got := todotest.Dump(t, db); strings.Contains(got, "Old") || !strings.Contains(got, "# New") {
		t.Fatalf("store not replaced:\n%s", got)
	}
}

func TestImportFileReplaceEmptyStoreTakesNoBackup(t *testing.T) {
	db := todotest.Seed(t, "")
	md := filepath.Join(t.TempDir(), "TODO.md")
	if err := os.WriteFile(md, []byte("# New\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	s, rep, err := todo.ImportFile(db, md, true)
	if err != nil {
		t.Fatal(err)
	}
	_ = s.Close()
	if rep.Backup != "" {
		t.Fatalf("Backup = %q, want none", rep.Backup)
	}
	if left, _ := filepath.Glob(db + ".bak*"); len(left) > 0 {
		t.Fatalf("backup files for an empty store: %v", left)
	}
}
