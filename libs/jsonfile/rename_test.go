package jsonfile

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"
)

// Windows error codes rename retries on.
const (
	winAccessDenied     = syscall.Errno(5)
	winSharingViolation = syscall.Errno(32)
)

// fakeRename replaces renameFile for the test with one that fails with the
// given errors in turn and then succeeds, and returns the call count.
func fakeRename(t *testing.T, errs ...error) *int {
	t.Helper()
	calls := new(int)
	renameFile = func(_, _ string) error {
		*calls++
		if *calls <= len(errs) {
			return &os.LinkError{Op: "rename", Old: "a", New: "b", Err: errs[*calls-1]}
		}
		return nil
	}
	t.Cleanup(func() { renameFile = os.Rename })
	return calls
}

func TestRenameFailsAtOnceOffWindows(t *testing.T) {
	calls := fakeRename(t, winSharingViolation, winSharingViolation)
	if err := renameWithRetry("linux", "a", "b"); err == nil {
		t.Fatal("rename returned nil, want the first failure")
	}
	if *calls != 1 {
		t.Errorf("rename tried %d times, want 1", *calls)
	}
}

func TestRenameRetriesSharingErrorsOnWindows(t *testing.T) {
	calls := fakeRename(t, winSharingViolation, winAccessDenied)
	if err := renameWithRetry("windows", "a", "b"); err != nil {
		t.Fatalf("rename: %v", err)
	}
	if *calls != 3 {
		t.Errorf("rename tried %d times, want 3", *calls)
	}
}

func TestRenameDoesNotRetryOtherErrorsOnWindows(t *testing.T) {
	notFound := syscall.Errno(2) // ERROR_FILE_NOT_FOUND
	calls := fakeRename(t, notFound, notFound)
	err := renameWithRetry("windows", "a", "b")
	if !errors.Is(err, notFound) {
		t.Fatalf("err = %v, want the first failure", err)
	}
	if *calls != 1 {
		t.Errorf("rename tried %d times, want 1", *calls)
	}
}

// TestAtomicWriteRenamesOnceOffWindows checks atomicWrite goes through the
// same rule: off Windows, a failing rename is not retried.
func TestAtomicWriteRenamesOnceOffWindows(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows retries")
	}
	calls := fakeRename(t, winSharingViolation, winSharingViolation)
	if err := atomicWrite(filepath.Join(t.TempDir(), "list.json"), []byte("{}")); err == nil {
		t.Fatal("atomicWrite returned nil, want the rename failure")
	}
	if *calls != 1 {
		t.Errorf("rename tried %d times, want 1", *calls)
	}
}
