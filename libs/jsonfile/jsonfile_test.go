package jsonfile_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gofrs/flock"

	"github.com/candy-tools/todo/libs/jsonfile"
)

type doc struct {
	N    int      `json:"n"`
	Tags []string `json:"tags"`
}

func tempPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "list.json")
}

func setN(n int) func(*doc, jsonfile.Snapshot) (bool, error) {
	return func(v *doc, _ jsonfile.Snapshot) (bool, error) {
		v.N = n
		return true, nil
	}
}

func mustUpdate(t *testing.T, s *jsonfile.Store[doc], fn func(*doc, jsonfile.Snapshot) (bool, error)) jsonfile.Result[doc] {
	t.Helper()
	res, err := s.Update(context.Background(), fn)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	return res
}

func readBytes(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return b
}

func TestLoadMissing(t *testing.T) {
	s := jsonfile.New(jsonfile.Config[doc]{Path: tempPath(t)})
	v, fp, err := s.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if v.N != 0 || v.Tags != nil {
		t.Errorf("value = %+v, want zero", v)
	}
	if !fp.IsMissing() {
		t.Errorf("fingerprint = %x, want missing", fp)
	}
}

func TestUpdateCreatesAndLoads(t *testing.T) {
	path := tempPath(t)
	s := jsonfile.New(jsonfile.Config[doc]{Path: path})
	if s.Path() != path {
		t.Errorf("Path() = %q, want %q", s.Path(), path)
	}
	res := mustUpdate(t, s, setN(1))
	if !res.Written {
		t.Error("Written = false, want true")
	}
	if !res.Read.IsMissing() {
		t.Errorf("Read = %x, want missing", res.Read)
	}
	if res.Wrote.IsMissing() {
		t.Error("Wrote is missing, want a fingerprint")
	}
	if res.Value.N != 1 {
		t.Errorf("Value.N = %d, want 1", res.Value.N)
	}

	v, fp, err := s.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if v.N != 1 {
		t.Errorf("loaded N = %d, want 1", v.N)
	}
	if fp != res.Wrote {
		t.Errorf("Load fingerprint = %x, want Wrote %x", fp, res.Wrote)
	}
	want := "{\n  \"n\": 1,\n  \"tags\": null\n}\n"
	if got := string(readBytes(t, path)); got != want {
		t.Errorf("file = %q, want %q", got, want)
	}
}

// snapshotOf runs an Update that changes nothing and returns the Snapshot fn
// got.
func snapshotOf(t *testing.T, s *jsonfile.Store[doc]) (jsonfile.Snapshot, jsonfile.Result[doc]) {
	t.Helper()
	var snap jsonfile.Snapshot
	res := mustUpdate(t, s, func(_ *doc, cur jsonfile.Snapshot) (bool, error) {
		snap = cur
		return false, nil
	})
	return snap, res
}

func TestUpdateSnapshotOfMissingFile(t *testing.T) {
	path := tempPath(t)
	snap, _ := snapshotOf(t, jsonfile.New(jsonfile.Config[doc]{Path: path}))
	if snap.Path != path || snap.Exists || snap.Bytes != nil || !snap.Fingerprint.IsMissing() {
		t.Errorf("Snapshot = %+v, want {Path: %q, Exists: false, no bytes, missing fingerprint}", snap, path)
	}
}

func TestUpdateSnapshotOfExistingFile(t *testing.T) {
	path := tempPath(t)
	s := jsonfile.New(jsonfile.Config[doc]{Path: path})
	wrote := mustUpdate(t, s, setN(1)).Wrote

	snap, res := snapshotOf(t, s)
	if snap.Path != path || !snap.Exists {
		t.Errorf("Snapshot Path, Exists = %q, %v; want %q, true", snap.Path, snap.Exists, path)
	}
	if want := readBytes(t, path); !bytes.Equal(snap.Bytes, want) {
		t.Errorf("Snapshot.Bytes = %q, want the file's %q", snap.Bytes, want)
	}
	if snap.Fingerprint != wrote || snap.Fingerprint != res.Read {
		t.Errorf("Snapshot.Fingerprint = %x, want the file's %x (Result.Read %x)", snap.Fingerprint, wrote, res.Read)
	}
}

