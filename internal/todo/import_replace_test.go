package todo

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// writeMarkdown writes md to a TODO.md in a temporary directory and returns
// its path.
func writeMarkdown(t *testing.T, md string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "TODO.md")
	if err := os.WriteFile(p, []byte(md), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// importMarkdown runs ImportFile of md into path.
func importMarkdown(t *testing.T, path, md string, replace bool) (ImportReport, error) {
	t.Helper()
	_, rep, err := ImportFile(path, writeMarkdown(t, md), replace)
	return rep, err
}

func TestImportFileReplaceBacksUp(t *testing.T) {
	s := newStore(t)
	mustID(t)(s.AppendCategory(context.Background(), "Old", ""))
	old := fileBytes(t, s.Path())

	rep, err := importMarkdown(t, s.Path(), "# New\n", true)
	if err != nil {
		t.Fatal(err)
	}
	if want := s.Path() + ".bak"; rep.Backup != want || BackupPath(s.Path(), 0) != want {
		t.Fatalf("Backup = %q, want %q", rep.Backup, want)
	}
	if got := fileBytes(t, rep.Backup); !bytes.Equal(got, old) {
		t.Fatalf("backup holds:\n%s\nwant the old bytes:\n%s", got, old)
	}
	if got := titles(load(t, s).Roots); len(got) != 1 || got[0] != "New" {
		t.Fatalf("roots after the replace = %v, want [New]", got)
	}

	second := fileBytes(t, s.Path())
	rep, err = importMarkdown(t, s.Path(), "# Newer\n", true)
	if err != nil {
		t.Fatal(err)
	}
	if want := s.Path() + ".bak.1"; rep.Backup != want || BackupPath(s.Path(), 1) != want {
		t.Fatalf("second Backup = %q, want %q", rep.Backup, want)
	}
	if got := fileBytes(t, rep.Backup); !bytes.Equal(got, second) {
		t.Fatalf("second backup holds:\n%s\nwant:\n%s", got, second)
	}
	if got := fileBytes(t, s.Path()+".bak"); !bytes.Equal(got, old) {
		t.Fatal("the second replace changed the first backup")
	}
	if left, _ := filepath.Glob(filepath.Join(filepath.Dir(s.Path()), ".todo-backup-*")); len(left) > 0 {
		t.Fatalf("temporary backup files left behind: %v", left)
	}
}

func TestImportFileNotEmpty(t *testing.T) {
	s := newStore(t)
	mustID(t)(s.AppendCategory(context.Background(), "Old", ""))
	old := fileBytes(t, s.Path())

	if _, err := importMarkdown(t, s.Path(), "# New\n", false); !errors.Is(err, ErrNotEmpty) {
		t.Fatalf("ImportFile = %v, want ErrNotEmpty", err)
	}
	if got := fileBytes(t, s.Path()); !bytes.Equal(got, old) {
		t.Fatalf("file after a refused import:\n%s\nwant:\n%s", got, old)
	}
	if left, _ := filepath.Glob(s.Path() + ".bak*"); len(left) > 0 {
		t.Fatalf("backup taken for a refused import: %v", left)
	}
}

// A replace whose write fails after the backup was taken leaves the list as
// it was, so the backup it took is removed again, while an older backup from a
// previous run stays.
func TestImportReplaceRemovesBackupWhenWriteFails(t *testing.T) {
	s := newStore(t)
	mustID(t)(s.AppendCategory(context.Background(), "Old", ""))
	old := fileBytes(t, s.Path())
	older := BackupPath(s.Path(), 0)
	if err := os.WriteFile(older, []byte("older backup"), 0o600); err != nil {
		t.Fatal(err)
	}

	// An ID the format refuses fails the write's validation, after fn ran.
	doc := &Document{}
	doc.AppendRoot(&Item{ID: "not-an-id", Kind: Category, Title: "New"})
	_, backup, err := s.importDoc(context.Background(), doc, true, true)
	if !errors.Is(err, ErrInvalidList) {
		t.Fatalf("importDoc = %v, want ErrInvalidList", err)
	}
	if backup != "" {
		t.Errorf("backup = %q after a failed import, want none", backup)
	}
	if _, err := os.Stat(BackupPath(s.Path(), 1)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("backup .bak.1 after a failed import: stat err = %v, want it removed", err)
	}
	if got, err := os.ReadFile(older); err != nil || string(got) != "older backup" {
		t.Errorf("older backup = %q, %v; want it kept", got, err)
	}
	if got := fileBytes(t, s.Path()); !bytes.Equal(got, old) {
		t.Fatalf("file after a failed import:\n%s\nwant:\n%s", got, old)
	}
}

// An import cleans its titles, gives its items IDs and replaces the list.
func TestImportCleansTitlesAndAssignsIDs(t *testing.T) {
	s := newStore(t)
	doc := &Document{}
	cat := &Item{Kind: Category, Level: 1, Title: "  A  "}
	doc.AppendRoot(cat)
	cat.AppendChild(NewTask(" t ", "", false))
	if _, err := s.Import(context.Background(), doc, false); err != nil {
		t.Fatal(err)
	}
	got := load(t, s)
	if len(got.Roots) != 1 || got.Roots[0].Title != "A" || len(got.Roots[0].Children) != 1 || got.Roots[0].Children[0].Title != "t" {
		t.Fatalf("imported %v, want A holding t", titles(got.Roots))
	}
	for _, it := range got.Enumerate() {
		if !validID(it.ID) {
			t.Errorf("%s has ID %q, want a valid one", it.Title, it.ID)
		}
	}

	bad := &Document{}
	bad.AppendRoot(&Item{Kind: Category, Level: 1, Title: " "})
	rejects(t, s, ErrInvalidTitle, func() error { return errOf(s.Import(context.Background(), bad, true)) })
}
