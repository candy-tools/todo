package jsonfile

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

// renameAttempts and renameDelay bound the rename retry: on Windows, replacing
// a file another process has open for a read can fail for a moment.
const (
	renameAttempts = 5
	renameDelay    = 20 * time.Millisecond
)

// atomicWrite replaces path with data so that a reader sees either the old or
// the new content, never a mix: it writes and fsyncs a temp file in the same
// directory, gives it the existing file's mode (0o644 for a new file), renames
// it over path and, on Unix, fsyncs the directory. The temp file is removed on
// any failure.
func atomicWrite(path string, data []byte) (err error) {
	dir, base := filepath.Split(path)
	if dir == "" {
		dir = "."
	}
	tmp, err := os.CreateTemp(dir, "."+base+".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() {
		if err != nil {
			_ = tmp.Close()
			_ = os.Remove(tmpName)
		}
	}()

	if _, err = tmp.Write(data); err != nil {
		return err
	}
	if err = tmp.Sync(); err != nil {
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}

	mode := fs.FileMode(0o644)
	if info, statErr := os.Stat(path); statErr == nil {
		mode = info.Mode().Perm()
	} else if !errors.Is(statErr, fs.ErrNotExist) {
		return statErr
	}
	if err = os.Chmod(tmpName, mode); err != nil {
		return err
	}

	for attempt := 1; ; attempt++ {
		if err = os.Rename(tmpName, path); err == nil || attempt == renameAttempts {
			break
		}
		time.Sleep(renameDelay)
	}
	if err != nil {
		return err
	}

	if runtime.GOOS != "windows" {
		if d, openErr := os.Open(dir); openErr == nil { //nolint:gosec // dir is the store's own directory, opened only to fsync the rename
			_ = d.Sync()
			_ = d.Close()
		}
	}
	return nil
}
