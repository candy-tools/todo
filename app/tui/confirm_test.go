package tui

import (
	"strings"
	"testing"

	"github.com/candy-tools/todo/internal/todo"
)

func TestDeletionQuestion(t *testing.T) {
	leaf := todo.NewTask("solo", "", false)
	if got := deletionQuestion(leaf); strings.Contains(got, "nested") {
		t.Errorf("a leaf task should not mention nested items: %q", got)
	}
	parent := todo.NewTask("p", "", false)
	parent.AppendChild(todo.NewTask("c1", "", false))
	parent.AppendChild(todo.NewTask("c2", "", false))
	if got := deletionQuestion(parent); !strings.Contains(got, "2 nested") {
		t.Errorf("parent question should count 2 nested items: %q", got)
	}
	cat := &todo.Item{Kind: todo.Category, Title: "Cat"}
	if got := deletionQuestion(cat); !strings.Contains(got, "category") {
		t.Errorf("category question should say 'category': %q", got)
	}
}

func TestConfirmModalRenders(t *testing.T) {
	out := confirmModal(`Delete task "x"?`, false, 80)
	for _, want := range []string{"Confirm delete", "Delete", "Cancel", `Delete task "x"?`} {
		if !strings.Contains(out, want) {
			t.Errorf("confirm modal missing %q", want)
		}
	}
}
