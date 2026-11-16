package jsonfile_test

import (
	"os"
	"testing"
	"time"

	"github.com/candy-tools/todo/libs/jsonfile"
)

func TestChangedDetectsOutsideWrite(t *testing.T) {
	path := tempPath(t)
	a := jsonfile.New(jsonfile.Config[doc]{Path: path})
	b := jsonfile.New(jsonfile.Config[doc]{Path: path})
	mustUpdate(t, a, setN(1))

	_, f, err := a.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	changed, now, err := a.Changed(f)
	if err != nil {
		t.Fatalf("Changed: %v", err)
	}
	if changed || now != f {
		t.Errorf("Changed(f) = (%v, %x), want (false, %x)", changed, now, f)
	}

	wrote := mustUpdate(t, b, setN(2)).Wrote
	changed, now, err = a.Changed(f)
	if err != nil {
		t.Fatalf("Changed: %v", err)
	}
	if !changed {
		t.Error("Changed(f) = false after an outside write")
	}
	if now != wrote {
		t.Errorf("now = %x, want %x", now, wrote)
	}
}

func TestChangedIgnoresTouch(t *testing.T) {
	path := tempPath(t)
	s := jsonfile.New(jsonfile.Config[doc]{Path: path})
	f := mustUpdate(t, s, setN(1)).Wrote
	if changed, _, err := s.Changed(f); err != nil || changed {
		t.Fatalf("Changed(f) = (%v, %v), want (false, nil)", changed, err)
	}

	later := time.Now().Add(time.Hour)
	if err := os.Chtimes(path, later, later); err != nil {
		t.Fatal(err)
	}
	changed, now, err := s.Changed(f)
	if err != nil {
		t.Fatalf("Changed: %v", err)
	}
	if changed || now != f {
		t.Errorf("Changed(f) after touch = (%v, %x), want (false, %x)", changed, now, f)
	}
}

// TestChangedRacyStat rewrites the file in place with bytes of the same size
// and puts its mtime back, as a coarse mtime tick would leave it: the stat
// matches the cache, but the mtime is too recent to trust, so Changed must
// re-hash and see the new content.
func TestChangedRacyStat(t *testing.T) {
	path := tempPath(t)
	s := jsonfile.New(jsonfile.Config[doc]{Path: path})
	f := mustUpdate(t, s, setN(1)).Wrote
	tick := time.Now()
	if err := os.Chtimes(path, tick, tick); err != nil {
		t.Fatal(err)
	}
	if changed, _, err := s.Changed(f); err != nil || changed {
		t.Fatalf("Changed(f) = (%v, %v), want (false, nil)", changed, err)
	}

	// Same inode, same size, same mtime, new bytes.
	if err := os.WriteFile(path, []byte("{\n  \"n\": 2,\n  \"tags\": null\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, tick, tick); err != nil {
		t.Fatal(err)
	}
	changed, now, err := s.Changed(f)
	if err != nil {
		t.Fatalf("Changed: %v", err)
	}
	if !changed || now == f {
		t.Errorf("Changed(f) after a same-tick rewrite = (%v, %x), want (true, a new fingerprint)", changed, now)
	}
}

func TestChangedDeleted(t *testing.T) {
	path := tempPath(t)
	s := jsonfile.New(jsonfile.Config[doc]{Path: path})
	f := mustUpdate(t, s, setN(1)).Wrote
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}

	changed, now, err := s.Changed(f)
	if err != nil {
		t.Fatalf("Changed: %v", err)
	}
	if !changed || !now.IsMissing() {
		t.Errorf("Changed(f) = (%v, %x), want (true, missing)", changed, now)
	}

	changed, now, err = s.Changed(jsonfile.Fingerprint{})
	if err != nil {
		t.Fatalf("Changed: %v", err)
	}
	if changed || !now.IsMissing() {
		t.Errorf("Changed(missing) = (%v, %x), want (false, missing)", changed, now)
	}
}
