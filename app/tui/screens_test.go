package tui

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/candy-tools/todo/internal/todo"
	"github.com/charmbracelet/x/ansi"
)

// seedSprint is a sprint of a sprintJSON list; status is "open" or "done".
type seedSprint struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Goal   string `json:"goal,omitempty"`
	Status string `json:"status"`
}

// seedTask is a top-level task of a sprintJSON list, in category C; sprint is
// "" for a backlog task.
type seedTask struct {
	ID       string `json:"id"`
	Category string `json:"category"`
	Sprint   string `json:"sprint,omitempty"`
	Title    string `json:"title"`
	Status   string `json:"status"`
}

// sprintJSON builds a version 6 list file holding sprints, the tasks (all in
// one category C) and, when view is not nil, a view section.
func sprintJSON(t *testing.T, view map[string]any, sprints []seedSprint, tasks ...seedTask) string {
	t.Helper()
	for i := range tasks {
		tasks[i].Category = "c000000"
	}
	l := map[string]any{
		"version":    6,
		"sprints":    append([]seedSprint{}, sprints...),
		"categories": []map[string]string{{"id": "c000000", "title": "C"}},
		"tasks":      append([]seedTask{}, tasks...),
	}
	if view != nil {
		l["view"] = view
	}
	b, err := json.Marshal(l)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// threeSprints is a list with the sprints A (open), B (done) and C (open), a
// backlog task, and one task in each sprint.
func threeSprints(t *testing.T) (model, string) {
	t.Helper()
	return newJSONModel(t, sprintJSON(t, nil,
		[]seedSprint{
			{ID: "s00000a", Name: "A", Goal: "goal a", Status: "open"},
			{ID: "s00000b", Name: "B", Status: "done"},
			{ID: "s00000c", Name: "C", Goal: "goal c", Status: "open"},
		},
		seedTask{ID: "t000001", Title: "backlogTask", Status: "open"},
		seedTask{ID: "t000002", Title: "aNext", Status: "new", Sprint: "s00000a"},
		seedTask{ID: "t000003", Title: "aDone", Status: "done", Sprint: "s00000a"},
		seedTask{ID: "t000004", Title: "bDone", Status: "done", Sprint: "s00000b"},
		seedTask{ID: "t000005", Title: "cWorking", Status: "working", Sprint: "s00000c"},
	))
}

func TestOpensOnBacklogByDefault(t *testing.T) {
	m, _ := threeSprints(t)
	if m.view.screen != screenBacklog || m.view.all {
		t.Fatalf("opened on screen %v (unified %v), want the Backlog", m.view.screen, m.view.all)
	}
	if h := m.header(100); !strings.Contains(h, "Backlog · 1 tasks") {
		t.Errorf("header = %q, want the Backlog line", h)
	}
}

func TestBacklogHidesSprintTasks(t *testing.T) {
	m, _ := threeSprints(t)
	got := rowTitles(m.tree)
	if !contains(got, "backlogTask") {
		t.Errorf("Backlog rows %v miss backlogTask", got)
	}
	for _, hidden := range []string{"aNext", "aDone", "bDone", "cWorking"} {
		if contains(got, hidden) {
			t.Errorf("Backlog rows %v must not show the sprint task %q", got, hidden)
		}
	}
}

func TestBSwitchesScreens(t *testing.T) {
	m, _ := threeSprints(t)
	m = press(m, "s")
	if m.view.screen != screenSprints {
		t.Fatalf("s: screen = %v, want the Sprint list", m.view.screen)
	}
	if h := m.header(100); !strings.Contains(h, "Sprints · 2 open") {
		t.Errorf("header = %q, want the Sprint list line", h)
	}
	m = press(m, "b")
	if m.view.screen != screenBacklog {
		t.Fatalf("b: screen = %v, want the Backlog", m.view.screen)
	}
	m = press(m, "s", "down", "enter") // the second row: C
	if m.view.screen != screenSprint || m.view.sprint != "s00000c" {
		t.Fatalf("enter: screen = %v sprint = %q, want C's board", m.view.screen, m.view.sprint)
	}
	if m.view.column != todo.ColNext {
		t.Errorf("a board opens on Next, got %v", m.view.column)
	}
	m = press(m, "2")
	if got := rowTitles(m.tree); !equalStrings(got, []string{"C", "cWorking"}) {
		t.Errorf("C's In Progress rows = %v", got)
	}
	if h := m.header(100); !strings.Contains(h, "C · ") || !strings.Contains(h, "[In Progress 1]") || strings.Contains(h, "Backlog") {
		t.Errorf("header = %q, want C's name and its tab bar", h)
	}
}

func TestBoardHeaderMarksDoneSprint(t *testing.T) {
	m, _ := threeSprints(t)
	m.showSprint("s00000b")
	m = press(m, "3")
	if h := m.header(100); !strings.Contains(h, "B ✓ done   ") || strings.Contains(h, "B ✓ done · ") || !strings.Contains(h, "[Done 1]") {
		t.Errorf("header = %q, want B marked done", h)
	}
	if got := rowTitles(m.tree); !equalStrings(got, []string{"C", "bDone"}) {
		t.Errorf("B's Done rows = %v, want only B's task", got)
	}
}

func TestBoardHeaderShowsGoal(t *testing.T) {
	m, _ := threeSprints(t)
	m.showSprint("s00000c")
	if h := m.header(100); !strings.Contains(h, "C · goal c") || !strings.Contains(h, "[Next 0]") {
		t.Errorf("header = %q, want C's name and goal before the tabs", h)
	}
	long := strings.Repeat("a very long goal ", 20)
	m, _ = newJSONModel(t, sprintJSON(t, nil, []seedSprint{{ID: "s00000a", Name: "A", Goal: long, Status: "open"}},
		seedTask{ID: "t000001", Title: "w", Status: "working", Sprint: "s00000a"}))
	m.showSprint("s00000a")
	h := m.header(100)
	line := strings.Split(h, "\n")[1]
	if w := ansi.StringWidth(line); w > 100 {
		t.Errorf("the board line is %d wide, want at most 100: %q", w, line)
	}
	for _, want := range []string{"A · a very long goal", "…", "[Next 0]", "In Progress 1", "Done 0"} {
		if !strings.Contains(line, want) {
			t.Errorf("a long goal must be cut, not the tabs: line %q misses %q", line, want)
		}
	}
}

func TestQOnBoardReturnsToList(t *testing.T) {
	m, _ := threeSprints(t)
	m.showSprint("s00000a")
	nm, cmd := m.Update(keyMsg("q"))
	m = nm.(model)
	if quits(cmd) {
		t.Fatal("q on a board must not quit")
	}
	if m.view.screen != screenSprints {
		t.Errorf("q on a board: screen = %v, want the Sprint list", m.view.screen)
	}
}

func TestQOnListQuits(t *testing.T) {
	m, _ := threeSprints(t)
	m = press(m, "s")
	if _, cmd := m.Update(keyMsg("q")); !quits(cmd) {
		t.Error("q on the Sprint list must quit")
	}
	if _, cmd := press(m, "b").Update(keyMsg("q")); !quits(cmd) {
		t.Error("q on the Backlog must quit")
	}
}

func TestEscOnBoardReturnsToList(t *testing.T) {
	m, _ := threeSprints(t)
	m.showSprint("s00000a")
	// A search is cleared first, on the board.
	m.setFilter("zzz")
	nm, cmd := m.Update(keyMsg("esc"))
	if m = nm.(model); quits(cmd) || m.view.screen != screenSprint || m.view.filter != "" {
		t.Fatalf("esc with a search: screen %v filter %q quit %v, want the search cleared on the board", m.view.screen, m.view.filter, quits(cmd))
	}
	nm, cmd = m.Update(keyMsg("esc"))
	if m = nm.(model); quits(cmd) || m.view.screen != screenSprints {
		t.Fatalf("esc on a board: screen %v quit %v, want the Sprint list", m.view.screen, quits(cmd))
	}
	// The unified view over a board still quits.
	m.showSprint("s00000a")
	if _, cmd := press(m, "a").Update(keyMsg("esc")); !quits(cmd) {
		t.Error("esc in the unified view must quit")
	}
}

func TestKFromBacklogSkipsClosedLastViewed(t *testing.T) {
	m, _ := threeSprints(t)
	m.showSprint("s00000b") // B is done
	m = press(m, "b", "k")
	if m.view.screen != screenSprint || m.view.sprint != "s00000a" {
		t.Errorf("k from the Backlog after a closed sprint: screen %v sprint %q, want the first open one, A", m.view.screen, m.view.sprint)
	}
}

func TestEnterOnEmptySprintListHints(t *testing.T) {
	m, _ := newJSONModel(t, sprintJSON(t, nil, nil))
	m = press(m, "s", "enter")
	if m.mode != modeMain || m.view.screen != screenSprints {
		t.Errorf("enter on an empty Sprint list: mode %v screen %v, want to stay", m.mode, m.view.screen)
	}
	if want := "no open sprints — press s, then n to create one"; m.status != want {
		t.Errorf("footer = %q, want %q", m.status, want)
	}
}

func TestKRotatesOpenSprintsAndWraps(t *testing.T) {
	m, _ := threeSprints(t)
	m.showSprint("s00000a")
	for _, want := range []string{"s00000c", "s00000a", "s00000c"} {
		m = press(m, "k")
		if m.view.screen != screenSprint || m.view.sprint != want {
			t.Fatalf("k: screen = %v sprint = %q, want %q", m.view.screen, m.view.sprint, want)
		}
	}
	// From a done sprint's board, k goes to the next open one after it.
	m.showSprint("s00000b")
	if m = press(m, "k"); m.view.sprint != "s00000c" {
		t.Errorf("k from B = %q, want C", m.view.sprint)
	}
}

func TestKFromBacklogOpensLastViewed(t *testing.T) {
	m, _ := threeSprints(t)
	if m = press(m, "k"); m.view.sprint != "s00000a" {
		t.Fatalf("k with no sprint viewed yet = %q, want the first open one, A", m.view.sprint)
	}
	m.showSprint("s00000c")
	m = press(m, "b", "k")
	if m.view.screen != screenSprint || m.view.sprint != "s00000c" {
		t.Errorf("k from the Backlog: screen = %v sprint = %q, want C", m.view.screen, m.view.sprint)
	}
	m = press(m, "s", "k")
	if m.view.sprint != "s00000c" {
		t.Errorf("k from the Sprint list = %q, want C", m.view.sprint)
	}
}

func TestKWithoutOpenSprintsHints(t *testing.T) {
	m, _ := newJSONModel(t, sprintJSON(t, nil, []seedSprint{{ID: "s00000b", Name: "B", Status: "done"}}))
	m = press(m, "k")
	if m.view.screen != screenBacklog {
		t.Errorf("k with no open sprint must stay put, screen = %v", m.view.screen)
	}
	if want := "no open sprints — press s, then n to create one"; m.status != want {
		t.Errorf("footer = %q, want %q", m.status, want)
	}
}

func TestBoardRemembersItsTab(t *testing.T) {
	m, _ := threeSprints(t)
	m.showSprint("s00000a")
	m = press(m, "3")
	m.showSprint("s00000c")
	if m.view.column != todo.ColNext {
		t.Fatalf("C's board must open on Next, got %v", m.view.column)
	}
	m.showSprint("s00000a")
	if m.view.column != todo.ColDone {
		t.Errorf("back on A's board the tab = %v, want Done", m.view.column)
	}
}

func TestAToggleUnifiedOnTopOfScreen(t *testing.T) {
	m, _ := threeSprints(t)
	m.showSprint("s00000c")
	m = press(m, "a")
	if !m.view.all {
		t.Fatal("a should turn the unified view on")
	}
	if got := rowTitles(m.tree); !equalStrings(got, []string{"C", "backlogTask", "aNext", "aDone", "bDone", "cWorking"}) {
		t.Errorf("unified rows = %v, want every task", got)
	}
	m = press(m, "a")
	if m.view.all || m.view.screen != screenSprint || m.view.sprint != "s00000c" {
		t.Errorf("a again: unified %v screen %v sprint %q, want C's board", m.view.all, m.view.screen, m.view.sprint)
	}
	m = press(m, "a", "b")
	if m.view.all || m.view.screen != screenBacklog {
		t.Errorf("b in the unified view: unified %v screen %v, want the Backlog", m.view.all, m.view.screen)
	}
	m = press(m, "s", "a")
	if m.view.all {
		t.Error("a does nothing on the Sprint list")
	}
}

func TestTabsOnlyOnBoard(t *testing.T) {
	m, _ := threeSprints(t)
	before := rowTitles(m.tree)
	for _, key := range []string{"2", "tab", "shift+tab"} {
		got := press(m, key)
		if got.view.screen != screenBacklog || !equalStrings(rowTitles(got.tree), before) {
			t.Errorf("%q on the Backlog: screen %v rows %v, want nothing changed", key, got.view.screen, rowTitles(got.tree))
		}
	}
	m.showSprint("s00000a")
	steps := []struct {
		key  string
		want todo.Column
	}{
		{"2", todo.ColInProgress},
		{"3", todo.ColDone},
		{"tab", todo.ColNext},
		{"shift+tab", todo.ColDone},
		{"1", todo.ColNext},
		{"4", todo.ColNext}, // no fourth tab
	}
	for _, s := range steps {
		m = press(m, s.key)
		if m.view.column != s.want {
			t.Fatalf("after %q column = %v, want %v", s.key, m.view.column, s.want)
		}
	}
}

func TestSprintListRowsOrderAndCounts(t *testing.T) {
	m, _ := newJSONModel(t, sprintJSON(t, nil,
		[]seedSprint{
			{ID: "s00000a", Name: "Alpha", Goal: "first goal", Status: "open"},
			{ID: "s00000b", Name: "Beta", Status: "done"},
			{ID: "s00000c", Name: "Gamma", Goal: "third goal", Status: "open"},
			{ID: "s00000d", Name: "Delta", Status: "done"},
		},
		seedTask{ID: "t000001", Title: "a1", Status: "new", Sprint: "s00000a"},
		seedTask{ID: "t000002", Title: "a2", Status: "ready", Sprint: "s00000a"},
		seedTask{ID: "t000003", Title: "a3", Status: "review", Sprint: "s00000a"},
		seedTask{ID: "t000004", Title: "a4", Status: "done", Sprint: "s00000a"},
		seedTask{ID: "t000005", Title: "b1", Status: "done", Sprint: "s00000b"},
	))
	var names []string
	for _, s := range m.sprints.rows(m.doc) {
		names = append(names, s.Name)
	}
	if want := []string{"Alpha", "Gamma", "Delta", "Beta"}; !equalStrings(names, want) {
		t.Fatalf("rows = %v, want %v (open in creation order, then done newest first)", names, want)
	}
	out := m.sprints.view(m.doc, 100, 10)
	lines := strings.Split(out, "\n")
	if len(lines) != 4 {
		t.Fatalf("want 4 rows, got %d:\n%s", len(lines), out)
	}
	wants := [][]string{
		{"Alpha", "open", "first goal", "2 · 1 · 1"},
		{"Gamma", "open", "third goal", "0 · 0 · 0"},
		{"Delta", "done", "0 · 0 · 0"},
		{"Beta", "done", "0 · 0 · 1"},
	}
	for i, want := range wants {
		for _, w := range want {
			if !strings.Contains(lines[i], w) {
				t.Errorf("row %d = %q, misses %q", i, lines[i], w)
			}
		}
	}
	m = press(m, "s", "down", "down")
	if sel := m.sprints.selected(m.doc); sel == nil || sel.Name != "Delta" {
		t.Errorf("selected = %v, want Delta", sel)
	}
	if out := m.View(); !strings.Contains(out, "Gamma") || strings.Contains(out, "a1") {
		t.Errorf("the Sprint list screen shows the sprints, not the tasks:\n%s", out)
	}
}

func TestSaveViewStoresScreen(t *testing.T) {
	m, path := threeSprints(t)
	m.showSprint("s00000c")
	if got, want := storedView(t, path), (todo.View{Screen: todo.ScreenSprint, Sprint: "s00000c"}); got != want {
		t.Errorf("stored view = %+v, want %+v", got, want)
	}
	m = press(m, "s")
	if got, want := storedView(t, path), (todo.View{Screen: todo.ScreenSprints}); got != want {
		t.Errorf("stored view = %+v, want %+v", got, want)
	}
	press(m, "b")
	if got := storedView(t, path); got != (todo.View{}) {
		t.Errorf("stored view = %+v, want the default", got)
	}
}

func TestOpensOnStoredBoard(t *testing.T) {
	m, _ := newJSONModel(t, sprintJSON(t, map[string]any{"screen": "sprint", "sprint": "s00000a", "unified": true},
		[]seedSprint{{ID: "s00000a", Name: "A", Status: "open"}},
		seedTask{ID: "t000001", Title: "aNext", Status: "new", Sprint: "s00000a"}))
	if m.view.screen != screenSprint || m.view.sprint != "s00000a" || !m.view.all {
		t.Fatalf("opened on screen %v sprint %q unified %v, want A's board under the unified view", m.view.screen, m.view.sprint, m.view.all)
	}
	if m = press(m, "a"); m.view.all || !hasRow(m, "aNext") {
		t.Errorf("a should return to A's board: unified %v rows %v", m.view.all, rowTitles(m.tree))
	}
}

func TestApplyViewMissingSprint(t *testing.T) {
	m, _ := newJSONModel(t, sprintJSON(t, map[string]any{"screen": "sprint", "sprint": "s0000zz"},
		[]seedSprint{{ID: "s00000a", Name: "A", Status: "open"}}))
	if m.view.screen != screenSprints {
		t.Errorf("a saved view naming a missing sprint opened screen %v, want the Sprint list", m.view.screen)
	}
}

func TestReloadSprintGoneFallsBackToList(t *testing.T) {
	m, path := threeSprints(t)
	m.showSprint("s00000c")
	if _, err := openSecond(t, path).DeleteSprint(context.Background(), "s00000c"); err != nil {
		t.Fatal(err)
	}
	m = pollOnce(t, m)
	if m.view.screen != screenSprints {
		t.Errorf("after the sprint was deleted the screen = %v, want the Sprint list", m.view.screen)
	}
	if m.err != nil {
		t.Errorf("err = %v, want none", m.err)
	}
}

func TestWDoesNotMove(t *testing.T) {
	m, _ := threeSprints(t)
	m = press(m, "down")
	cursor := m.tree.cursor
	if m = press(m, "w"); m.tree.cursor != cursor {
		t.Errorf("w moved the cursor %d -> %d; only ↑ moves up", cursor, m.tree.cursor)
	}
}

func TestSprintListIgnoresTreeKeys(t *testing.T) {
	m, _ := threeSprints(t)
	m = press(m, "s")
	for _, key := range []string{"v", "x", "m", "t", "p", "/", "f"} {
		got := press(m, key)
		if got.mode != modeMain || got.split || got.view.screen != screenSprints || got.tree.grabbed {
			t.Errorf("%q on the Sprint list must do nothing: mode %v split %v screen %v", key, got.mode, got.split, got.view.screen)
		}
	}
}
