package jsonfile

import (
	"errors"
	"os"
	"syscall"
	"time"
)

// The rename retry, on Windows only: replacing a file another process has open
// (a reader without FILE_SHARE_DELETE, an antivirus or indexer scan) fails for
// a moment with a sharing violation or access denied. The retry backs off from
// renameFirstDelay, doubling up to renameMaxDelay, and gives up once the next
// wait would pass renameBudget.
const (
	renameBudget     = 2 * time.Second
	renameFirstDelay = 10 * time.Millisecond
	renameMaxDelay   = 250 * time.Millisecond
)

// Windows error codes a rename is retried on. They are plain numbers so the
// rule builds, and is tested, on every system.
const (
	errorAccessDenied     = syscall.Errno(5)
	errorSharingViolation = syscall.Errno(32)
)

// renameFile renames a file. It is a test seam, os.Rename outside tests.
var renameFile = os.Rename

// renameWithRetry renames from over to. On goos "windows" a failure that is a
// sharing violation or access denied is retried with backoff for up to
// renameBudget; any other failure, and every failure on other systems, is
// returned at once.
func renameWithRetry(goos, from, to string) error {
	deadline := time.Now().Add(renameBudget)
	delay := renameFirstDelay
	for {
		err := renameFile(from, to)
		if err == nil || !renameRetryable(goos, err) || time.Now().Add(delay).After(deadline) {
			return err
		}
		time.Sleep(delay)
		delay = min(2*delay, renameMaxDelay)
	}
}

// renameRetryable reports whether a rename that failed with err on goos may
// succeed when tried again shortly.
func renameRetryable(goos string, err error) bool {
	if goos != "windows" {
		return false
	}
	var errno syscall.Errno
	if !errors.As(err, &errno) {
		return false
	}
	return errno == errorAccessDenied || errno == errorSharingViolation
}
