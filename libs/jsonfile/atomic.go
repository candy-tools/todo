package jsonfile

import (
	"crypto/rand"
	"errors"
	"fmt"
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

// tempAttempts bounds the search for an unused temp file name.
const tempAttempts = 10000

// maxLinks bounds how many symlinks resolveLinks follows, so a link loop fails
// instead of spinning.
const maxLinks = 255

// afterTempWrite, when not nil, is called with the temp file's name once the
// data is in it. It is a test seam, nil outside tests.
var afterTempWrite func(name string)

// atomicWrite replaces path with data so that a reader sees either the old or
// the new content, never a mix: it writes and fsyncs a temp file in the same
// directory, renames it over path and, on Unix, fsyncs the directory. A new
// file gets mode 0o666 less the umask; a replaced one keeps its mode. The temp
// file is removed on any failure. path must not be a symlink (the rename would
// replace the link): callers resolve it first (see resolveLinks).
func atomicWrite(path string, data []byte) (err error) {
	dir, base := filepath.Split(path)
	if dir == "" {
		dir = "."
	}
	info, statErr := os.Stat(path)
	if statErr != nil && !errors.Is(statErr, fs.ErrNotExist) {
		return statErr
	}

	tmp, err := createTemp(dir, "."+base+".tmp-")
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
	// Give the temp file the existing file's mode before any data is in it, so
	// the new content is never readable more widely than the old was.
	if statErr == nil {
		if err = os.Chmod(tmpName, info.Mode().Perm()); err != nil {
			return err
		}
	}

	if _, err = tmp.Write(data); err != nil {
		return err
	}
	if afterTempWrite != nil {
		afterTempWrite(tmpName)
	}
	if err = tmp.Sync(); err != nil {
		return err
	}
	if err = tmp.Close(); err != nil {
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

// createTemp creates a new file named prefix plus a random suffix in dir, with
// mode 0o666 less the umask (os.CreateTemp would force 0o600), retrying when
// the name is taken.
func createTemp(dir, prefix string) (*os.File, error) {
	for range tempAttempts {
		name := filepath.Join(dir, prefix+rand.Text())
		f, err := os.OpenFile(name, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o666) //nolint:gosec // a fresh temp file next to the store's own file
		if errors.Is(err, fs.ErrExist) {
			continue
		}
		return f, err
	}
	return nil, fmt.Errorf("jsonfile: no unused temp file name in %s", dir)
}

// resolveLinks returns the file path names once every symlink in its last
// element is followed, or path itself when that is not a symlink (or does not
// exist). A dangling link resolves to the path it points at, so a write
// creates that file rather than replace the link.
//
// A link target is resolved as the system does: a relative one against the
// directory the link really sits in, and `..` against the directories on disk,
// not by cleaning the path as text (reached through a symlinked directory,
// `..` leads out of the directory linked to, not back up the path). Once a
// link was followed, the directory of the result is the real one (symlinked
// directories resolved); a path that is not a link is returned as given.
func resolveLinks(path string) (string, error) {
	p := path
	for range maxLinks {
		info, err := os.Lstat(p)
		if errors.Is(err, fs.ErrNotExist) {
			return p, nil
		}
		if err != nil {
			return "", err
		}
		if info.Mode()&fs.ModeSymlink == 0 {
			return p, nil
		}
		target, err := os.Readlink(p)
		if err != nil {
			return "", err
		}
		if !filepath.IsAbs(target) {
			// Concatenate rather than filepath.Join, which would clean `..`
			// as text; realDir resolves it on disk.
			linkDir, _ := filepath.Split(p)
			target = linkDir + target
		}
		if p, err = realDir(target); err != nil {
			return "", err
		}
	}
	return "", fmt.Errorf("jsonfile: %s: too many levels of symbolic links", path)
}

// realDir returns p with its directory resolved on disk: symlinked
// directories followed and `..` taken against the real directories. The last
// element is kept as is (it may itself be a link). When the directory does
// not exist, p is returned unchanged; a write there fails on its own.
func realDir(p string) (string, error) {
	dir, base := filepath.Split(p)
	if dir == "" {
		dir = "."
	}
	resolved, err := filepath.EvalSymlinks(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return p, nil
	}
	if err != nil {
		return "", err
	}
	return filepath.Join(resolved, base), nil
}
