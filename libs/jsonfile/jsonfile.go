// Package jsonfile keeps one Go value in one JSON file that several processes
// can share safely. Writes go through Update, a read-modify-write that holds an
// exclusive lock on a separate `<path>.lock` file and replaces the data file
// atomically (temp file, fsync, rename), so a reader never sees a half-written
// file. The directory is fsynced after the rename on a best-effort basis only
// (errors ignored, skipped on Windows), so a crash right after a write may
// still lose the rename on some file systems. Reads (Load) and change polling (Changed) take no lock. The package is
// generic over the stored type and knows nothing about what it stores: the
// path, the lock timeout and the validation hook all come from Config.
//
// The lock file is left in place after a write and must not be deleted while
// the data file is in use: a writer that locked a deleted lock file does not
// exclude one that creates a new lock file. Update notices when the lock file
// it locked was replaced and locks again, but a writer already holding the
// lock when the file is deleted cannot be protected.
package jsonfile

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"sync"
	"time"

	"github.com/gofrs/flock"
)

// DefaultLockTimeout is how long Update waits for the lock when
// Config.LockTimeout is zero or negative.
const DefaultLockTimeout = 5 * time.Second

// lockRetry is how often Update retries a busy lock while it waits.
const lockRetry = 10 * time.Millisecond

// afterLock, when not nil, is called each time lock has taken the file lock,
// before it checks the lock file is still the one at the lock path. It is a
// test seam, nil outside tests.
var afterLock func()

// Fingerprint is the SHA-256 of a file's bytes. It identifies one version of
// the file's content. The zero Fingerprint stands for a file that does not
// exist (see IsMissing).
type Fingerprint [32]byte

// IsMissing reports whether f is the fingerprint of a file that does not
// exist: the zero value.
func (f Fingerprint) IsMissing() bool { return f == Fingerprint{} }

// ErrBusy is returned by Update when it gave up waiting for another writer to
// release the lock.
var ErrBusy = errors.New("lock busy: another writer holds it")

// ErrInvalid matches (via errors.Is) every *InvalidError: a file that could not
// be decoded or that failed the Validate hook.
var ErrInvalid = errors.New("invalid file")

// InvalidError reports a file whose bytes do not decode into the stored type
// or whose decoded value fails Config.Validate. Its message is the inner
// error's message unchanged, so callers can show it as is.
type InvalidError struct {
	// Err is the decode or validation error.
	Err error
}

// Error returns the inner error's message.
func (e *InvalidError) Error() string { return e.Err.Error() }

// Unwrap returns the inner error, so errors.Is and errors.As reach it.
func (e *InvalidError) Unwrap() error { return e.Err }

// Is reports whether target is ErrInvalid.
func (e *InvalidError) Is(target error) bool { return target == ErrInvalid }

// ErrInvalidValue matches (via errors.Is) every *InvalidValueError: a value
// Update refused to write because it failed the Validate hook. It is not
// ErrInvalid: the file on disk is fine and was left as it was.
var ErrInvalidValue = errors.New("invalid value")

// InvalidValueError reports a value the Update callback produced that fails
// Config.Validate, so Update wrote nothing. That is a bug in the caller, not a
// problem with the file. Its message is the inner error's message unchanged.
type InvalidValueError struct {
	// Err is the validation error.
	Err error
}

// Error returns the inner error's message.
func (e *InvalidValueError) Error() string { return e.Err.Error() }

// Unwrap returns the inner error, so errors.Is and errors.As reach it.
func (e *InvalidValueError) Unwrap() error { return e.Err }

// Is reports whether target is ErrInvalidValue.
func (e *InvalidValueError) Is(target error) bool { return target == ErrInvalidValue }

// Config configures a Store.
type Config[T any] struct {
	// Path is the JSON file. The lock file is Path + ".lock" (or, when Path
	// is a symlink, the linked file's path + ".lock"): see LockPath.
	Path string
	// LockTimeout is how long Update waits for the lock before failing with
	// ErrBusy. Zero or negative means DefaultLockTimeout.
	LockTimeout time.Duration
	// Validate, when not nil, checks every decoded value and every value
	// Update is about to write. An error on a decoded value makes the file
	// invalid: Load and Update return it as an *InvalidError. An error on a
	// value to write refuses it: Update returns an *InvalidValueError. Either
	// way Update writes nothing.
	Validate func(*T) error
}

