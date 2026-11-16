package jsonfile_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
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

func setN(n int) func(*doc) (bool, error) {
	return func(v *doc) (bool, error) {
		v.N = n
		return true, nil
	}
}

func mustUpdate(t *testing.T, s *jsonfile.Store[doc], fn func(*doc) (bool, error)) jsonfile.Result[doc] {
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
	if fp != jsonfile.Missing {
		t.Errorf("fingerprint = %x, want Missing", fp)
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
	if res.Read != jsonfile.Missing {
		t.Errorf("Read = %x, want Missing", res.Read)
	}
	if res.Wrote == jsonfile.Missing {
		t.Error("Wrote = Missing, want a fingerprint")
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

	res := mustUpdate(t, s, func(*doc) (bool, error) { return false, nil })
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
	_, err := s.Update(context.Background(), func(v *doc) (bool, error) {
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
			_, err := s.Update(context.Background(), func(*doc) (bool, error) {
				called = true
				return true, nil
			})
			if !errors.Is(err, jsonfile.ErrInvalid) {
				t.Fatalf("err = %v, want ErrInvalid", err)
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

func TestUpdateReportsOutsideWrite(t *testing.T) {
	path := tempPath(t)
	a := jsonfile.New(jsonfile.Config[doc]{Path: path})
	b := jsonfile.New(jsonfile.Config[doc]{Path: path})
	f1 := mustUpdate(t, a, setN(1)).Wrote
	mustUpdate(t, b, setN(2))

	var seen int
	res := mustUpdate(t, a, func(v *doc) (bool, error) {
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
				if _, err := s.Update(context.Background(), func(v *doc) (bool, error) {
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
	mustUpdate(t, s, func(v *doc) (bool, error) {
		v.Tags = []string{"<a&b>"}
		return true, nil
	})
	if b := readBytes(t, path); !bytes.Contains(b, []byte(`"<a&b>"`)) {
		t.Errorf("file = %q, want the literal <a&b>", b)
	}
}

func TestNoTempFilesLeft(t *testing.T) {
	path := tempPath(t)
	s := jsonfile.New(jsonfile.Config[doc]{Path: path})
	mustUpdate(t, s, setN(1))
	if _, err := s.Update(context.Background(), func(v *doc) (bool, error) {
		v.N = 2
		return true, errors.New("fail")
	}); err == nil {
		t.Fatal("failing Update returned nil error")
	}

	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)
	want := []string{"list.json", "list.json.lock"}
	if len(names) != len(want) || names[0] != want[0] || names[1] != want[1] {
		t.Errorf("dir = %v, want %v", names, want)
	}
}
