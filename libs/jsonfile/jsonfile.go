// Package jsonfile keeps one Go value in one JSON file that several processes
// can share safely. Writes go through Update, a read-modify-write that holds an
// exclusive lock on a separate `<path>.lock` file and replaces the data file
// atomically (temp file, fsync, rename), so a reader never sees a half-written
// file. Reads (Load) and change polling (Changed) take no lock. The package is
// generic over the stored type and knows nothing about what it stores: the
// path, the lock timeout and the validation hook all come from Config.
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

// Fingerprint is the SHA-256 of a file's bytes. It identifies one version of
// the file's content.
type Fingerprint [32]byte

// Missing is the fingerprint of a file that does not exist (the zero value).
var Missing Fingerprint

// ErrBusy is returned by Update when it gave up waiting for another writer to
// release the lock.
var ErrBusy = errors.New("jsonfile: lock busy")

// ErrInvalid matches (via errors.Is) every *InvalidError: a file that could not
// be decoded or that failed the Validate hook.
var ErrInvalid = errors.New("jsonfile: invalid file")

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

// Config configures a Store.
type Config[T any] struct {
	// Path is the JSON file. The lock file is Path + ".lock".
	Path string
	// LockTimeout is how long Update waits for the lock before failing with
	// ErrBusy. Zero or negative means DefaultLockTimeout.
	LockTimeout time.Duration
	// Validate, when not nil, checks every decoded value and every value
	// Update is about to write. An error makes the value invalid: Load and
	// Update return it as an *InvalidError, and Update writes nothing.
	Validate func(*T) error
}

// Store keeps a value of type T in the JSON file at Config.Path. A Store is
// safe for concurrent use, and several Stores (in one process or many) may
// share one path.
type Store[T any] struct {
	path        string
	lockTimeout time.Duration
	validate    func(*T) error

	// mu guards the stat cache used by Changed.
	mu       sync.Mutex
	statInfo fs.FileInfo
	statFP   Fingerprint
}

// Result is the outcome of an Update.
type Result[T any] struct {
	// Value is the value after fn ran: the one written, or the one read when
	// nothing was written.
	Value T
	// Read is the fingerprint of the file as Update found it under the lock
	// (Missing when there was no file). Comparing it with the fingerprint of
	// the caller's last Load tells whether someone else wrote in between.
	Read Fingerprint
	// Wrote is the fingerprint of the bytes Update wrote, or Read when it
	// wrote nothing.
	Wrote Fingerprint
	// Written reports whether the file was replaced.
	Written bool
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
// Validate hook. A missing file gives the zero T, Missing and a nil error. A
// file that fails to decode or validate gives an *InvalidError, along with the
// fingerprint of the bytes read.
func (s *Store[T]) Load() (T, Fingerprint, error) {
	return s.read()
}

// Update runs a locked read-modify-write. It waits up to the lock timeout for
// the lock on Path + ".lock" (ErrBusy when it gives up, or ctx's error when
// ctx is cancelled), then reads the file as Load does. An unreadable or
// invalid file is returned as an error and never overwritten. Otherwise fn
// gets the fresh value (the zero T for a missing file) and may change it. When
// fn returns an error, or changed == false, nothing is written. Otherwise the
// changed value is run through the Validate hook (an *InvalidError writes
// nothing), encoded as indented JSON, and atomically replaces the file. On
// error the Result is the zero value.
func (s *Store[T]) Update(ctx context.Context, fn func(v *T) (changed bool, err error)) (Result[T], error) {
	unlock, err := s.lock(ctx)
	if err != nil {
		return Result[T]{}, err
	}
	defer unlock()

	v, read, err := s.read()
	if err != nil {
		return Result[T]{}, err
	}
	changed, err := fn(&v)
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
			return Result[T]{}, &InvalidError{Err: err}
		}
	}

	data, err := encode(&v)
	if err != nil {
		return Result[T]{}, err
	}
	if err := atomicWrite(s.path, data); err != nil {
		return Result[T]{}, err
	}
	return Result[T]{Value: v, Read: read, Wrote: sha256.Sum256(data), Written: true}, nil
}

// lock takes the exclusive lock on the lock file and returns its release.
func (s *Store[T]) lock(ctx context.Context) (func(), error) {
	lockCtx, cancel := context.WithTimeout(ctx, s.lockTimeout)
	defer cancel()

	fl := flock.New(s.path + ".lock")
	ok, err := fl.TryLockContext(lockCtx, lockRetry)
	if ok {
		return func() { _ = fl.Unlock() }, nil
	}
	if ctxErr := ctx.Err(); ctxErr != nil && !errors.Is(ctxErr, context.DeadlineExceeded) {
		return nil, ctxErr
	}
	if err == nil || errors.Is(err, context.DeadlineExceeded) {
		return nil, ErrBusy
	}
	return nil, fmt.Errorf("jsonfile: lock %s: %w", fl.Path(), err)
}

// read reads, fingerprints, decodes and validates the file.
func (s *Store[T]) read() (T, Fingerprint, error) {
	var zero T
	data, err := os.ReadFile(s.path)
	if errors.Is(err, fs.ErrNotExist) {
		return zero, Missing, nil
	}
	if err != nil {
		return zero, Missing, err
	}
	fp := Fingerprint(sha256.Sum256(data))

	var v T
	if err := json.Unmarshal(data, &v); err != nil {
		return zero, fp, &InvalidError{Err: err}
	}
	if s.validate != nil {
		if err := s.validate(&v); err != nil {
			return zero, fp, &InvalidError{Err: err}
		}
	}
	return v, fp, nil
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