// Store keeps a value of type T in the JSON file at Config.Path. A Store is
// safe for concurrent use, and several Stores (in one process or many) may
// share one path.
type Store[T any] struct {
	path        string
	lockTimeout time.Duration
	validate    func(*T) error

	// mu guards the stat cache used by Changed: the last stat, the fingerprint
	// of the bytes read after it and when they were read.
	mu       sync.Mutex
	statInfo fs.FileInfo
	statFP   Fingerprint
	statAt   time.Time
}

// Snapshot describes the file as Update read it under the lock, for fn to
// check preconditions on (say, that the file exists) or to copy the bytes
// (say, to back the file up) without reading it again outside the lock.
type Snapshot struct {
	// Path is the file read: Config.Path, or the file it links to when it is
	// a symlink (see Update). The write, if any, replaces this file.
	Path string
	// Exists reports whether there was a file.
	Exists bool
	// Bytes is the file's content, nil when there was no file. It is fn's to
	// keep; the Store does not use it again.
	Bytes []byte
	// Fingerprint is the fingerprint of Bytes (the zero, missing one when
	// there was no file). It is Result.Read.
	Fingerprint Fingerprint
}

// Result is the outcome of an Update.
type Result[T any] struct {
	// Value is the value after fn ran: the one written, or the one read when
	// nothing was written.
	Value T
	// Read is the fingerprint of the file as Update found it under the lock
	// (the zero, missing fingerprint when there was no file). Comparing it with the fingerprint of
	// the caller's last Load tells whether someone else wrote in between.
	Read Fingerprint
	// Wrote is the fingerprint of the bytes Update wrote, or Read when it
	// wrote nothing.
	Wrote Fingerprint
	// Written reports whether the file was replaced.
	Written bool
}

// lockSuffix is appended to the data file's path to name its lock file.
const lockSuffix = ".lock"

// LockPath returns the lock file Update takes for a Store on path: path, with
// symlinks to the file followed as Update follows them, plus ".lock". Another
// program holding an exclusive flock on it excludes every Update on path. When
// the links cannot be followed (a loop, an unreadable link), it is path plus
// ".lock"; Update fails on such a path before it locks anything.
func LockPath(path string) string {
	if p, err := resolveLinks(path); err == nil {
		path = p
	}
	return path + lockSuffix
}

// New returns a Store for cfg. It does no I/O: the file and the lock file are
// only touched by Load, Update and Changed.
func New[T any](cfg Config[T]) *Store[T] {
	timeout := cfg.LockTimeout
	if timeout <= 0 {
		timeout = DefaultLockTimeout
	}
	return &Store[T]{path: cfg.Path, lockTimeout: timeout, validate: cfg.Validate}
}

// Path returns the JSON file's path.
func (s *Store[T]) Path() string { return s.path }

// Load reads the file without taking the lock, decodes it and runs the
// Validate hook. A missing file gives the zero T, the zero (missing)
// fingerprint and a nil error. A file that fails to decode or validate gives
// an *InvalidError, along with the fingerprint of the bytes read.
func (s *Store[T]) Load() (T, Fingerprint, error) {
	v, cur, err := s.read(s.path)
	return v, cur.Fingerprint, err
}

