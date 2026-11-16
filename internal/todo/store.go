package todo

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/candy-tools/todo/libs/jsonfile"
)

// newerSchemaError returns ErrNewerSchema wrapped with the format version
// found in the file.
func newerSchemaError(version int) error {
	return fmt.Errorf("%w (format version %d); upgrade todo", ErrNewerSchema, version)
}

// Fingerprint identifies one version of the list file's content (see
// jsonfile.Fingerprint). The zero Fingerprint, for which IsMissing is true,
// stands for a list file that does not exist.
type Fingerprint = jsonfile.Fingerprint

// DefaultBusyTimeout is how long a store's writes wait, by default, for
// another process to release the write lock before failing with ErrBusy.
const DefaultBusyTimeout = jsonfile.DefaultLockTimeout

// Option configures NewStore.
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

// WriteResult is the outcome of a successful store write, returned by every
// write method: the list it left and the fingerprints of the file before and
// after it. A write that changed nothing has Written unset and Read == Wrote.
// A failed write returns the zero WriteResult.
type WriteResult struct {
	// Doc is the list after the write, as a Load of the file would return it.
	// It is the caller's own: the Store keeps no reference to it.
	Doc *Document
	// Read is the fingerprint of the file as the write found it under the
	// lock. When it differs from the caller's last Load, someone else wrote in
	// between.
	Read Fingerprint
	// Wrote is the fingerprint of the file the write left.
	Wrote Fingerprint
	// Written reports whether the write replaced the file.
	Written bool
}

// Store is a TODO list kept in one JSON file. Every write is one locked
// read-modify-write of the file as it is now (see write), so several Stores, in
// one process or many, may share a path. Reads take no lock. A Store is safe
// for concurrent use.
type Store struct {
	file *jsonfile.Store[diskList]
	// mustExist makes every write fail with ErrListMissing on a missing file
	// (see Existing).
	mustExist bool
}

// NewStore returns a store for the list file at path. It does no I/O and
// holds no open file, so there is nothing to close: a missing file reads as an
// empty list, and the first write creates it.
func NewStore(path string, opts ...Option) *Store {
	var cfg storeCfg
	for _, o := range opts {
		o(&cfg)
	}
	file := jsonfile.New(jsonfile.Config[diskList]{
		Path:        path,
		LockTimeout: cfg.busyTimeout, // jsonfile turns a non-positive one into the default
		Decode:      decodeList,
		Validate:    func(l *diskList) error { return l.validate() },
	})
	return &Store{file: file}
}

// Path returns the path of the list file.
func (s *Store) Path() string { return s.file.Path() }

// Existing returns a store on the same file whose writes require the file to
// exist: on a missing file each write, even one that would change nothing,
// fails with ErrListMissing and writes nothing (it does not create the list
// file; the lock file next to it may be). The check is made under the write's
// lock, before the write looks at the list. A caller that has read the file
// writes through Existing so that a file removed since is not silently
// recreated as a new list.
func (s *Store) Existing() *Store {
	c := *s
	c.mustExist = true
	return &c
}

// Load reads the whole list and the fingerprint of the bytes it came from,
// without taking the lock. A missing file is an empty Document and the zero
// (missing) fingerprint. A file that is not a valid list fails with
// ErrInvalidList (or ErrNewerSchema) naming the path, and still returns the
// fingerprint of the bytes read.
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

// write runs fn as one store write: a single jsonfile Update that holds the
// lock, reads the file as it is now into a Document (an empty one for a
// missing file, or ErrListMissing for a store from Existing) and runs fn on
// it. When fn reports a change, the Document is written back, validated
// first; when it reports none, or fails, nothing is written. It returns the
// write's result, the zero one on error. Errors are mapped as mapErr does.
func (s *Store) write(ctx context.Context, fn func(d *Document) (changed bool, err error)) (WriteResult, error) {
	return s.update(ctx, func(d *Document, _ jsonfile.Snapshot) (bool, error) { return fn(d) })
}

// update is write with fn also given the Snapshot of the file as the write
// read it under the lock.
func (s *Store) update(ctx context.Context, fn func(d *Document, cur jsonfile.Snapshot) (changed bool, err error)) (WriteResult, error) {
	res, err := s.file.Update(ctx, func(l *diskList, cur jsonfile.Snapshot) (bool, error) {
		if s.mustExist && !cur.Exists {
			return false, ErrListMissing
		}
		d := fromDisk(*l)
		changed, err := fn(d, cur)
		if err != nil || !changed {
			return false, err
		}
		*l = toDisk(d)
		return true, nil
	})
	if err != nil {
		return WriteResult{}, s.mapErr(err)
	}
	// The Document is rebuilt from what was written, so it matches what a
	// Load of the file returns.
	return WriteResult{Doc: fromDisk(res.Value), Read: res.Read, Wrote: res.Wrote, Written: res.Written}, nil
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
		if errors.Is(err, ErrInvalidList) { // ErrNewerSchema included
			return fmt.Errorf("%s: %w", s.Path(), err)
		}
		return fmt.Errorf("%s: %w: %w", s.Path(), ErrInvalidList, err)
	}
	return err
}

// refusedError is a write that produced a list the format refuses, so nothing
// was written: a bug in the write, not a problem with the file. Its class is
// ErrInternal, and it does not wrap the validation error, so it never reads as
// ErrInvalidList (a bad file); its message says what the validation refused.
type refusedError struct {
	err error
}

// Error returns the validation failure, minus its "invalid list file" prefix.
func (e *refusedError) Error() string {
	msg := strings.TrimPrefix(e.err.Error(), ErrInvalidList.Error()+": ")
	return "refused to write an invalid list: " + msg
}

// Is reports whether target is ErrInternal.
func (e *refusedError) Is(target error) bool { return target == ErrInternal }
