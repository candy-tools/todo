package tui

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/candy-tools/todo/internal/todo"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

// deleteSprintOnDisk deletes sprint id through a second store on path, as
// another session would: its tasks are finished first, as only a done sprint
// may be deleted.
func deleteSprintOnDisk(t *testing.T, path, id string) {
	t.Helper()
	finishOnDisk(t, path, id)
	if _, err := openSecond(t, path).DeleteSprint(context.Background(), id); err != nil {
		t.Fatal(err)
	}
}

// finishOnDisk completes sprint id's tasks, and so the sprint, through a
// second store on path, as another session would.
func finishOnDisk(t *testing.T, path, id string) {
	t.Helper()
	ctx := context.Background()
	st := openSecond(t, path)
	doc, _, err := st.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range doc.SprintTasks(id) {
		if _, err := st.SetState(ctx, it.ID, todo.Done, todo.StateOptions{Cascade: true}); err != nil {
			t.Fatalf("complete %s: %v", it.ID, err)
		}
	}
}

// openBoard opens the Sprints workspace on sprint id with the board focused,
// as k then tab do.
func openBoard(m *model, id string) {
	m.showSprint(id)
	m.listFocus = false
}

// inBoardPane reports whether a line of m's screen holds want to the right of
// the Sprints sidebar, i.e. in the board pane.
func inBoardPane(m model, want string) bool {
	w := m.width
	if w == 0 {
		w = 80 // mainView's fallback when the terminal size is unknown
	}
	sw := sidebarWidth(w)
	for _, l := range strings.Split(ansi.Strip(m.View()), "\n") {
		if i := strings.Index(l, want); i >= 0 && ansi.StringWidth(l[:i]) >= sw {
			return true
		}
	}
	return false
}

func TestSOpensWorkspaceWithListFocusAndBoard(t *testing.T) {
	m, _ := threeSprints(t)
	m = press(m, "s")
	if m.view.screen != screenSprint || m.view.sprint != "s00000a" || !m.listFocus {
		t.Fatalf("s: screen %v sprint %q listFocus %v, want A's board with the list focused", m.view.screen, m.view.sprint, m.listFocus)
	}
	if got := rowTitles(m.tree); !slices.Contains(got, "aNext") {
		t.Errorf("board rows = %v, want A's Next task", got)
	}
}

func TestListCursorDrivesBoard(t *testing.T) {
	m, _ := threeSprints(t)
	m = press(m, "s", "down")
	if m.view.sprint != "s00000c" {
		t.Fatalf("down in the list shows %q, want C (open sprints first)", m.view.sprint)
	}
	m = press(m, "tab")
	if m.listFocus {
		t.Fatal("tab should focus the board")
	}
	m = press(m, "down")
	if m.view.sprint != "s00000c" {
		t.Fatal("down on the board must not change the sprint")
	}
	m = press(m, "esc")
	if !m.listFocus {
		t.Fatal("esc on the board should focus the list")
	}
}

func TestQQuitsFromBoard(t *testing.T) {
	m, _ := threeSprints(t)
	m = press(m, "s", "tab")
	if !quitsConfirmed(t, m, "q") {
		t.Fatal("q on the board should quit")
	}
}

func TestNewSprintFromListIsShown(t *testing.T) {
	m, _ := threeSprints(t)
	m = press(m, "s", "n")
	m = typeText(m, "D")
	m = press(m, "enter")
	if s := m.sprints.selected(m.doc); s == nil || s.Name != "D" || m.view.sprint != s.ID {
		t.Fatalf("after adding D: selected %v, board %q", s, m.view.sprint)
	}
}

func TestNoSprintsShowsEmptyBoard(t *testing.T) {
	m, _ := newTestModel(t, "# Work\n- [ ] a\n")
	m = press(m, "s")
	if m.view.screen != screenSprint || m.view.sprint != "" || len(m.tree.rows) != 0 {
		t.Fatalf("no sprints: screen %v sprint %q rows %d, want an empty workspace", m.view.screen, m.view.sprint, len(m.tree.rows))
	}
	if !inBoardPane(m, "no sprints yet — n creates one") {
		t.Errorf("the board pane should say there are no sprints:\n%s", ansi.Strip(m.View()))
	}
	// Back from the unified view the sidebar keeps the keys: there is no board.
	if m = press(m, "u", "u"); !m.listFocus {
		t.Error("after u u with no sprints the sidebar should have the keys")
	}
}