// Update runs a locked read-modify-write. It waits up to the lock timeout for
// the lock on Path + ".lock" (ErrBusy when it gives up, or ctx's error when
// ctx is cancelled), then reads the file as Load does. When Path is a symlink,
// Update works on the file it points at: the lock is that file's + ".lock",
// so Stores on different links to one file share one lock, and the write
// replaces that file, leaving the link in place. An unreadable or
// invalid file is returned as an error and never overwritten. Otherwise fn
// gets the fresh value (the zero T for a missing file), which it may change,
// and a Snapshot of the file it came from. When fn returns an error, or
// changed == false, nothing is written. Otherwise the changed value is run
// through the Validate hook (an *InvalidValueError writes nothing), encoded as
// indented JSON, and atomically replaces the file. On error the Result is the
// zero value.
func (s *Store[T]) Update(ctx context.Context, fn func(v *T, cur Snapshot) (changed bool, err error)) (Result[T], error) {
	path, err := resolveLinks(s.path)
	if err != nil {
		return Result[T]{}, err
	}
	unlock, err := s.lock(ctx, path+lockSuffix)
	if err != nil {
		return Result[T]{}, err
	}
	defer unlock()

	v, cur, err := s.read(path)
	if err != nil {
		return Result[T]{}, err
	}
	read := cur.Fingerprint
	changed, err := fn(&v, cur)
	if err != nil {
		return Result[T]{}, err
	}
	if !changed {
		return Result[T]{Value: v, Read: read, Wrote: read}, nil
	}
	// Validate what fn produced too: a value that fails here would make every
	// later Load and Update refuse the file.
	if s.validate != nil {
		if err := s.validate(&v); err != nil {
			return Result[T]{}, &InvalidValueError{Err: err}
		}
	}

	data, err := encode(&v)
	if err != nil {
		return Result[T]{}, err
	}
	if err := atomicWrite(path, data); err != nil {
		return Result[T]{}, fmt.Errorf("write %s: %w", path, err)
	}
	return Result[T]{Value: v, Read: read, Wrote: sha256.Sum256(data), Written: true}, nil
}

// lock takes the exclusive lock on the lock file at path and returns its
// release. A new lock file gets mode 0o666 less the umask, as the data file
// does.
//
// The lock only excludes writers that lock the same file. When the lock file
// was deleted (or replaced) between another writer's open and its lock, the
// file this one locked is no longer the one at path, and a writer arriving now
// would create and lock a new one. So once locked, lock checks the file it
// holds is still the one at path, and otherwise lets go and tries again.
func (s *Store[T]) lock(ctx context.Context, path string) (func(), error) {
	lockCtx, cancel := context.WithTimeout(ctx, s.lockTimeout)
	defer cancel()

	for {
		fl := flock.New(path, flock.SetPermissions(0o666))
		ok, err := fl.TryLockContext(lockCtx, lockRetry)
		if !ok {
			return nil, lockErr(ctx, path, err)
		}
		if afterLock != nil {
			afterLock()
		}
		current, err := holdsFileAt(fl, path)
		if err != nil {
			_ = fl.Unlock()
			return nil, fmt.Errorf("lock %s: %w", path, err)
		}
		if current {
			return func() { _ = fl.Unlock() }, nil
		}
		_ = fl.Unlock()
	}
}

// lockErr turns a failed TryLockContext into lock's error: ctx's own error
// when the caller cancelled, ErrBusy when the lock timeout ran out, and err
// wrapped with the lock path otherwise.
func lockErr(ctx context.Context, path string, err error) error {
	if ctxErr := ctx.Err(); ctxErr != nil && !errors.Is(ctxErr, context.DeadlineExceeded) {
		return ctxErr
	}
	if err == nil || errors.Is(err, context.DeadlineExceeded) {
		return ErrBusy
	}
	return fmt.Errorf("lock %s: %w", path, err)
}

// holdsFileAt reports whether the file fl has locked is the file at path now.
// A path with no file is not.
func holdsFileAt(fl *flock.Flock, path string) (bool, error) {
	held, err := fl.Stat() // the open handle's file, not the path's
	if err != nil {
		return false, err
	}
	now, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return os.SameFile(held, now), nil
}

// read reads, fingerprints, decodes and validates the file at path (the
// Store's path, or the file it links to), and returns the value with a
// Snapshot of the file. The Snapshot is filled in as far as the read got, so
// an invalid file still has its fingerprint.
func (s *Store[T]) read(path string) (T, Snapshot, error) {
	var zero T
	cur := Snapshot{Path: path}
	data, err := os.ReadFile(path) //nolint:gosec // path is the Store's own file (Config.Path or the file it links to)
	if errors.Is(err, fs.ErrNotExist) {
		return zero, cur, nil
	}
	if err != nil {
		return zero, cur, err
	}
	cur.Exists, cur.Bytes, cur.Fingerprint = true, data, sha256.Sum256(data)

	var v T
	if err := json.Unmarshal(data, &v); err != nil {
		return zero, cur, &InvalidError{Err: err}
	}
	if s.validate != nil {
		if err := s.validate(&v); err != nil {
			return zero, cur, &InvalidError{Err: err}
		}
	}
	return v, cur, nil
}

// encode renders v as 2-space indented JSON with a trailing newline, without
// escaping HTML characters.
func encode[T any](v *T) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
