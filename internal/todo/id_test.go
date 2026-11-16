package todo

import "testing"

func TestNewIDShape(t *testing.T) {
	for i := 0; i < 1000; i++ {
		if id := newID(func(string) bool { return false }); !validID(id) {
			t.Fatalf("newID() = %q, not valid", id)
		}
	}
}

func TestNewIDAvoidsTaken(t *testing.T) {
	var rejected []string
	got := newID(func(id string) bool {
		if len(rejected) < 3 {
			rejected = append(rejected, id)
			return true
		}
		return false
	})
	if len(rejected) != 3 {
		t.Fatalf("rejected %d draws, want 3", len(rejected))
	}
	if !validID(got) {
		t.Fatalf("newID() = %q, not valid", got)
	}
	for _, r := range rejected {
		if r == got {
			t.Fatalf("returned rejected id %q", got)
		}
	}
}

func TestValidID(t *testing.T) {
	for _, bad := range []string{"", "abc", "ABCDEFG", "abcdefi", "abcdefgh"} {
		if validID(bad) {
			t.Errorf("validID(%q) = true, want false", bad)
		}
	}
	if !validID("k3f9qw2") {
		t.Error(`validID("k3f9qw2") = false, want true`)
	}
}
