package todo

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// TestImportFileReplaceConflictsWithWriteAfterBackup lands a write from another
// handle between the backup and the replacing import, for a store with items
// (backed up) and an empty one (no backup). The import must fail with
// ErrConflict and write nothing, so the new item is not silently lost.
func TestImportFileReplaceConflictsWithWriteAfterBackup(t *testing.T) {
	for _, tc := range []struct {
		name string
		old  []string
	}{
		{"non-empty", []string{"Old"}},
		{"empty", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			dir := t.TempDir()
			md := filepath.Join(dir, "TODO.md")
			if err := os.WriteFile(md, []byte("# New\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			other, err := OpenStore(filepath.Join(dir, "t.db"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = other.Close() })
			for _, title := range tc.old {
				if _, err := other.AppendCategory(ctx, title, 0); err != nil {
					t.Fatal(err)
				}
			}

			importAfterBackup = func() {
				importAfterBackup = nil
				if _, err := other.AppendCategory(ctx, "External", 0); err != nil {
					t.Errorf("external write: %v", err)
				}
			}
			t.Cleanup(func() { importAfterBackup = nil })

			s, _, err := importFile(ctx, other.Path(), md, true)
			if err == nil {
				_ = s.Close()
			}
			if !errors.Is(err, ErrConflict) {
				t.Fatalf("importFile = %v, want ErrConflict", err)
			}
			doc, _, err := other.Load(ctx)
			if err != nil {
				t.Fatal(err)
			}
			var got []string
			for _, r := range doc.Roots {
				got = append(got, r.Title)
			}
			want := append(append([]string{}, tc.old...), "External")
			if len(got) != len(want) {
				t.Fatalf("store roots = %q, want %q", got, want)
			}
			for i := range want {
				if got[i] != want[i] {
					t.Fatalf("store roots = %q, want %q", got, want)
				}
			}
		})
	}
}

// TestImportFileReplaceRemovesBackupWhenNothingWritten makes the replacing
// import fail after the backup, by a conflicting write and by a cancelled
// context. The store is unchanged, so the backup this run took must be gone,
// while an older backup from a previous run stays.
func TestImportFileReplaceRemovesBackupWhenNothingWritten(t *testing.T) {
	for _, tc := range []struct {
		name  string
		after func(t *testing.T, other *Store, cancel context.CancelFunc)
		want  error
	}{
		{"conflict", func(t *testing.T, other *Store, _ context.CancelFunc) {
			if _, err := other.AppendCategory(context.Background(), "External", 0); err != nil {
				t.Errorf("external write: %v", err)
			}
		}, ErrConflict},
		{"cancelled", func(_ *testing.T, _ *Store, cancel context.CancelFunc) { cancel() }, context.Canceled},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			t.Cleanup(cancel)
			dir := t.TempDir()
			md := filepath.Join(dir, "TODO.md")
			if err := os.WriteFile(md, []byte("# New\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			other, err := OpenStore(filepath.Join(dir, "t.db"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = other.Close() })
			if _, err := other.AppendCategory(ctx, "Old", 0); err != nil {
				t.Fatal(err)
			}
			older := BackupPath(other.Path(), 0)
			if err := os.WriteFile(older, []byte("older backup"), 0o600); err != nil {
				t.Fatal(err)
			}

			var backup string
			importAfterBackup = func() {
				importAfterBackup = nil
				backup = BackupPath(other.Path(), 1)
				if _, err := os.Stat(backup); err != nil {
					t.Errorf("backup %s not taken: %v", backup, err)
				}
				tc.after(t, other, cancel)
			}
			t.Cleanup(func() { importAfterBackup = nil })

			s, _, err := importFile(ctx, other.Path(), md, true)
			if err == nil {
				_ = s.Close()
			}
			if !errors.Is(err, tc.want) {
				t.Fatalf("importFile = %v, want %v", err, tc.want)
			}
			if _, err := os.Stat(backup); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("backup %s after a failed import: stat err = %v, want it removed", backup, err)
			}
			if got, err := os.ReadFile(older); err != nil || string(got) != "older backup" {
				t.Errorf("older backup = %q, %v; want it kept", got, err)
			}
		})
	}
}
