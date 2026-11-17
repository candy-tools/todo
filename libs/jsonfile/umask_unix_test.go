//go:build unix

package jsonfile_test

import (
	"io/fs"
	"os"
	"syscall"
	"testing"

	"github.com/candy-tools/todo/libs/jsonfile"
)

// withUmask sets the process umask for the test and restores it after. The
// umask is process-wide: tests using it must not run in parallel.
func withUmask(t *testing.T, mask int) {
	t.Helper()
	old := syscall.Umask(mask)
	t.Cleanup(func() { syscall.Umask(old) })
}

func perm(t *testing.T, path string) fs.FileMode {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	return info.Mode().Perm()
}

func TestNewFileHonoursUmask(t *testing.T) {
	withUmask(t, 0o077)
	path := tempPath(t)
	mustUpdate(t, jsonfile.New(jsonfile.Config[doc]{Path: path}), setN(1))
	if got := perm(t, path); got != 0o600 {
		t.Errorf("new file mode = %v, want 0600 under umask 077", got)
	}
	if got := perm(t, path+".lock"); got != 0o600 {
		t.Errorf("lock file mode = %v, want 0600 under umask 077", got)
	}
}

func TestNewFileDefaultUmask(t *testing.T) {
	withUmask(t, 0o022)
	path := tempPath(t)
	mustUpdate(t, jsonfile.New(jsonfile.Config[doc]{Path: path}), setN(1))
	if got := perm(t, path); got != 0o644 {
		t.Errorf("new file mode = %v, want 0644 under umask 022", got)
	}
	if got := perm(t, path+".lock"); got != 0o644 {
		t.Errorf("lock file mode = %v, want 0644 under umask 022", got)
	}
}

func TestExistingFileKeepsMode(t *testing.T) {
	withUmask(t, 0o077)
	path := tempPath(t)
	s := jsonfile.New(jsonfile.Config[doc]{Path: path})
	mustUpdate(t, s, setN(1))
	if err := os.Chmod(path, 0o640); err != nil {
		t.Fatal(err)
	}
	mustUpdate(t, s, setN(2))
	if got := perm(t, path); got != 0o640 {
		t.Errorf("rewritten file mode = %v, want the existing 0640", got)
	}
}