func TestUpdateNoChangeDoesNotWrite(t *testing.T) {
	path := tempPath(t)
	s := jsonfile.New(jsonfile.Config[doc]{Path: path})
	mustUpdate(t, s, setN(1))
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	// Make a rewrite observable through the mtime even on coarse clocks.
	old := before.ModTime().Add(-time.Hour)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
	before, err = os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}

	res := mustUpdate(t, s, func(*doc, jsonfile.Snapshot) (bool, error) { return false, nil })
	if res.Written {
		t.Error("Written = true, want false")
	}
	if res.Wrote != res.Read {
		t.Errorf("Wrote = %x, want Read %x", res.Wrote, res.Read)
	}
	after, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !after.ModTime().Equal(before.ModTime()) {
		t.Errorf("ModTime changed: %v -> %v", before.ModTime(), after.ModTime())
	}
	if !os.SameFile(before, after) {
		t.Error("file was replaced (inode changed)")
	}
}

func TestUpdateFnErrorWritesNothing(t *testing.T) {
	path := tempPath(t)
	s := jsonfile.New(jsonfile.Config[doc]{Path: path})
	mustUpdate(t, s, setN(1))
	before := readBytes(t, path)

	boom := errors.New("boom")
	_, err := s.Update(context.Background(), func(v *doc, _ jsonfile.Snapshot) (bool, error) {
		v.N = 99
		return true, boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want boom", err)
	}
	if after := readBytes(t, path); !bytes.Equal(before, after) {
		t.Errorf("file changed: %q -> %q", before, after)
	}
}

func TestUpdateRefusesInvalidFile(t *testing.T) {
	cases := []struct {
		name     string
		content  string
		validate func(*doc) error
	}{
		{name: "decode", content: `{"n": "x"}`},
		{
			name:    "validate",
			content: `{"n":7}`,
			validate: func(v *doc) error {
				if v.N == 7 {
					return errors.New("n must not be 7")
				}
				return nil
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := tempPath(t)
			if err := os.WriteFile(path, []byte(tc.content), 0o644); err != nil {
				t.Fatal(err)
			}
			s := jsonfile.New(jsonfile.Config[doc]{Path: path, Validate: tc.validate})
			called := false
			_, err := s.Update(context.Background(), func(*doc, jsonfile.Snapshot) (bool, error) {
				called = true
				return true, nil
			})
			if !errors.Is(err, jsonfile.ErrInvalid) {
				t.Fatalf("err = %v, want ErrInvalid", err)
			}
			if errors.Is(err, jsonfile.ErrInvalidValue) {
				t.Errorf("err = %v is ErrInvalidValue, want only ErrInvalid", err)
			}
			var inv *jsonfile.InvalidError
			if !errors.As(err, &inv) {
				t.Fatalf("err = %T, want *InvalidError", err)
			}
			if err.Error() != inv.Err.Error() {
				t.Errorf("Error() = %q, want the inner message %q", err.Error(), inv.Err.Error())
			}
			if called {
				t.Error("fn was called on an invalid file")
			}
			if got := string(readBytes(t, path)); got != tc.content {
				t.Errorf("file = %q, want %q", got, tc.content)
			}
			if _, _, err := s.Load(); !errors.Is(err, jsonfile.ErrInvalid) {
				t.Errorf("Load err = %v, want ErrInvalid", err)
			}
		})
	}
}

func TestInvalidErrorUnwrap(t *testing.T) {
	inner := errors.New("inner")
	err := error(&jsonfile.InvalidError{Err: inner})
	if err.Error() != "inner" {
		t.Errorf("Error() = %q, want %q", err.Error(), "inner")
	}
	if !errors.Is(err, inner) {
		t.Error("errors.Is(err, inner) = false")
	}
	if !errors.Is(err, jsonfile.ErrInvalid) {
		t.Error("errors.Is(err, ErrInvalid) = false")
	}
}

func TestInvalidValueErrorUnwrap(t *testing.T) {
	inner := errors.New("inner")
	err := error(&jsonfile.InvalidValueError{Err: inner})
	if err.Error() != "inner" {
		t.Errorf("Error() = %q, want %q", err.Error(), "inner")
	}
	if !errors.Is(err, inner) || !errors.Is(err, jsonfile.ErrInvalidValue) {
		t.Error("errors.Is(err, inner / ErrInvalidValue) = false")
	}
	if errors.Is(err, jsonfile.ErrInvalid) {
		t.Error("errors.Is(err, ErrInvalid) = true for a refused value")
	}
}

func TestUpdateReportsOutsideWrite(t *testing.T) {
	path := tempPath(t)
	a := jsonfile.New(jsonfile.Config[doc]{Path: path})
	b := jsonfile.New(jsonfile.Config[doc]{Path: path})
	f1 := mustUpdate(t, a, setN(1)).Wrote
	mustUpdate(t, b, setN(2))

	var seen int
	res := mustUpdate(t, a, func(v *doc, _ jsonfile.Snapshot) (bool, error) {
		seen = v.N
		return false, nil
	})
	if res.Read == f1 {
		t.Error("Read == f1, want the fingerprint of B's write")
	}
	if seen != 2 {
		t.Errorf("fn saw N = %d, want 2", seen)
	}
}

func TestConcurrentUpdatesLoseNothing(t *testing.T) {
	path := tempPath(t)
	const workers, rounds = 8, 25
	var wg sync.WaitGroup
	errs := make(chan error, workers*rounds)
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s := jsonfile.New(jsonfile.Config[doc]{Path: path, LockTimeout: 30 * time.Second})
			for range rounds {
				if _, err := s.Update(context.Background(), func(v *doc, _ jsonfile.Snapshot) (bool, error) {
					v.N++
					return true, nil
				}); err != nil {
					errs <- err
				}
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("Update: %v", err)
	}
	v, _, err := jsonfile.New(jsonfile.Config[doc]{Path: path}).Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if v.N != workers*rounds {
		t.Errorf("N = %d, want %d", v.N, workers*rounds)
	}
}

func TestUpdateBusy(t *testing.T) {
	path := tempPath(t)
	lock := flock.New(path + ".lock")
	if err := lock.Lock(); err != nil {
		t.Fatalf("lock: %v", err)
	}
	defer func() { _ = lock.Unlock() }()

	s := jsonfile.New(jsonfile.Config[doc]{Path: path, LockTimeout: 50 * time.Millisecond})
	start := time.Now()
	_, err := s.Update(context.Background(), setN(1))
	if !errors.Is(err, jsonfile.ErrBusy) {
		t.Fatalf("err = %v, want ErrBusy", err)
	}
	if d := time.Since(start); d > time.Second {
		t.Errorf("Update took %v, want under 1s", d)
	}
}

func TestUpdateCancelledWhileWaiting(t *testing.T) {
	path := tempPath(t)
	lock := flock.New(path + ".lock")
	if err := lock.Lock(); err != nil {
		t.Fatalf("lock: %v", err)
	}
	defer func() { _ = lock.Unlock() }()

	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(20*time.Millisecond, cancel)
	s := jsonfile.New(jsonfile.Config[doc]{Path: path, LockTimeout: 5 * time.Second})
	_, err := s.Update(ctx, setN(1))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

func TestNoHTMLEscape(t *testing.T) {
	path := tempPath(t)
	s := jsonfile.New(jsonfile.Config[doc]{Path: path})
	mustUpdate(t, s, func(v *doc, _ jsonfile.Snapshot) (bool, error) {
		v.Tags = []string{"<a&b>"}
		return true, nil
	})
	if b := readBytes(t, path); !bytes.Contains(b, []byte(`"<a&b>"`)) {
		t.Errorf("file = %q, want the literal <a&b>", b)
	}
}

// TestNoTempFilesLeft writes twice, creating the file and then replacing it:
// each successful write leaves only the file and its lock file behind.
func TestNoTempFilesLeft(t *testing.T) {
	path := tempPath(t)
	s := jsonfile.New(jsonfile.Config[doc]{Path: path})
	for n := 1; n <= 2; n++ {
		mustUpdate(t, s, setN(n))

		entries, err := os.ReadDir(filepath.Dir(path))
		if err != nil {
			t.Fatal(err)
		}
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		if want := []string{"list.json", "list.json.lock"}; !slices.Equal(names, want) {
			t.Errorf("after write %d: dir = %v, want %v", n, names, want)
		}
	}
}

// TestWriteSweepsStaleTempFiles leaves temp files behind as a crashed writer
// would: the next write, holding the lock, removes them, and leaves other
// files alone.
func TestWriteSweepsStaleTempFiles(t *testing.T) {
	path := tempPath(t)
	dir := filepath.Dir(path)
	const suffix = "ABCDEFGHIJKLMNOPQRSTUVWXYZ" // rand.Text's form: 26 of [A-Z2-7]
	stale := []string{".list.json.tmp-" + suffix, ".list.json.tmp-234567ABCDEFGHIJKLMNOPQRST"}
	keep := []string{
		".other.json.tmp-" + suffix,
		"list.json.tmp-" + suffix,
		".list.json.tmp" + suffix,
		".list.json.tmp-x.tmp-" + suffix, // a temp of the list "list.json.tmp-x"
		".list.json.tmp-CRASHED1",        // too short
		".list.json.tmp-" + suffix + "0", // too long
		".list.json.tmp-abcdefghijklmnopqrstuvwxyz",
	}
	for _, name := range append(append([]string{}, stale...), keep...) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("partial"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	mustUpdate(t, jsonfile.New(jsonfile.Config[doc]{Path: path}), setN(1))
	for _, name := range stale {
		assertNotExist(t, filepath.Join(dir, name))
	}
	for _, name := range keep {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("%s was removed: %v", name, err)
		}
	}
}

// TestNoTempFileLeftOnWriteFailure makes the write itself fail: fn swaps the
// file for a non-empty directory, so every rename attempt fails, and the temp
// file must still be cleaned up.
func TestNoTempFileLeftOnWriteFailure(t *testing.T) {
	path := tempPath(t)
	s := jsonfile.New(jsonfile.Config[doc]{Path: path})
	mustUpdate(t, s, setN(1))

	_, err := s.Update(context.Background(), func(v *doc, _ jsonfile.Snapshot) (bool, error) {
		if err := os.Remove(path); err != nil {
			return false, err
		}
		if err := os.Mkdir(path, 0o755); err != nil {
			return false, err
		}
		if err := os.WriteFile(filepath.Join(path, "keep"), nil, 0o644); err != nil {
			return false, err
		}
		v.N = 2
		return true, nil
	})
	if err == nil {
		t.Fatal("Update over a non-empty directory returned nil error")
	}
	assertUserError(t, err, "write "+path+": ")

	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".list.json.tmp-") {
			t.Errorf("temp file left behind: %s", e.Name())
		}
	}
}

