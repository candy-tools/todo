package todo_test

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
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

// holdWriteLockFor takes the writer lock on the store at path from a raw
// connection, as todotest.HoldWriteLock does, and releases it after d, standing
// in for another process finishing a write. With rollbackJournal it first
// switches the file out of WAL mode, so opening the store, which switches it
// back, is refused with SQLITE_BUSY at once instead of waiting.
func holdWriteLockFor(t *testing.T, path string, d time.Duration, rollbackJournal bool) {
	t.Helper()
	ctx := context.Background()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	conn, err := db.Conn(ctx)
	if err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	queries := []string{"BEGIN IMMEDIATE", "UPDATE meta SET revision = revision"}
	if rollbackJournal {
		queries = append([]string{"PRAGMA journal_mode=DELETE"}, queries...)
	}
	for _, q := range queries {
		if _, err := conn.ExecContext(ctx, q); err != nil {
			_ = conn.Close()
			_ = db.Close()
			t.Fatalf("%s: %v", q, err)
		}
	}
	released := make(chan struct{})
	go func() {
		defer close(released)
		time.Sleep(d)
		_, _ = conn.ExecContext(ctx, "ROLLBACK")
		_ = conn.Close()
		_ = db.Close()
	}()
	t.Cleanup(func() { <-released })
}

// A zero busy timeout means the default: an open SQLite refuses as busy is
// retried while another process finishes its write, not given up at once.
func TestOpenStoreZeroBusyTimeoutRetriesBusyOpen(t *testing.T) {
	path := todotest.Seed(t, "# Work\n- [ ] a\n")
	holdWriteLockFor(t, path, 1500*time.Millisecond, true)

	s, err := todo.OpenStore(path, todo.WithBusyTimeout(0))
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	_ = s.Close()
}

// A short busy timeout limits only the writes after the open: opening and
// migrating the store still wait out another process's write up to the
// default, and a later write still gives up fast with ErrBusy.
func TestOpenStoreShortBusyTimeoutWaitsAtOpen(t *testing.T) {
	for _, tc := range []struct {
		name            string
		rollbackJournal bool
	}{
		{"wal", false},
		{"busy open", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := todotest.Seed(t, "# Work\n- [ ] a\n")
			holdWriteLockFor(t, path, 1500*time.Millisecond, tc.rollbackJournal)

			s, err := todo.OpenStore(path, todo.WithBusyTimeout(750*time.Millisecond))
			if err != nil {
				t.Fatalf("OpenStore: %v", err)
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
		})
	}
}

// An import into an existing store opened with a short busy timeout (as the
// TUI's first run does) is one large, deliberate write: it waits out another
// process's write up to the default, not the store's timeout. Later writes
// still give up after the short timeout.
func TestImportFileWaitsDefaultBusyTimeout(t *testing.T) {
	path := todotest.Seed(t, "# Work\n- [ ] a\n")
	md := filepath.Join(t.TempDir(), "TODO.md")
	if err := os.WriteFile(md, []byte("# New\n- [ ] b\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	todo.SetImportAfterBackup(t, func() {
		holdWriteLockFor(t, path, 1500*time.Millisecond, false)
	})

	s, _, err := todo.ImportFile(path, md, true, todo.WithBusyTimeout(750*time.Millisecond))
	if err != nil {
		t.Fatalf("ImportFile: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	doc, _, err := s.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Roots) != 1 || doc.Roots[0].Title != "New" {
		t.Fatalf("roots after import = %+v, want only New", doc.Roots)
	}

	b := doc.Roots[0].Children[0]
	todotest.HoldWriteLock(t, path)
	start := time.Now()
	err = s.SetStatuses(context.Background(), map[uint]todo.Status{b.ID: todo.Done})
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("a write after the import waited %v, want the short timeout", elapsed)
	}
	if !errors.Is(err, todo.ErrBusy) {
		t.Errorf("err = %v, want ErrBusy", err)
	}
}
