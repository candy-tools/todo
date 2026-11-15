package todo_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/candy-tools/todo/internal/todo"
	"github.com/candy-tools/todo/internal/todo/todotest"
)

// A write while another connection holds the SQLite writer lock gives up after
// the store's busy timeout, not the 5 s default, and reports ErrBusy.
func TestWriteWhileLockedIsBusy(t *testing.T) {
	path := todotest.Seed(t, "# Work\n- [ ] a\n")
	s, err := todo.OpenStore(path, todo.WithBusyTimeout(100*time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	doc, _, err := s.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	a := doc.Roots[0].Children[0]
	todotest.HoldWriteLock(t, path)

	start := time.Now()
	err = s.SetStatuses(context.Background(), map[uint]todo.Status{a.ID: todo.Done})
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("the write waited %v, want well under the 5 s default", elapsed)
	}
	if !errors.Is(err, todo.ErrBusy) {
		t.Errorf("err = %v, want ErrBusy", err)
	}
}
