package todo

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/candy-tools/todo/libs/jsonfile"
)

// ErrNewerSchema is returned when the list file was written by a newer todo,
// whose format this binary does not know and must not read or write. It is
// wrapped with the version found.
var ErrNewerSchema = errors.New("list was written by a newer todo")

// ErrBusy is returned by a write that gave up waiting for another process to
// release the list's write lock (after the store's busy timeout).
var ErrBusy = errors.New("list busy (another todo is writing), try again")

// newerSchemaError returns ErrNewerSchema wrapped with the format version
// found in the file.
func newerSchemaError(version int) error {
	return fmt.Errorf("%w (schema %d); upgrade todo", ErrNewerSchema, version)
}

// Fingerprint identifies one version of the list file's content (see
// jsonfile.Fingerprint).
type Fingerprint = jsonfile.Fingerprint

// Missing is the fingerprint of a list file that does not exist.
var Missing = jsonfile.Missing

// DefaultBusyTimeout is how long a store's writes wait, by default, for
// another process to release the write lock before failing with ErrBusy.
const DefaultBusyTimeout = jsonfile.DefaultLockTimeout

// Option configures OpenStore.
type Option func(*storeCfg)

// storeCfg holds the settings Options change.
type storeCfg struct {
	busyTimeout time.Duration
}

// WithBusyTimeout sets how long the store's writes, imports included, wait for
// another process to release the write lock before failing with ErrBusy
// (DefaultBusyTimeout when not given, or when d is not positive). An
// interactive caller keeps it short, so a stuck writer elsewhere does not
// freeze it.
func WithBusyTimeout(d time.Duration) Option {
	return func(c *storeCfg) { c.busyTimeout = d }
}

// WriteResult is the outcome of a store write: the list it left and the
// fingerprints of the file before and after it. A write that changed nothing
// has Read == Wrote.
type WriteResult struct {
	// Doc is the list after the write, as a Load of the file would return it.
	Doc *Document
	// Read is the fingerprint of the file as the write found it under the
	// lock. When it differs from the caller's last Load, someone else wrote in
	// between.
	Read Fingerprint
	// Wrote is the fingerprint of the file the write left.
	Wrote Fingerprint
}

// Store is a TODO list kept in one JSON file. Every write is one locked
// read-modify-write of the file as it is now (see write), so several Stores, in
// one process or many, may share a path. Reads take no lock. A Store is safe
// for concurrent reads and writes, but LastWrite describes the caller's own
// write only while a single goroutine writes through the Store: with several,
// it may be another goroutine's later write.
type Store struct {
	file        *jsonfile.Store[diskList]
	busyTimeout time.Duration

	mu   sync.Mutex
	last WriteResult
}

// OpenStore returns a store for the list file at path. It does no I/O: a
// missing file reads as an empty list, and the first write creates it.
func OpenStore(path string, opts ...Option) (*Store, error) {
	cfg := storeCfg{busyTimeout: DefaultBusyTimeout}
	for _, o := range opts {
		o(&cfg)
	}
	if cfg.busyTimeout <= 0 {
		cfg.busyTimeout = DefaultBusyTimeout
	}
	file := jsonfile.New(jsonfile.Config[diskList]{
		Path:        path,
		LockTimeout: cfg.busyTimeout,
		Validate:    func(l *diskList) error { return l.validate() },
	})
	return &Store{file: file, busyTimeout: cfg.busyTimeout}, nil
}

// Close releases nothing and always returns nil: a Store holds no open file
// between calls. It is kept so callers need not change.
func (s *Store) Close() error { return nil }

// Path returns the path of the list file.
func (s *Store) Path() string { return s.file.Path() }

// StoreExists reports whether a list file exists at path.
func StoreExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// Load reads the whole list and the fingerprint of the bytes it came from,
// without taking the lock. A missing file is an empty Document and Missing. A
// file that is not a valid list fails with ErrInvalidList (or ErrNewerSchema)
// naming the path, and still returns the fingerprint of the bytes read.
func (s *Store) Load(_ context.Context) (*Document, Fingerprint, error) {
	l, fp, err := s.file.Load()
	if err != nil {
		return nil, fp, s.mapErr(err)
	}
	return fromDisk(l), fp, nil
}

// Changed reports whether the file's content differs from since, and returns
// its current fingerprint. It is cheap enough to poll (see jsonfile's
// Changed): it does not decode or validate the file.
func (s *Store) Changed(_ context.Context, since Fingerprint) (bool, Fingerprint, error) {
	return s.file.Changed(since)
}

// LastWrite returns the result of this Store's last successful write,
// including one that changed nothing (Read == Wrote). It is the zero
// WriteResult before the first. It is "this write's" result only when one
// goroutine writes through the Store (see Store).
func (s *Store) LastWrite() WriteResult {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.last
}

// write runs fn as one store write: a single jsonfile Update that holds the
// lock, reads the file as it is now into a Document (an empty one for a
// missing file) and runs fn on it. When fn reports a change, the Document is
// written back, validated first; when it reports none, or fails, nothing is
// written. A successful write is recorded as LastWrite. Errors are mapped as
// mapErr does.
func (s *Store) write(ctx context.Context, fn func(d *Document) (changed bool, err error)) error {
	res, err := s.file.Update(ctx, func(l *diskList) (bool, error) {
		d := fromDisk(*l)
		changed, err := fn(d)
		if err != nil || !changed {
			return false, err
		}
		*l = toDisk(d)
		return true, nil
	})
	if err != nil {
		return s.mapErr(err)
	}
	// The Document is rebuilt from what was written, so it matches what a
	// Load of the file returns.
	doc := fromDisk(res.Value)
	s.mu.Lock()
	s.last = WriteResult{Doc: doc, Read: res.Read, Wrote: res.Wrote}
	s.mu.Unlock()
	return nil
}

// mapErr turns jsonfile's errors into the package's: a lock timeout is
// ErrBusy, a file that fails to decode or validate is ErrInvalidList (or
// ErrNewerSchema), prefixed with the path, and a list a write produced that
// fails validation is a refusedError (a bug, not a bad file). Other errors
// pass through.
func (s *Store) mapErr(err error) error {
	switch {
	case errors.Is(err, jsonfile.ErrBusy):
		return ErrBusy
	case errors.Is(err, jsonfile.ErrInvalidValue):
		return &refusedError{err: err}
	case errors.Is(err, jsonfile.ErrInvalid):
		if errors.Is(err, ErrInvalidList) || errors.Is(err, ErrNewerSchema) {
			return fmt.Errorf("%s: %w", s.Path(), err)
		}
		return fmt.Errorf("%s: %w: %w", s.Path(), ErrInvalidList, err)
	}
	return err
}

// refusedError is a write that produced a list the format refuses, so nothing
// was written: a bug in the write, not a problem with the file. Its message
// says so without calling the file invalid; it still wraps the validation
// error, so errors.Is reaches ErrInvalidList and the like.
type refusedError struct {
	err error
}

// Error returns the validation failure, minus its "invalid list file" prefix.
func (e *refusedError) Error() string {
	msg := strings.TrimPrefix(e.err.Error(), ErrInvalidList.Error()+": ")
	return "refused to write an invalid list: " + msg
}

// Unwrap returns the validation error.
func (e *refusedError) Unwrap() error { return e.err }
