//go:build unix

package jsonfile

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

// TestTempFileNeverWiderThanTarget replaces a 0600 file under umask 022: the
// temp file holding the new data must never be readable by others, not even
// before the rename.
func TestTempFileNeverWiderThanTarget(t *testing.T) {
	old := syscall.Umask(0o022)
	t.Cleanup(func() { syscall.Umask(old) })
	path := filepath.Join(t.TempDir(), "list.json")
	if err := os.WriteFile(path, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}

	called := false
	afterTempWrite = func(name string) {
		called = true
		info, err := os.Stat(name)
		if err != nil {
			t.Errorf("stat temp file: %v", err)
			return
		}
		if got := info.Mode().Perm(); got&^0o600 != 0 {
			t.Errorf("temp file mode = %v while holding the data, want at most 0600", got)
		}
	}
	t.Cleanup(func() { afterTempWrite = nil })

	if err := atomicWrite(path, []byte("new secret")); err != nil {
		t.Fatalf("atomicWrite: %v", err)
	}
	if !called {
		t.Fatal("afterTempWrite was not called")
	}
}
