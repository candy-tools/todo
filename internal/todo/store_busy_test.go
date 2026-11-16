package todo_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/candy-tools/todo/internal/todo"
	"github.com/candy-tools/todo/internal/todo/todotest"
)

// A write while another process holds the lock gives up after the store's
// busy timeout, not the 5 s default, and reports ErrBusy; a read does not wait.
func TestStoreBusy(t *testing.T) {
	path := todotest.Seed(t, "# Work\n\n- [ ] a\n")
	s := todo.NewStore(path, todo.WithBusyTimeout(50*time.Millisecond))
	ctx := context.Background()
	todotest.HoldWriteLock(t, path)

	start := time.Now()
	_, err := s.AppendCategory(ctx, "B", "")
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("the write waited %v, want well under the 5 s default", elapsed)
	}
	if !errors.Is(err, todo.ErrBusy) {
		t.Fatalf("AppendCategory = %v, want ErrBusy", err)
	}
	if err.Error() != "list busy (another todo is writing), try again" {
		t.Errorf("ErrBusy message = %q", err)
	}
	doc, _, err := s.Load(ctx)
	if err != nil || len(doc.Roots) != 1 {
		t.Fatalf("Load while locked = %v, %v; want the list", doc, err)
	}
	if got := todotest.Dump(t, path); got != "# Work\n\n- [ ] a\n" {
		t.Fatalf("list after the busy write:\n%s", got)
	}
}