func TestUpdateValidatesBeforeWriting(t *testing.T) {
	path := tempPath(t)
	s := jsonfile.New(jsonfile.Config[doc]{
		Path: path,
		Validate: func(v *doc) error {
			if v.N == 7 {
				return errors.New("n must not be 7")
			}
			return nil
		},
	})
	mustUpdate(t, s, setN(1))
	before := readBytes(t, path)

	_, err := s.Update(context.Background(), setN(7))
	if !errors.Is(err, jsonfile.ErrInvalidValue) {
		t.Fatalf("err = %v, want ErrInvalidValue", err)
	}
	if errors.Is(err, jsonfile.ErrInvalid) {
		t.Errorf("err = %v is ErrInvalid, but the file on disk is fine", err)
	}
	var inv *jsonfile.InvalidValueError
	if !errors.As(err, &inv) {
		t.Fatalf("err = %T, want *InvalidValueError", err)
	}
	if err.Error() != "n must not be 7" {
		t.Errorf("Error() = %q, want the Validate message", err.Error())
	}
	if after := readBytes(t, path); !bytes.Equal(before, after) {
		t.Errorf("file changed: %q -> %q", before, after)
	}
}

// assertUserError fails unless err's message starts with prefix (the action
// and the path) and does not name this package, which means nothing to a user.
func assertUserError(t *testing.T, err error, prefix string) {
	t.Helper()
	if msg := err.Error(); !strings.HasPrefix(msg, prefix) || strings.Contains(msg, "jsonfile") {
		t.Errorf("err = %q, want it to start with %q and not name the package", msg, prefix)
	}
}

func TestLockErrorNamesActionAndPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "no-such-dir", "list.json")
	_, err := jsonfile.New(jsonfile.Config[doc]{Path: path}).Update(context.Background(), setN(1))
	if err == nil {
		t.Fatal("Update in a missing directory returned nil error")
	}
	assertUserError(t, err, "lock "+path+".lock: ")
}

func TestSentinelsDoNotNameThePackage(t *testing.T) {
	for _, err := range []error{jsonfile.ErrBusy, jsonfile.ErrInvalid, jsonfile.ErrInvalidValue} {
		if strings.Contains(err.Error(), "jsonfile") {
			t.Errorf("sentinel text %q names the package", err)
		}
	}
}
