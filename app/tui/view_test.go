package tui

import (
	"testing"

	"github.com/candy-tools/todo/internal/todo"
)

func TestSetActiveStatesKeepsOtherColumns(t *testing.T) {
	v := newViewState()
	v.states[todo.Deferred] = true // a Backlog filter
	v.screen, v.sprint = screenSprint, "s000001"
	v.setActiveStates(map[todo.Status]bool{todo.Ready: true})
	if !v.states[todo.Deferred] || !v.states[todo.Ready] {
		t.Fatalf("states = %v, want Deferred kept and Ready added", v.states)
	}
	if got := v.activeStates(); len(got) != 1 || got[0] != todo.Ready {
		t.Errorf("activeStates = %v, want [ready]", got)
	}
	v.setActiveStates(nil)
	if v.states[todo.Ready] || !v.states[todo.Deferred] {
		t.Errorf("clearing the board's must keep Backlog's filter, states = %v", v.states)
	}
}

func TestSetActiveStatesUnified(t *testing.T) {
	v := newViewState()
	v.states[todo.Deferred] = true
	v.all = true
	v.setActiveStates(map[todo.Status]bool{todo.Ready: true})
	if !v.allStates[todo.Ready] || v.states[todo.Ready] || !v.states[todo.Deferred] {
		t.Errorf("unified filter must live apart: states %v allStates %v", v.states, v.allStates)
	}
	if got := v.activeStates(); len(got) != 1 || got[0] != todo.Ready {
		t.Errorf("activeStates = %v, want [ready]", got)
	}
}
