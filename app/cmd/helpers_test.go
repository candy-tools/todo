package cmd

import (
	"context"
	"testing"

	"github.com/candy-tools/todo/internal/todo"
)

// idOf returns the id of the item titled title in the store at dbPath.
func idOf(t *testing.T, dbPath, title string) uint {
	t.Helper()
	s, err := todo.OpenStore(dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = s.Close() }()
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