// On a tiny terminal the two-pane screens never draw wider than it.
func TestTinyTerminalFitsTwoPanes(t *testing.T) {
	for _, w := range []int{20, 30} {
		m, _ := threeSprints(t)
		m = send(m, tea.WindowSizeMsg{Width: w, Height: 20})
		for _, keys := range [][]string{{"s"}, {"s", "down", "down"}, {"s", "tab", "L"}, {"b", "v"}} {
			for i, line := range strings.Split(press(m, keys...).View(), "\n") {
				if lw := ansi.StringWidth(line); lw > w {
					t.Errorf("width %d after %v: line %d is %d wide: %q", w, keys, i, lw, ansi.Strip(line))
				}
			}
		}
	}
}

// With every sprint done the workspace opens on the newest, the sidebar's
// first sprint, and shows its board.
func TestOnlyDoneSprintsShowNewestDone(t *testing.T) {
	m, path := newJSONModel(t, sprintJSON(t, nil,
		[]seedSprint{{ID: "s00000a", Name: "A"}, {ID: "s00000b", Name: "B"}},
		seedTask{ID: "t000002", Title: "aDone", Status: "done", Sprint: "s00000a"},
		seedTask{ID: "t000001", Title: "bDone", Status: "done", Sprint: "s00000b"}))
	m = press(m, "s")
	if s := m.sprints.selected(m.doc); m.view.screen != screenSprint || m.view.sprint != "s00000b" || s == nil || s.ID != "s00000b" {
		t.Fatalf("only done: screen %v sprint %q selected %v, want B, the newest, on both", m.view.screen, m.view.sprint, s)
	}
	if !inBoardPane(m, "✓ Done") || !hasRow(press(m, "tab", "3"), "bDone") {
		t.Errorf("want B's board:\n%s", ansi.Strip(m.View()))
	}
	if v := storedView(t, path); v.Screen != todo.ScreenSprint || v.Sprint != "s00000b" {
		t.Errorf("stored view %+v, want B's board", v)
	}
}

func TestBoardIgnoresSplit(t *testing.T) {
	m, _ := threeSprints(t)
	m = press(m, "v", "s", "tab")
	if m.splitShown() || m.focused() != &m.tree {
		t.Fatal("the board never shows the category sidebar")
	}
	if !hasRow(m, "aNext") || m.tree.pane != paneMixed {
		t.Fatalf("the board with a stored split: pane %v rows %v, want the whole board", m.tree.pane, rowTitles(m.tree))
	}
	m = press(m, "v")
	if !m.split || !strings.Contains(m.footer(200), "v splits the Backlog") {
		t.Errorf("v on the board: split %v footer %q, want the hint and the split kept", m.split, m.footer(200))
	}
	m = press(m, "b")
	if !m.splitShown() || m.tree.pane != paneTasks {
		t.Error("the Backlog keeps its split")
	}
}

func TestShownSprintDeletedElsewhereFallsBack(t *testing.T) {
	m, path := threeSprints(t)
	m = press(m, "s", "down") // C
	deleteSprintOnDisk(t, path, "s00000c")
	m = pollOnce(t, m)
	if m.view.screen != screenSprint || m.view.sprint != "s00000a" {
		t.Fatalf("after C vanished: screen %v sprint %q, want A", m.view.screen, m.view.sprint)
	}
	if s := m.sprints.selected(m.doc); s == nil || s.ID != "s00000a" || !m.listFocus {
		t.Errorf("after C vanished: selected %v listFocus %v, want A selected in the focused list", s, m.listFocus)
	}
}

func TestShownSprintDeletedElsewhereKeepsUnifiedView(t *testing.T) {
	m, path := threeSprints(t)
	m = press(m, "s", "down", "u") // C, then the unified view over it
	if !m.view.all {
		t.Fatal("u should turn the unified view on")
	}
	stored := storedView(t, path)
	deleteSprintOnDisk(t, path, "s00000c")
	m = pollOnce(t, m)
	if !m.view.all {
		t.Error("the unified view was turned off when the hidden sprint was deleted")
	}
	if v := storedView(t, path); v != stored {
		t.Errorf("stored view %+v, want %+v unchanged", v, stored)
	}
}

func TestOldSavedSprintsScreenOpensWorkspace(t *testing.T) {
	m, _ := newJSONModel(t, sprintJSON(t, map[string]any{"screen": "sprints", "split": true},
		[]seedSprint{{ID: "s00000a", Name: "A"}},
		seedTask{ID: "t000001", Title: "x", Status: "new", Sprint: "s00000a"}))
	if m.view.screen != screenSprint || m.view.sprint != "s00000a" || m.splitShown() {
		t.Fatalf("screen %v sprint %q splitShown %v", m.view.screen, m.view.sprint, m.splitShown())
	}
}

