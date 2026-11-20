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
	if h := m.header(100); !strings.Contains(h, "[b Backlog]") || !strings.Contains(h, "Backlog › C · 1 tasks waiting") {
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
	if m.view.screen != screenSprint || m.view.sprint != "s00000a" {
		t.Fatalf("s: screen = %v sprint = %q, want the Sprints workspace on A", m.view.screen, m.view.sprint)
	}
	if h := m.header(100); !strings.Contains(h, "[s Sprints]") || !strings.Contains(h, "Sprints › A › Next") {
		t.Errorf("header = %q, want A's board line", h)
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
	if h := m.header(100); !strings.Contains(h, "Sprints › C › In Progress") || strings.Contains(h, "[b Backlog]") {
		t.Errorf("header = %q, want C's crumb", h)
	}
	if v := m.View(); !strings.Contains(v, "[In Progress 1]") {
		t.Errorf("board head misses C's tab bar:\n%s", v)
	}
}

func TestBoardHeaderMarksDoneSprint(t *testing.T) {
	m, _ := threeSprints(t)
	openBoard(&m, "s00000b")
	m = press(m, "3")
	if h := m.header(100); !strings.Contains(h, "Sprints › B ✓ › Done") {
		t.Errorf("header = %q, want B marked done in the crumb", h)
	}
	if v := ansi.Strip(m.View()); !strings.Contains(v, "closed — c in the sidebar reopens it") || !strings.Contains(v, "[Done 1]") ||
		!strings.Contains(v, "B ✓ closed") {
		t.Errorf("board head = %s, want B marked closed", v)
	}
	if got := rowTitles(m.tree); !equalStrings(got, []string{"C", "bDone"}) {
		t.Errorf("B's Done rows = %v, want only B's task", got)
	}
}

func TestBoardHeadShowsGoal(t *testing.T) {
	m, _ := threeSprints(t)
	m.showSprint("s00000c")
	if v := ansi.Strip(m.View()); !strings.Contains(v, "Goal  goal c") || !strings.Contains(v, "[Next 0]") {
		t.Errorf("view = %s, want C's goal and its tab bar in the board head", v)
	}
	long := strings.Repeat("a very long goal ", 20)
	m, _ = newJSONModel(t, sprintJSON(t, nil, []seedSprint{{ID: "s00000a", Name: "A", Goal: long, Status: "open"}},
		seedTask{ID: "t000001", Title: "w", Status: "working", Sprint: "s00000a"}))
	m.showSprint("s00000a")
	lines := strings.Split(ansi.Strip(m.View()), "\n")
	for _, l := range lines {
		if w := ansi.StringWidth(l); w > 100 {
			t.Errorf("a line is %d wide, want at most 100: %q", w, l)
		}
	}
	var goal, tabs string
	for _, l := range lines {
		if strings.Contains(l, "Goal  a very long goal") {
			goal = l
		}
		if strings.Contains(l, "[Next 0]") {
			tabs = l
		}
	}
	if !strings.Contains(goal, "…") {
		t.Errorf("a long goal must be cut with an ellipsis: %q", goal)
	}
	if !strings.Contains(tabs, "In Progress 1") || !strings.Contains(tabs, "Done 0") {
		t.Errorf("the tab bar must stay whole: %q", tabs)
	}
}

func TestQOnBoardQuits(t *testing.T) {
	m, _ := threeSprints(t)
	openBoard(&m, "s00000a")
	if _, cmd := m.Update(keyMsg("q")); !quits(cmd) {
		t.Fatal("q on a board must quit")
	}
}

func TestQOnListQuits(t *testing.T) {
	m, _ := threeSprints(t)
	m = press(m, "s")
	if _, cmd := m.Update(keyMsg("q")); !quits(cmd) {
		t.Error("q in the sprint sidebar must quit")
	}
	if _, cmd := press(m, "b").Update(keyMsg("q")); !quits(cmd) {
		t.Error("q on the Backlog must quit")
	}
}

func TestEscOnBoardFocusesList(t *testing.T) {
	m, _ := threeSprints(t)
	openBoard(&m, "s00000a")
	// A search is cleared first, on the board.
	m.setFilter("zzz")
	nm, cmd := m.Update(keyMsg("esc"))
	if m = nm.(model); quits(cmd) || m.view.screen != screenSprint || m.view.filter != "" {
		t.Fatalf("esc with a search: screen %v filter %q quit %v, want the search cleared on the board", m.view.screen, m.view.filter, quits(cmd))
	}
	nm, cmd = m.Update(keyMsg("esc"))
	if m = nm.(model); quits(cmd) || m.view.screen != screenSprint || m.view.sprint != "s00000a" || !m.listFocus {
		t.Fatalf("esc on a board: screen %v sprint %q listFocus %v quit %v, want the sprint list focused", m.view.screen, m.view.sprint, m.listFocus, quits(cmd))
	}
	// The unified view over a board still quits.
	m.showSprint("s00000a")
	if _, cmd := press(m, "u").Update(keyMsg("esc")); !quits(cmd) {
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

func TestEnterOnEmptySprintListDoesNothing(t *testing.T) {
	m, _ := newJSONModel(t, sprintJSON(t, nil, nil))
	m = press(m, "s", "enter")
	if m.mode != modeMain || !m.onList() {
		t.Errorf("enter on an empty sprint sidebar: mode %v listFocus %v, want to stay", m.mode, m.listFocus)
	}
	if m.status != "" {
		t.Errorf("footer = %q, want no message", m.status)
	}
	if m = press(m, "tab"); m.status != "" || !m.onList() {
		t.Errorf("tab on an empty sprint sidebar: status %q listFocus %v, want nothing", m.status, m.listFocus)
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
	if m.view.sprint != "s00000a" {
		t.Errorf("k in the Sprints workspace on C = %q, want the next open one, A", m.view.sprint)
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
	openBoard(&m, "s00000a")
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

func TestUToggleUnifiedOnTopOfScreen(t *testing.T) {
	m, _ := threeSprints(t)
	m.showSprint("s00000c")
	m = press(m, "u")
	if !m.view.all {
		t.Fatal("u should turn the unified view on")
	}
	if got := rowTitles(m.tree); !equalStrings(got, []string{"C", "backlogTask", "aNext", "aDone", "bDone", "cWorking"}) {
		t.Errorf("unified rows = %v, want every task", got)
	}
	m = press(m, "u")
	if m.view.all || m.view.screen != screenSprint || m.view.sprint != "s00000c" {
		t.Errorf("u again: unified %v screen %v sprint %q, want C's board", m.view.all, m.view.screen, m.view.sprint)
	}
	m = press(m, "u", "b")
	if m.view.all || m.view.screen != screenBacklog {
		t.Errorf("b in the unified view: unified %v screen %v, want the Backlog", m.view.all, m.view.screen)
	}
	m = press(m, "s", "u")
	if !m.view.all || m.listFocus {
		t.Errorf("u in the sprint sidebar: unified %v listFocus %v, want the unified view with the tree focused", m.view.all, m.listFocus)
	}
}

func TestTabsOnlyOnBoard(t *testing.T) {
	m, _ := threeSprints(t)
	before := rowTitles(m.tree)
	for _, key := range []string{"2", "[", "]"} {
		got := press(m, key)
		if got.view.screen != screenBacklog || !equalStrings(rowTitles(got.tree), before) {
			t.Errorf("%q on the Backlog: screen %v rows %v, want nothing changed", key, got.view.screen, rowTitles(got.tree))
		}
	}
	openBoard(&m, "s00000a")
	steps := []struct {
		key  string
		want todo.Column
	}{
		{"2", todo.ColInProgress},
		{"3", todo.ColDone},
		{"]", todo.ColNext},
		{"[", todo.ColDone},
		{"tab", todo.ColDone}, // tab no longer switches columns
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
	keys := func(m model) (k []string) {
		for _, r := range m.sprints.rows(m.doc) {
			if r.group {
				k = append(k, "Closed")
			} else {
				k = append(k, r.sprint.Name)
			}
		}
		return k
	}
	if want := []string{"Alpha", "Gamma", "Closed"}; !equalStrings(keys(m), want) {
		t.Fatalf("rows = %v, want %v (open in creation order, then the folded Closed group)", keys(m), want)
	}
	m = press(m, "s", "down", "down", "right") // on the Closed row, unfolded
	if want := []string{"Alpha", "Gamma", "Closed", "Delta", "Beta"}; !equalStrings(keys(m), want) {
		t.Fatalf("unfolded rows = %v, want %v (done newest first)", keys(m), want)
	}
	lines := strings.Split(ansi.Strip(m.sprints.view(m.doc, 100, 20)), "\n")
	if len(lines) != 10 {
		t.Fatalf("want 5 two-line rows, got %d lines", len(lines))
	}
	wants := [][]string{
		{"● Alpha", "2·1·1"}, {"first goal"},
		{"● Gamma", "0·0·0"}, {"third goal"},
		{"▾", "Closed", "2"}, {},
		{"✓ Delta", "0·0·0"}, {},
		{"✓ Beta", "0·0·1"}, {},
	}
	for i, want := range wants {
		for _, w := range want {
			if !strings.Contains(lines[i], w) {
				t.Errorf("line %d = %q, misses %q", i, lines[i], w)
			}
		}
	}
	m = press(m, "down")
	if sel := m.sprints.selected(m.doc); sel == nil || sel.Name != "Delta" {
		t.Errorf("selected = %v, want Delta", sel)
	}
	if out := m.View(); !strings.Contains(out, "Gamma") || strings.Contains(out, "a1") {
		t.Errorf("the sidebar lists the sprints, and Delta's empty board none of Alpha's tasks:\n%s", out)
	}
}

func TestSaveViewStoresScreen(t *testing.T) {
	m, path := threeSprints(t)
	m.showSprint("s00000c")
	if got, want := storedView(t, path), (todo.View{Screen: todo.ScreenSprint, Sprint: "s00000c"}); got != want {
		t.Errorf("stored view = %+v, want %+v", got, want)
	}
	m = press(m, "b", "s") // the Sprints workspace opens on the last viewed sprint
	if got, want := storedView(t, path), (todo.View{Screen: todo.ScreenSprint, Sprint: "s00000c"}); got != want {
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
	if m = press(m, "u"); m.view.all || !hasRow(m, "aNext") {
		t.Errorf("u should return to A's board: unified %v rows %v", m.view.all, rowTitles(m.tree))
	}
}

func TestApplyViewMissingSprint(t *testing.T) {
	m, _ := newJSONModel(t, sprintJSON(t, map[string]any{"screen": "sprint", "sprint": "s0000zz"},
		[]seedSprint{{ID: "s00000a", Name: "A", Status: "open"}}))
	if m.view.screen != screenSprint || m.view.sprint != "s00000a" || !m.listFocus {
		t.Errorf("a saved view naming a missing sprint opened screen %v sprint %q, want the Sprints workspace on A", m.view.screen, m.view.sprint)
	}
}

func TestReloadSprintGoneFallsBackToList(t *testing.T) {
	m, path := threeSprints(t)
	m.showSprint("s00000c")
	if _, err := openSecond(t, path).DeleteSprint(context.Background(), "s00000c"); err != nil {
		t.Fatal(err)
	}
	m = pollOnce(t, m)
	if m.view.screen != screenSprint || m.view.sprint != "s00000a" {
		t.Errorf("after the sprint was deleted the screen = %v sprint = %q, want A's board", m.view.screen, m.view.sprint)
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
	for _, key := range []string{"v", "m", "t", "a", "D"} {
		got := press(m, key)
		if got.mode != modeMain || got.split || !got.onList() || got.tree.grabbed {
			t.Errorf("%q in the sprint sidebar must do nothing: mode %v split %v listFocus %v", key, got.mode, got.split, got.listFocus)
		}
	}
}
