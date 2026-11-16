package cmd

import (
	"context"
	"os"
	"testing"

	"github.com/candy-tools/todo/internal/todo"
)

// idOf returns the id of the item titled title in the store at dbPath.
func idOf(t *testing.T, dbPath, title string) string {
	t.Helper()
	s := todo.NewStore(dbPath)
	doc, _, err := s.Load(context.Background())
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	m := doc.ByTitle(title)
	if len(m) != 1 {
		t.Fatalf("want exactly one item titled %q, got %d", title, len(m))
	}
	return m[0].ID
}

// bytesOf returns the content of the list file at path.
func bytesOf(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return b
}
