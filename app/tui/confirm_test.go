package tui

import (
	"strings"
	"testing"

	"github.com/candy-tools/todo/internal/todo"
)

func TestDeletionQuestion(t *testing.T) {
	leaf := newTask("solo", "", todo.Open)
	if got := deletionQuestion(leaf); strings.Contains(got, "nested") {
		t.Errorf("a leaf task should not mention nested items: %q", got)
	}
	parent := newTask("p", "", todo.Open)
	parent.AppendChild(newTask("c1", "", todo.Open))
	parent.AppendChild(newTask("c2", "", todo.Open))
	if got := deletionQuestion(parent); !strings.Contains(got, "2 nested") {
		t.Errorf("parent question should count 2 nested items: %q", got)
	}
	cat := &todo.Item{Kind: todo.Category, Title: "Cat"}
	if got := deletionQuestion(cat); !strings.Contains(got, "category") {
		t.Errorf("category question should say 'category': %q", got)
	}
}

func TestConfirmModalRenders(t *testing.T) {
	out := confirmModal(`Delete task "x"?`, deleteLabels, false, 80)
	for _, want := range []string{"Confirm delete", "Delete", "Cancel", `Delete task "x"?`} {
		if !strings.Contains(out, want) {
			t.Errorf("confirm modal missing %q", want)
		}
	}
}
