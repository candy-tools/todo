package todotest_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/candy-tools/todo/internal/todo"
	"github.com/candy-tools/todo/internal/todo/todotest"
)

func TestParseRenderRoundTrip(t *testing.T) {
	md := "# A\n\n- [x] done\n  - [ ] sub\n\n## B\n\n- [/] one\n  note\n\n  more\n"
	if got := todotest.Render(todotest.Parse(t, md)); got != md {
		t.Fatalf("got:\n%s\nwant:\n%s", got, md)
	}
}

func TestParseMovesLooseTasksToUncategorized(t *testing.T) {
	d := todotest.Parse(t, "- [ ] loose\n")
	if len(d.Roots) != 1 || d.Roots[0].Title != "Uncategorized" {
		t.Fatalf("roots = %v, want a single Uncategorized category", d.Roots)
	}
}

func TestRenderEmptyDocument(t *testing.T) {
	if got := todotest.Render(&todo.Document{}); got != "" {
		t.Errorf("Render(empty) = %q, want empty", got)
	}
}

func TestSeedDumpRoundTrip(t *testing.T) {
	md := "# A\n\n- [/] one\n  note\n  - [ ] sub\n\n## B\n\n- [>] two\n"
	if got := todotest.Dump(t, todotest.Seed(t, md)); got != md {
		t.Fatalf("got:\n%s\nwant:\n%s", got, md)
	}
}

// HoldWriteLock keeps every other writer out until the test that took it ends.
func TestHoldWriteLock(t *testing.T) {
	path := todotest.Seed(t, "# A\n")
	s := todo.NewStore(path, todo.WithBusyTimeout(50*time.Millisecond))
	ctx := context.Background()
	t.Run("held", func(t *testing.T) {
		todotest.HoldWriteLock(t, path)
		if _, _, err := s.AddCategory(ctx, "B", "", ""); !errors.Is(err, todo.ErrBusy) {
			t.Errorf("write while held: err = %v, want ErrBusy", err)
		}
	})
	if _, _, err := s.AddCategory(ctx, "B", "", ""); err != nil {
		t.Errorf("write after the holding test ended: %v", err)
	}
}

// HoldWriteLock on a symlinked list takes the lock writers through the link
// take: the one beside the file the link points at.
func TestHoldWriteLockThroughSymlink(t *testing.T) {
	target := todotest.Seed(t, "# A\n")
	link := filepath.Join(t.TempDir(), "link.json")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	todotest.HoldWriteLock(t, link)
	s := todo.NewStore(link, todo.WithBusyTimeout(50*time.Millisecond))
	if _, _, err := s.AddCategory(context.Background(), "B", "", ""); !errors.Is(err, todo.ErrBusy) {
		t.Errorf("write through the link while held: err = %v, want ErrBusy", err)
	}
}

// SeedJSON writes the fixture as the list file as is, so a test can give
// kanban states, IDs and plans directly.
func TestSeedJSON(t *testing.T) {
	const fixture = `{"version": 3, "categories": [{"id": "c000000", "title": "C"}], "tasks": [
		{"id": "t000001", "category": "c000000", "title": "r", "status": "ready", "plan": "docs/r.md"}]}`
	path := todotest.SeedJSON(t, fixture)
	if b, err := os.ReadFile(path); err != nil || string(b) != fixture {
		t.Fatalf("list file = %q, %v; want the fixture as is", b, err)
	}
	doc, _, err := todo.NewStore(path).Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	r, ok := doc.ByID("t000001")
	if !ok || r.Status != todo.Ready || r.Plan != "docs/r.md" {
		t.Fatalf("t000001 = %+v, want the ready task r with its plan", r)
	}
}
