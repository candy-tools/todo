package jsonfile

import (
	"crypto/rand"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
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
// replace the link): callers resolve it first (see resolveLinks). It must be
// called under the lock: it first removes temp files a crashed writer left
// (see sweepTemps).
//
// The directory fsync is best-effort: its errors are ignored, and it is
// skipped on Windows, which cannot fsync a directory. The write has succeeded
// once the rename has; the fsync only makes the rename more likely to survive
// a crash or power loss right after it.
func atomicWrite(path string, data []byte) (err error) {
	dir, base := filepath.Split(path)
	if dir == "" {
		dir = "."
	}
	info, statErr := os.Stat(path)
	if statErr != nil && !errors.Is(statErr, fs.ErrNotExist) {
		return statErr
	}

	prefix := "." + base + ".tmp-"
	sweepTemps(dir, prefix)
	tmp, err := createTemp(dir, prefix)
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

	if err = renameWithRetry(runtime.GOOS, tmpName, path); err != nil {
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

// sweepTemps removes the files in dir named prefix plus a createTemp suffix:
// temp files a writer left behind when it crashed between creating and
// renaming one. It must only run under the lock: only the lock holder creates
// temp files, so any found then are leftovers. The suffix must have
// createTemp's exact form, so the temps of another file whose name starts
// with this one's (list "a.tmp-b" next to list "a") are left alone. It is
// best-effort, and errors are ignored (a leftover only wastes space).
func sweepTemps(dir, prefix string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		suffix, ok := strings.CutPrefix(e.Name(), prefix)
		if ok && isTempSuffix(suffix) && e.Type().IsRegular() {
			_ = os.Remove(filepath.Join(dir, e.Name()))
		}
	}
}

// tempSuffixLen is the length of rand.Text's output, createTemp's suffix.
const tempSuffixLen = 26

// isTempSuffix reports whether s has the form of rand.Text's output, the
// suffix createTemp gives a temp file: tempSuffixLen characters of the
// base32 alphabet [A-Z2-7].
func isTempSuffix(s string) bool {
	if len(s) != tempSuffixLen {
		return false
	}
	for _, c := range []byte(s) {
		if (c < 'A' || c > 'Z') && (c < '2' || c > '7') {
			return false
		}
	}
	return true
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
	return nil, fmt.Errorf("no unused temp file name in %s", dir)
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
	return "", fmt.Errorf("%s: too many levels of symbolic links", path)
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
