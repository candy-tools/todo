package jsonfile_test

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/gofrs/flock"

	"github.com/candy-tools/todo/libs/jsonfile"
)

// symlink makes link point at target, skipping the test where the platform
// (Windows without the privilege) cannot create symlinks.
func symlink(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("cannot create symlinks here: %v", err)
	}
}

// assertSymlink fails unless path is still a symlink.
func assertSymlink(t *testing.T, path string) {
	t.Helper()
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("lstat %s: %v", path, err)
	}
	if info.Mode()&fs.ModeSymlink == 0 {
		t.Errorf("%s is no longer a symlink (mode %v)", path, info.Mode())
	}
}

// assertNotExist fails when path exists.
func assertNotExist(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Lstat(path); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("%s exists (lstat err %v), want none", path, err)
	}
}

func TestUpdateThroughSymlinkKeepsLink(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "real.json")
	link := filepath.Join(dir, "link.json")
	mustUpdate(t, jsonfile.New(jsonfile.Config[doc]{Path: real}), setN(1))
	symlink(t, "real.json", link) // relative, as a dotfiles link often is

	s := jsonfile.New(jsonfile.Config[doc]{Path: link})
	res := mustUpdate(t, s, setN(2))
	if res.Value.N != 2 {
		t.Errorf("Value.N = %d, want 2", res.Value.N)
	}
	assertSymlink(t, link)
	v, fp, err := jsonfile.New(jsonfile.Config[doc]{Path: real}).Load()
	if err != nil {
		t.Fatalf("Load target: %v", err)
	}
	if v.N != 2 || fp != res.Wrote {
		t.Errorf("target N = %d (fp %x), want 2 (fp %x)", v.N, fp, res.Wrote)
	}
	if _, fp, err := s.Load(); err != nil || fp != res.Wrote {
		t.Errorf("Load through the link = %x, %v; want %x", fp, err, res.Wrote)
	}
	assertNotExist(t, link+".lock")
	if _, err := os.Stat(real + ".lock"); err != nil {
		t.Errorf("the lock must be taken on the target's lock file: %v", err)
	}
}

func TestUpdateThroughDanglingSymlinkCreatesTarget(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "real.json")
	link := filepath.Join(dir, "link.json")
	symlink(t, "real.json", link)

	mustUpdate(t, jsonfile.New(jsonfile.Config[doc]{Path: link}), setN(1))
	assertSymlink(t, link)
	if got := string(readBytes(t, real)); got != "{\n  \"n\": 1,\n  \"tags\": null\n}\n" {
		t.Errorf("target = %q", got)
	}
}

func TestSymlinkChainResolvesToTarget(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	real := filepath.Join(sub, "real.json")
	mid := filepath.Join(sub, "mid.json")
	link := filepath.Join(dir, "link.json")
	symlink(t, "real.json", mid)                       // relative to sub
	symlink(t, filepath.Join("sub", "mid.json"), link) // relative to dir

	mustUpdate(t, jsonfile.New(jsonfile.Config[doc]{Path: link}), setN(3))
	assertSymlink(t, link)
	assertSymlink(t, mid)
	v, _, err := jsonfile.New(jsonfile.Config[doc]{Path: real}).Load()
	if err != nil || v.N != 3 {
		t.Errorf("target N = %d, %v; want 3", v.N, err)
	}
}

// TestSymlinkDotDotFollowsRealDirectory reaches a link through a symlinked
// directory: its `..` must be taken from the directory the link really sits
// in (real/x/y), not cleaned away against the path it was reached by.
func TestSymlinkDotDotFollowsRealDirectory(t *testing.T) {
	dir := t.TempDir()
	deep := filepath.Join(dir, "real", "x", "y")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	symlink(t, deep, filepath.Join(dir, "dirlink"))
	symlink(t, filepath.Join("..", "list.json"), filepath.Join(deep, "l.json"))

	mustUpdate(t, jsonfile.New(jsonfile.Config[doc]{Path: filepath.Join(dir, "dirlink", "l.json")}), setN(4))
	assertSymlink(t, filepath.Join(deep, "l.json"))
	v, _, err := jsonfile.New(jsonfile.Config[doc]{Path: filepath.Join(dir, "real", "x", "list.json")}).Load()
	if err != nil || v.N != 4 {
		t.Errorf("real/x/list.json N = %d, %v; want 4", v.N, err)
	}
	assertNotExist(t, filepath.Join(dir, "list.json"))
}