func TestStoredViewNamesShownSprint(t *testing.T) {
	m, path := threeSprints(t)
	if m = press(m, "s", "down", "tab"); m.listFocus {
		t.Fatal("tab should focus the board")
	}
	if v := storedView(t, path); v.Screen != todo.ScreenSprint || v.Sprint != "s00000c" {
		t.Fatalf("stored view %+v, want sprint C", v)
	}
}

func TestQuitStoresSprintOnScreen(t *testing.T) {
	m, path := threeSprints(t)
	m = press(m, "s", "down")
	if v := storedView(t, path); v.Sprint != "s00000a" {
		t.Fatalf("an arrow in the list stored %+v, want A still stored", v)
	}
	nm, cmd := press(m, "q").Update(keyMsg("y"))
	if !quits(cmd) {
		t.Fatal("q, confirmed, should quit")
	}
	if v := storedView(t, path); v.Screen != todo.ScreenSprint || v.Sprint != "s00000c" {
		t.Errorf("after q stored view %+v, want sprint C", v)
	}
	// Nothing changed since: quitting again writes nothing.
	m = nm.(model)
	before := storeFP(t, m)
	if _, cmd := press(m, "q").Update(keyMsg("y")); !quits(cmd) || storeFP(t, m) != before {
		t.Error("q with the view unchanged must quit without a write")
	}
}

func TestSidebarIgnoresBoardKeys(t *testing.T) {
	m, path := threeSprints(t)
	m = press(m, "s")
	m = selectTitle(t, m, "aNext") // the board's cursor is on a task
	before := storeFP(t, m)
	// [ ] 1–4 jump the sidebar's sections; c no longer closes a sprint.
	for _, key := range []string{"a", "A", "m", "t", "c"} {
		got := press(m, key)
		if got.boardColumn() != todo.ColNext || got.mode != modeMain || got.tree.grabbed || got.view.sprint != "s00000a" {
			t.Errorf("%q in the sidebar: column %v mode %v grabbed %v, want nothing", key, got.boardColumn(), got.mode, got.tree.grabbed)
		}
		if it := find(loadSprintDoc(t, path), "aNext"); it == nil || it.Status != todo.New || it.SprintID() != "s00000a" || storeFP(t, got) != before {
			t.Errorf("%q in the sidebar changed the task on disk: %+v", key, it)
		}
	}
	if got := press(m, "f"); got.mode != modeStateFilter {
		t.Errorf("f in the sidebar: mode %v, want the board's state filter", got.mode)
	}
	if got := press(m, "/"); got.mode != modeSearch {
		t.Errorf("/ in the sidebar: mode %v, want the search bar", got.mode)
	}
}

func TestUClearsListFocus(t *testing.T) {
	m, _ := threeSprints(t)
	m = press(m, "s", "u")
	if !m.view.all || m.listFocus {
		t.Fatalf("u in the sidebar: unified %v listFocus %v, want the unified view with the tree focused", m.view.all, m.listFocus)
	}
	if m = press(m, "down"); m.view.sprint != "s00000a" || m.tree.cursor == 0 {
		t.Errorf("down in the unified view: sprint %q cursor %d, want the tree moved", m.view.sprint, m.tree.cursor)
	}
}

func TestEscInSidebarQuitsAndTabFocusesList(t *testing.T) {
	m, _ := threeSprints(t)
	m = press(m, "s")
	if !quitsConfirmed(t, m, "esc") {
		t.Error("esc in the sidebar with nothing to clear should quit once confirmed")
	}
	if m = press(m, "enter"); m.listFocus {
		t.Fatal("enter in the sidebar should focus the board")
	}
	if m = press(m, "shift+tab"); !m.listFocus {
		t.Error("shift+tab on the board should focus the list")
	}
}

func TestWorkspaceFootersPerFocus(t *testing.T) {
	m, _ := threeSprints(t)
	m = press(m, "s")
	f := ansi.Strip(m.footer(200))
	for _, want := range []string{"n: New", "e: Edit", "d: Delete", "tab: Board", "?: Help", "q: Quit"} {
		if !strings.Contains(f, want) {
			t.Errorf("sidebar footer %q misses %q", f, want)
		}
	}
	f = ansi.Strip(press(m, "tab").footer(200))
	for _, want := range []string{"?: Help", "/: Search", "q: Quit", "tab: Sprints"} {
		if !strings.Contains(f, want) {
			t.Errorf("board footer %q misses %q", f, want)
		}
	}
}
