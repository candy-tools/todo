//go:build unix

package jsonfile

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type counter struct {
	N int `json:"n"`
}

// TestLockRetriesWhenLockFileReplaced removes the lock file right after A
// locked it, and lets B lock the new file created in its place before A goes
// on. A's lock no longer excludes anyone, so A must notice, let go and wait
// for B's: the two increments must not overlap and neither may be lost.
func TestLockRetriesWhenLockFileReplaced(t *testing.T) {
	path := filepath.Join(t.TempDir(), "list.json")
	cfg := Config[counter]{Path: path, LockTimeout: 5 * time.Second}
	a, b := New(cfg), New(cfg)

	var (
		inFn, overlaps atomic.Int32
		hookCalls      atomic.Int32
		aIn            = make(chan struct{})
		aOnce          sync.Once
		bIn            = make(chan struct{})
		bDone          = make(chan error, 1)
	)
	incr := func(v *counter) {
		if inFn.Add(1) > 1 {
			overlaps.Add(1)
		}
		v.N++
	}
	afterLock = func() {
		if hookCalls.Add(1) != 1 {
			return
		}
		// A holds the lock: replace the lock file and let B take the new one.
		if err := os.Remove(path + ".lock"); err != nil {
			t.Errorf("remove lock file: %v", err)
		}
		go func() {
			_, err := b.Update(context.Background(), func(v *counter, _ Snapshot) (bool, error) {
				incr(v)
				close(bIn)
				select { // stay in fn while A would run, if it does
				case <-aIn:
				case <-time.After(300 * time.Millisecond):
				}
				inFn.Add(-1)
				return true, nil
			})
			bDone <- err
		}()
		select {
		case <-bIn:
		case <-time.After(5 * time.Second):
			t.Error("B never got its lock")
		}
	}
	t.Cleanup(func() { afterLock = nil })

	if _, err := a.Update(context.Background(), func(v *counter, _ Snapshot) (bool, error) {
		aOnce.Do(func() { close(aIn) })
		incr(v)
		inFn.Add(-1)
		return true, nil
	}); err != nil {
		t.Fatalf("A: %v", err)
	}
	if err := <-bDone; err != nil {
		t.Fatalf("B: %v", err)
	}

	if n := overlaps.Load(); n != 0 {
		t.Errorf("A and B ran fn at the same time (%d overlaps)", n)
	}
	v, _, err := a.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if v.N != 2 {
		t.Errorf("N = %d, want 2", v.N)
	}
}

// TestLockGivesUpWhenLockFileKeepsBeingReplaced deletes the lock file every
// time Update locks it: Update must wait between tries rather than spin, and
// fail with ErrBusy, saying why, when the lock timeout runs out.
func TestLockGivesUpWhenLockFileKeepsBeingReplaced(t *testing.T) {
	path := filepath.Join(t.TempDir(), "list.json")
	var calls atomic.Int32
	afterLock = func() {
		calls.Add(1)
		_ = os.Remove(path + ".lock")
	}
	t.Cleanup(func() { afterLock = nil })

	const timeout = 200 * time.Millisecond
	s := New(Config[counter]{Path: path, LockTimeout: timeout})
	_, err := s.Update(context.Background(), func(*counter, Snapshot) (bool, error) {
		t.Error("fn ran without a lock")
		return false, nil
	})
	if !errors.Is(err, ErrBusy) {
		t.Fatalf("err = %v, want ErrBusy", err)
	}
	if !strings.Contains(err.Error(), "replaced") {
		t.Errorf("err = %q, want it to say the lock file keeps being replaced", err)
	}
	// One try per lockRetry at most, with room for a slow machine.
	if n, most := calls.Load(), int32(2*timeout/lockRetry+1); n > most {
		t.Errorf("locked %d times in %v, want at most %d (no busy loop)", n, timeout, most)
	}
}
