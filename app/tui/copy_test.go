package tui

import (
	"errors"
	"strings"
	"testing"

	"github.com/candy-tools/todo/internal/todo"
	"github.com/atotto/clipboard"
)

// fakeClip records what the copy path wrote, standing in for the real system
// clipboard so the tests never touch it.
type fakeClip struct {
	calls int
	last  string
}

// stubClipboard swaps in a capturing clipboard (marked available) for the test
// and returns the recorder; it restores the real writeClipboard afterwards.
func stubClipboard(t *testing.T, writeErr error) *fakeClip {
	t.Helper()
	origWrite := writeClipboard
	origUnsupported := clipboard.Unsupported
	fc := &fakeClip{}
	writeClipboard = func(s string) error {
		fc.calls++
		fc.last = s
		return writeErr
	}
	clipboard.Unsupported = false
	t.Cleanup(func() {
		writeClipboard = origWrite
		clipboard.Unsupported = origUnsupported
	})
	return fc
}

func TestItemCopyText(t *testing.T) {
	tests := []struct {
		name string
		item *todo.Item
		want string
	}{
		{"task with multi-line description", todo.NewTask("Ship it", "line one\nline two", false), "Ship it\n\nline one\nline two"},
		{"task without description", todo.NewTask("bare", "", false), "bare"},
		{"category has no description", &todo.Item{Kind: todo.Category, Title: "Work"}, "Work"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := itemCopyText(tc.item); got != tc.want {
				t.Errorf("itemCopyText = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestCopySelectionCopiesTitleAndDescription(t *testing.T) {
	fc := stubClipboard(t, nil)
	m, _ := newTestModel(t, "# Work\n\n- [ ] a\n  line one\n  line two\n")
	m = press(m, "down") // select task a
	m = press(m, "y")
	if fc.calls != 1 {
		t.Fatalf("clipboard writes = %d, want 1", fc.calls)
	}
	if want := "a\n\nline one\nline two"; fc.last != want {
		t.Errorf("copied %q, want %q", fc.last, want)
	}
	if m.status != "Copied to clipboard." {
		t.Errorf("status = %q, want the copy confirmation", m.status)
	}
}

func TestCopySelectionOnPlaceholderIsNoOp(t *testing.T) {
	fc := stubClipboard(t, nil)
	m, _ := newTestModel(t, "") // empty document: the cursor sits on the placeholder row
	if !m.tree.onPlaceholder() {
		t.Fatal("an empty document should start on the placeholder row")
	}
	m = press(m, "y")
	if fc.calls != 0 {
		t.Errorf("clipboard writes = %d, want 0 on the placeholder", fc.calls)
	}
	if m.status != "" {
		t.Errorf("status = %q, want empty on the placeholder", m.status)
	}
}

func TestCopySelectionReportsWriteError(t *testing.T) {
	fc := stubClipboard(t, errors.New("boom"))
	m, _ := newTestModel(t, "# Work\n\n- [ ] a\n")
	m = press(m, "down") // select task a
	m = press(m, "y")
	if fc.calls != 1 {
		t.Fatalf("clipboard writes = %d, want 1", fc.calls)
	}
	if !strings.HasPrefix(m.status, "Copy failed:") {
		t.Errorf("status = %q, want a copy-failed message", m.status)
	}
}

func TestCopySelectionUnsupported(t *testing.T) {
	fc := stubClipboard(t, nil)
	clipboard.Unsupported = true // simulate a box with no clipboard tool installed
	m, _ := newTestModel(t, "# Work\n\n- [ ] a\n")
	m = press(m, "down")
	m = press(m, "y")
	if fc.calls != 0 {
		t.Errorf("clipboard writes = %d, want 0 when unsupported", fc.calls)
	}
	if !strings.Contains(m.status, "clipboard tool") {
		t.Errorf("status = %q, want the missing-tool hint", m.status)
	}
}