func TestSymlinkUpdateWaitsForTargetLock(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "real.json")
	link := filepath.Join(dir, "link.json")
	mustUpdate(t, jsonfile.New(jsonfile.Config[doc]{Path: real}), setN(1))
	symlink(t, real, link)

	lock := flock.New(real + ".lock")
	if err := lock.Lock(); err != nil {
		t.Fatalf("lock: %v", err)
	}
	defer func() { _ = lock.Unlock() }()

	s := jsonfile.New(jsonfile.Config[doc]{Path: link, LockTimeout: 50 * time.Millisecond})
	if _, err := s.Update(context.Background(), setN(2)); !errors.Is(err, jsonfile.ErrBusy) {
		t.Fatalf("err = %v, want ErrBusy (the target's lock is held)", err)
	}
}

// TestTwoSymlinksShareOneLock runs concurrent increments through two links to
// one file: they serialize on the target's lock, so none is lost.
func TestTwoSymlinksShareOneLock(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "real.json")
	links := []string{filepath.Join(dir, "a.json"), filepath.Join(dir, "b.json")}
	mustUpdate(t, jsonfile.New(jsonfile.Config[doc]{Path: real}), setN(0))
	for _, l := range links {
		symlink(t, real, l)
	}

	const perLink, rounds = 4, 25
	var wg sync.WaitGroup
	errs := make(chan error, len(links)*perLink*rounds)
	for _, l := range links {
		for range perLink {
			wg.Add(1)
			go func() {
				defer wg.Done()
				s := jsonfile.New(jsonfile.Config[doc]{Path: l, LockTimeout: 30 * time.Second})
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
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("Update: %v", err)
	}
	for _, l := range links {
		assertSymlink(t, l)
	}
	v, _, err := jsonfile.New(jsonfile.Config[doc]{Path: real}).Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if want := len(links) * perLink * rounds; v.N != want {
		t.Errorf("N = %d, want %d", v.N, want)
	}
}

func TestSymlinkLoopError(t *testing.T) {
	dir := t.TempDir()
	a, b := filepath.Join(dir, "a.json"), filepath.Join(dir, "b.json")
	symlink(t, "b.json", a)
	symlink(t, "a.json", b)
	_, err := jsonfile.New(jsonfile.Config[doc]{Path: a}).Update(context.Background(), setN(1))
	if err == nil {
		t.Fatal("Update through a link loop returned nil error")
	}
	assertUserError(t, err, a+": ")
}

func TestLockPath(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "real.json")
	link := filepath.Join(dir, "link.json")
	if got, want := jsonfile.LockPath(real), real+".lock"; got != want {
		t.Errorf("LockPath(plain) = %q, want %q", got, want)
	}
	symlink(t, "real.json", link)

	// Through a link, LockPath is the lock Update takes: holding it makes
	// Update through the link busy.
	got := jsonfile.LockPath(link)
	if got == link+".lock" {
		t.Errorf("LockPath(link) = %q, want the target's lock file", got)
	}
	lock := flock.New(got)
	if err := lock.Lock(); err != nil {
		t.Fatalf("lock: %v", err)
	}
	defer func() { _ = lock.Unlock() }()
	s := jsonfile.New(jsonfile.Config[doc]{Path: link, LockTimeout: 50 * time.Millisecond})
	if _, err := s.Update(context.Background(), setN(1)); !errors.Is(err, jsonfile.ErrBusy) {
		t.Fatalf("err = %v, want ErrBusy (LockPath's lock is held)", err)
	}
	a, errA := os.Stat(got)
	b, errB := os.Stat(real + ".lock")
	if errA != nil || errB != nil || !os.SameFile(a, b) {
		t.Errorf("LockPath(link) = %q is not the target's lock file %q (%v, %v)", got, real+".lock", errA, errB)
	}
}
