package tui

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/candy-tools/todo/internal/todo"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

// loadSprintDoc reads the list file at path back.
func loadSprintDoc(t *testing.T, path string) *todo.Document {
	t.Helper()
	doc, _, err := todo.NewStore(path).Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

func TestSprintListNewCreatesSprint(t *testing.T) {
	m, path := threeSprints(t)
	m = press(m, "s", "n")
	if m.mode != modeForm || m.form.kind != formSprint {
		t.Fatalf("n: mode %v kind %v, want the sprint form", m.mode, m.form.kind)
	}
	m = typeText(m, "Delta")
	m = press(m, "tab", "tab") // past the reference prefix to the goal
	m = typeText(m, "ship it")
	m = press(m, "enter")
	if m.mode != modeMain || m.err != nil {
		t.Fatalf("after enter: mode %v err %v", m.mode, m.err)
	}
	s, ok := loadSprintDoc(t, path).SprintByName("Delta")
	if !ok || s.Goal != "ship it" || s.Status != todo.SprintOpen {
		t.Fatalf("stored sprint = %+v (found %v), want open Delta with its goal", s, ok)
	}
	if sel := m.sprints.selected(m.doc); sel == nil || sel.ID != s.ID {
		t.Errorf("selected = %v, want the new sprint", sel)
	}
}

func TestSprintListNewDuplicateKeepsForm(t *testing.T) {
	m, _ := threeSprints(t)
	m = typeText(press(m, "s", "n"), "a")
	m = press(m, "enter")
	// A refused write is no failed save: the reason is the footer status.
	if m.mode != modeForm || m.err != nil || !strings.Contains(m.status, todo.ErrDuplicateSprint.Error()) {
		t.Errorf("a duplicate name: mode %v err %v status %q, want the form open with the refusal", m.mode, m.err, m.status)
	}
}

func TestSprintListEditRenames(t *testing.T) {
	m, path := threeSprints(t)
	m = press(m, "s", "e")
	if m.mode != modeForm || m.form.title.Value() != "A" || m.form.desc.Value() != "goal a" {
		t.Fatalf("e: mode %v name %q goal %q, want A's fields", m.mode, m.form.title.Value(), m.form.desc.Value())
	}
	m = typeText(m, "lpha")
	m = press(m, "enter")
	if m.mode != modeMain || m.err != nil {
		t.Fatalf("after enter: mode %v err %v", m.mode, m.err)
	}
	s, _ := loadSprintDoc(t, path).SprintByID("s00000a")
	if s.Name != "Alpha" || s.Goal != "goal a" {
		t.Errorf("stored sprint = %+v, want Alpha with the goal untouched", s)
	}
}

func TestSprintListCloseRefusedShowsCount(t *testing.T) {
	m, path := threeSprints(t) // A holds a task in Next
	m = press(m, "s", "c")
	if m.err != nil {
		t.Errorf("err = %v, want none: a refused close is a status", m.err)
	}
	if m.status != "can't close: 1 task not done" {
		t.Errorf("status = %q", m.status)
	}
	if s, _ := loadSprintDoc(t, path).SprintByID("s00000a"); s.Status != todo.SprintOpen {
		t.Errorf("A is %v, want still open", s.Status)
	}
	if f := m.footer(100); !strings.Contains(f, "can't close: 1 task not done") {
		t.Errorf("footer = %q", f)
	}
}

func TestSprintListCloseThenReopen(t *testing.T) {
	m, path := newJSONModel(t, sprintJSON(t, nil,
		[]seedSprint{{ID: "s00000a", Name: "A", Status: "open"}, {ID: "s00000b", Name: "B", Status: "open"}},
		seedTask{ID: "t000001", Title: "x", Status: "done", Sprint: "s00000a"}))
	m = press(m, "s", "c")
	if m.err != nil || m.status != "" {
		t.Fatalf("close: err %v status %q", m.err, m.status)
	}
	if s, _ := loadSprintDoc(t, path).SprintByID("s00000a"); s.Status != todo.SprintDone {
		t.Fatalf("A is %v, want done", s.Status)
	}
	// A moved below the open B; the cursor stays on A, so c reopens it.
	if sel := m.sprints.selected(m.doc); sel == nil || sel.ID != "s00000a" {
		t.Fatalf("selected = %v, want A after it moved", sel)
	}
	m = press(m, "c")
	if s, _ := loadSprintDoc(t, path).SprintByID("s00000a"); s.Status != todo.SprintOpen {
		t.Errorf("A is %v, want reopened", s.Status)
	}
}

func TestSprintListDeleteConfirm(t *testing.T) {
	m, path := newJSONModel(t, sprintJSON(t, nil,
		[]seedSprint{{ID: "s00000a", Name: "A", Status: "open"}},
		seedTask{ID: "t000001", Title: "w", Status: "working", Sprint: "s00000a"}))
	m = press(m, "s", "d")
	if m.mode != modeConfirm {
		t.Fatalf("d: mode %v, want the confirm dialog", m.mode)
	}
	if q, want := m.confirmQuestion(), `Delete sprint "A"? Its unfinished tasks go back to the Backlog.`; q != want {
		t.Errorf("question = %q, want %q", q, want)
	}
	m = press(m, "y")
	doc := loadSprintDoc(t, path)
	if len(doc.Sprints) != 0 {
		t.Errorf("sprints = %v, want none", doc.Sprints)
	}
	if it := find(doc, "w"); it == nil || it.Status != todo.Open || it.SprintID() != "" {
		t.Errorf("task = %+v, want open in the backlog", it)
	}
	if m.err != nil {
		t.Errorf("err = %v", m.err)
	}
}

func TestSprintListDeleteFallsBackByIndex(t *testing.T) {
	m, path := threeSprints(t) // rows: A, C (Open), B (Done)
	m = press(m, "s", "down", "d", "y")
	if len(loadSprintDoc(t, path).Sprints) != 2 {
		t.Fatalf("C was not deleted")
	}
	if s := m.sprints.selected(m.doc); s == nil || s.ID != "s00000b" || m.view.sprint != "s00000b" {
		t.Errorf("after deleting C: selected %v board %q, want B, now at C's index, and its board", s, m.view.sprint)
	}
	if !m.listFocus {
		t.Error("the sidebar should keep the focus")
	}
}

func TestSprintListDeleteCancelKeepsSprint(t *testing.T) {
	m, path := threeSprints(t)
	m = press(m, "s", "d", "esc")
	if len(loadSprintDoc(t, path).Sprints) != 3 || m.mode != modeMain {
		t.Errorf("esc must cancel the delete")
	}
}

func TestSprintListDeleteRefusedWithDone(t *testing.T) {
	m, path := threeSprints(t) // A holds a done task
	m = press(m, "s", "d", "y")
	if m.err != nil {
		t.Errorf("err = %v, want none: a refused delete is a status", m.err)
	}
	if m.status != todo.ErrSprintHasDone.Error() {
		t.Errorf("status = %q, want %q", m.status, todo.ErrSprintHasDone.Error())
	}
	if _, ok := loadSprintDoc(t, path).SprintByID("s00000a"); !ok {
		t.Error("A must still exist")
	}
}

func TestSprintListFooterShowsOwnHints(t *testing.T) {
	m, _ := threeSprints(t)
	m = press(m, "s")
	m.view.filter = "needle" // carried over from a board
	f := ansi.Strip(m.footer(120))
	if strings.Contains(f, "filter") || strings.Contains(f, "Clear") {
		t.Errorf("footer = %q, must not show the search filter", f)
	}
	for _, want := range []string{"New", "Edit", "Close/reopen", "Delete", "Board"} {
		if !strings.Contains(f, want) {
			t.Errorf("footer = %q, misses %q", f, want)
		}
	}
}

// manySprints is a list of n open sprints named S00, S01, ….
func manySprints(t *testing.T, n int) model {
	t.Helper()
	var ss []seedSprint
	for i := 0; i < n; i++ {
		ss = append(ss, seedSprint{ID: fmt.Sprintf("s0000%02d", i), Name: fmt.Sprintf("S%02d", i), Status: "open"})
	}
	m, _ := newJSONModel(t, sprintJSON(t, nil, ss))
	return press(m, "s")
}

// sidebarLines renders m's sidebar, width x height, as plain lines.
func sidebarLines(m model, width, height int) []string {
	return strings.Split(ansi.Strip(m.sprints.view(m.doc, width, height)), "\n")
}

func TestSprintListScrollsOnlyWhenCursorLeavesWindow(t *testing.T) {
	m := manySprints(t, 60)
	h := m.sprintListHeight()
	for i := 0; i < h/rowLines+3; i++ {
		m = press(m, "down")
	}
	top := sidebarLines(m, 100, h)[0]
	if strings.HasPrefix(top, "━━") {
		t.Fatalf("first line %q after %d downs, want the window scrolled off the Open rule", top, h/rowLines+3)
	}
	for i := 0; i < 2; i++ {
		m = press(m, "up")
		if got := sidebarLines(m, 100, h)[0]; got != top {
			t.Fatalf("up %d: first line %q, want it to stay %q", i+1, got, top)
		}
	}
	// Going above the window moves it by one row, the cursor's on top.
	for strings.Contains(sidebarLines(m, 100, h)[0], top) {
		m = press(m, "up")
	}
	if got := sidebarLines(m, 100, h)[0]; !strings.HasPrefix(got, "❯ ") {
		t.Errorf("first line %q, want the window to follow the cursor up", got)
	}
}

// sectionedSprints is open A (a Next task, a goal) and C (no goal) beside done
// B (older, a goal) and D (newer, no goal), each with a done task.
func sectionedSprints(t *testing.T) (model, string) {
	t.Helper()
	return newJSONModel(t, sprintJSON(t, nil,
		[]seedSprint{
			{ID: "s00000a", Name: "A", Goal: "goal a", Status: "open"},
			{ID: "s00000b", Name: "B", Goal: "goal b", Status: "done"},
			{ID: "s00000c", Name: "C", Status: "open"},
			{ID: "s00000d", Name: "D", Status: "done"},
		},
		seedTask{ID: "t000001", Title: "aNext", Status: "new", Sprint: "s00000a"},
		seedTask{ID: "t000002", Title: "bDone", Status: "done", Sprint: "s00000b"},
		seedTask{ID: "t000003", Title: "dDone", Status: "done", Sprint: "s00000d"}))
}

// The sidebar lists the open sprints under an Open rule, in creation order,
// then the done ones under a Done rule, newest first, as a board's sections.
func TestSidebarSectionsListOpenThenDone(t *testing.T) {
	m, _ := sectionedSprints(t)
	m = press(m, "s")
	rule := func(label string) string { return label + strings.Repeat("━", 30-ansi.StringWidth(label)) }
	row := func(lead, counts string) string {
		return lead + strings.Repeat(" ", 30-ansi.StringWidth(lead)-ansi.StringWidth(counts)) + counts
	}
	want := []string{
		rule("━━ Open 2 "),
		row("❯ ● A", "1·0·0"), "    goal a",
		row("  ● C", "0·0·0"), "    —",
		rule("━━ Done 2 "),
		row("  ✓ D", "0·0·1"), "    —",
		row("  ✓ B", "0·0·1"), "    goal b",
	}
	if got := sidebarLines(m, 30, 20); !equalStrings(got, want) {
		t.Errorf("sidebar =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// An empty section shows as its rule alone, as on a board.
func TestSidebarEmptySectionShowsItsRuleAlone(t *testing.T) {
	m := manySprints(t, 1)
	if got := sidebarLines(m, 30, 20); len(got) != 4 || !strings.HasPrefix(got[3], "━━ Done 0 ━") {
		t.Errorf("one open sprint renders %q, want its rule, its row, then the Done rule alone", got)
	}
	only, _ := newJSONModel(t, sprintJSON(t, nil, []seedSprint{{ID: "s00000b", Name: "B", Status: "done"}}))
	only = press(only, "s")
	if got := sidebarLines(only, 30, 20); len(got) != 4 || !strings.HasPrefix(got[0], "━━ Open 0 ━") || !strings.HasPrefix(got[1], "━━ Done 1 ━") {
		t.Errorf("one done sprint renders %q, want the Open rule alone, then the Done rule and its row", got)
	}
}

// The arrows walk the sprints across both sections, never onto a rule, and
// show each one's board at once.
func TestSidebarArrowsSkipRules(t *testing.T) {
	m, _ := sectionedSprints(t)
	m = press(m, "s")
	for _, step := range []struct{ key, want string }{
		{"down", "s00000c"}, {"down", "s00000d"}, {"down", "s00000b"}, {"down", "s00000b"},
		{"up", "s00000d"}, {"up", "s00000c"}, {"up", "s00000a"}, {"up", "s00000a"},
	} {
		m = press(m, step.key)
		if s := m.sprints.selected(m.doc); s == nil || s.ID != step.want || m.view.sprint != step.want {
			t.Fatalf("%s: selected %v board %q, want %s on both", step.key, s, m.view.sprint, step.want)
		}
	}
}

// A done sprint's row shows its frozen board, which c in the sidebar reopens.
func TestSidebarDoneSprintShowsItsBoard(t *testing.T) {
	m, path := sectionedSprints(t)
	m = press(m, "s", "down", "down") // A, C, then D
	for _, want := range []string{"D ✓ closed", "closed — c in the sidebar reopens it", "dDone"} {
		if !inBoardPane(m, want) {
			t.Errorf("D's board misses %q:\n%s", want, ansi.Strip(m.View()))
		}
	}
	if f := ansi.Strip(press(m, "tab").footer(200)); strings.Contains(f, "a/A:") {
		t.Errorf("D's board footer = %q, want it frozen (no a/A)", f)
	}
	m = press(m, "c")
	if s, _ := loadSprintDoc(t, path).SprintByID("s00000d"); s.Status != todo.SprintOpen {
		t.Fatalf("c on D: it is %v, want reopened", s.Status)
	}
	if s := m.sprints.selected(m.doc); s == nil || s.ID != "s00000d" || m.view.sprint != "s00000d" {
		t.Errorf("after reopening D: selected %v board %q, want the cursor kept on D, now under Open", s, m.view.sprint)
	}
	if got := sidebarLines(m, 30, 20); !strings.HasPrefix(got[0], "━━ Open 3 ") || !strings.Contains(got[5], "D") {
		t.Errorf("after reopening D the sidebar is %q, want D last under Open", got)
	}
}

// [ ] step between the sections that hold sprints, wrapping around, and 1 2
// jump to Open and Done, onto a section's first sprint, showing its board.
func TestSidebarSectionKeys(t *testing.T) {
	m, _ := sectionedSprints(t)
	m = press(m, "s", "down") // on C
	for _, step := range []struct{ key, want string }{
		{"]", "s00000d"}, {"]", "s00000a"}, {"[", "s00000d"}, {"[", "s00000a"},
		{"2", "s00000d"}, {"2", "s00000d"}, {"1", "s00000a"},
	} {
		m = press(m, step.key)
		if s := m.sprints.selected(m.doc); s == nil || s.ID != step.want || m.view.sprint != step.want || m.status != "" {
			t.Fatalf("%s: selected %v board %q footer %q, want %s on both", step.key, s, m.view.sprint, m.status, step.want)
		}
	}
	if !m.listFocus {
		t.Error("the section keys keep the keys on the sidebar")
	}
}

// With every sprint open, 2 says Done is empty and leaves the cursor; [ and ]
// find no other section and go to the first open sprint, as on a board.
func TestSidebarSectionKeysOnEmptySection(t *testing.T) {
	m := manySprints(t, 3)
	m = press(m, "down", "2")
	if s := m.sprints.selected(m.doc); s == nil || s.ID != "s000001" || m.status != "nothing in Done" {
		t.Errorf("2: selected %v footer %q, want S01 kept and %q", s, m.status, "nothing in Done")
	}
	for _, key := range []string{"]", "["} {
		got := press(m, "down", key)
		if s := got.sprints.selected(got.doc); s == nil || s.ID != "s000000" || got.view.sprint != "s000000" || got.status != "" {
			t.Errorf("%s: selected %v board %q footer %q, want S00, the first open sprint", key, s, got.view.sprint, got.status)
		}
	}
	empty, _ := newJSONModel(t, sprintJSON(t, nil, nil))
	if empty = press(empty, "s", "1"); empty.status != "nothing in Open" {
		t.Errorf("1 on an empty list: footer %q, want %q", empty.status, "nothing in Open")
	}
}

// The window counts the one-line rules and the two-line rows: the cursor's
// row is always whole on screen, the pane never overfilled, and the rule over
// a section's first sprint comes into view with it.
func TestSidebarScrollWithRules(t *testing.T) {
	var ss []seedSprint
	for i := range 12 {
		status := "open"
		if i%2 == 1 {
			status = "done"
		}
		ss = append(ss, seedSprint{ID: fmt.Sprintf("s0000%02d", i), Name: fmt.Sprintf("S%02d", i), Status: status})
	}
	m, _ := newJSONModel(t, sprintJSON(t, nil, ss))
	m = send(m, tea.WindowSizeMsg{Width: 100, Height: 12}) // 7 pane rows
	m = press(m, "s")
	h := m.sprintListHeight()
	check := func(when string) {
		t.Helper()
		lines := sidebarLines(m, 30, h)
		at := slices.IndexFunc(lines, func(l string) bool { return strings.HasPrefix(l, "❯ ") })
		if len(lines) > h || at < 0 || at+1 >= len(lines) {
			t.Fatalf("%s: %d lines (pane %d), cursor row at %d:\n%s", when, len(lines), h, at, strings.Join(lines, "\n"))
		}
		if s := m.sprints.selected(m.doc); !strings.Contains(lines[at], s.Name) {
			t.Fatalf("%s: cursor line %q, want %s", when, lines[at], s.Name)
		}
	}
	for i := 0; i < 12; i++ {
		check(fmt.Sprintf("after %d downs", i))
		m = press(m, "down")
	}
	for i := 0; i < 12; i++ {
		check(fmt.Sprintf("after %d ups", i))
		m = press(m, "up")
	}
	m = press(m, "1", "down", "down", "down", "down", "down", "down") // S00 … S10, then S11
	check("down onto Done")
	if lines := sidebarLines(m, 30, h); !strings.HasPrefix(lines[len(lines)-3], "━━ Done 6 ") {
		t.Errorf("on Done's first sprint the sidebar is\n%s\nwant the Done rule right above it", strings.Join(lines, "\n"))
	}
	m = press(m, "2", "1")
	if lines := sidebarLines(m, 30, h); !strings.HasPrefix(lines[0], "━━ Open 6 ") || !strings.HasPrefix(lines[1], "❯ ● S00") {
		t.Errorf("1 shows\n%s\nwant the Open rule on top over S00", strings.Join(lines, "\n"))
	}
	m = press(m, "2")
	if lines := sidebarLines(m, 30, h); !strings.HasPrefix(lines[0], "━━ Done 6 ") || !strings.HasPrefix(lines[1], "❯ ✓ S11") {
		t.Errorf("2 shows\n%s\nwant the Done rule on top over S11", strings.Join(lines, "\n"))
	}
}

func TestSprintListCursorFollowsSprintOnReload(t *testing.T) {
	m, path := newJSONModel(t, sprintJSON(t, nil, []seedSprint{
		{ID: "s00000a", Name: "A", Status: "open"},
		{ID: "s00000b", Name: "B", Status: "open"},
		{ID: "s00000c", Name: "C", Status: "open"},
	}))
	m = press(m, "s", "down", "down") // on C
	// Another session closes A and B: they move below C.
	st := openSecond(t, path)
	for _, id := range []string{"s00000a", "s00000b"} {
		if _, err := st.CloseSprint(context.Background(), id); err != nil {
			t.Fatal(err)
		}
	}
	m = pollOnce(t, m)
	if sel := m.sprints.selected(m.doc); sel == nil || sel.ID != "s00000c" {
		t.Errorf("selected = %v, want C (by ID, not index 2)", sel)
	}
}

func TestSidebarRowsAreTwoLinesAndCut(t *testing.T) {
	long := strings.Repeat("n", 80)
	m, _ := newJSONModel(t, sprintJSON(t, nil,
		[]seedSprint{{ID: "s00000a", Name: long, Goal: strings.Repeat("g", 80), Status: "open"}},
		seedTask{ID: "t000001", Title: "x", Status: "new", Sprint: "s00000a"}))
	lines := sidebarLines(m, 30, 10)
	if len(lines) != 4 {
		t.Fatalf("one sprint renders %d lines, want its two between the rules: %q", len(lines), lines)
	}
	for _, l := range lines {
		if ansi.StringWidth(l) > 30 {
			t.Errorf("line %q is wider than 30", l)
		}
	}
	if !strings.HasSuffix(lines[1], "1·0·0") || ansi.StringWidth(lines[1]) != 30 {
		t.Errorf("first line %q, want the counts ending at the right edge (width 30)", lines[1])
	}
	if !strings.Contains(lines[1], "…") || !strings.Contains(lines[1], "1·0·0") || !strings.Contains(lines[2], "…") {
		t.Errorf("lines = %q, want the name and goal cut and the counts kept", lines)
	}
}

func TestSidebarRowWithoutGoalShowsDash(t *testing.T) {
	m, _ := newJSONModel(t, sprintJSON(t, nil,
		[]seedSprint{{ID: "s00000a", Name: "A", Status: "open"}},
		seedTask{ID: "t000001", Title: "x", Status: "new", Sprint: "s00000a"}))
	lines := sidebarLines(m, 30, 10)
	if len(lines) != 4 || strings.TrimSpace(lines[2]) != "—" {
		t.Errorf("lines = %q, want a dim — as the goal of a sprint without one", lines)
	}
}

func TestSidebarScrollCountsLines(t *testing.T) {
	m := manySprints(t, 20)
	if got := len(sidebarLines(m, 30, 9)); got != 9 {
		t.Errorf("9 lines hold the rule and 4 two-line rows, got %d lines", got)
	}
	if got := len(sidebarLines(m, 30, 1)); got != 2 {
		t.Errorf("a one-line height still shows the cursor's row (2 lines), got %d", got)
	}
}

func TestKToDoneSprintSelectsItInSidebar(t *testing.T) {
	m, _ := threeSprints(t)
	m = press(m, "K", "down", "down", "enter") // A, C, ✓ B
	if s := m.sprints.selected(m.doc); m.view.sprint != "s00000b" || s == nil || s.ID != "s00000b" {
		t.Fatalf("K to B: board %q selected %v, want B on both", m.view.sprint, s)
	}
}

func TestClosingSelectedSprintMovesItToDone(t *testing.T) {
	m, _ := newJSONModel(t, sprintJSON(t, nil,
		[]seedSprint{{ID: "s00000a", Name: "A", Status: "open"}, {ID: "s00000b", Name: "B", Status: "open"}},
		seedTask{ID: "t000001", Title: "x", Status: "done", Sprint: "s00000a"}))
	m = press(m, "s", "c")
	if s := m.sprints.selected(m.doc); s == nil || s.ID != "s00000a" || m.view.sprint != "s00000a" {
		t.Errorf("after closing A: selected %v board %q, want A kept and shown", s, m.view.sprint)
	}
	if lines := sidebarLines(m, 30, 20); !strings.HasPrefix(lines[3], "━━ Done 1 ") || !strings.HasPrefix(lines[4], "❯ ✓ A") {
		t.Errorf("after closing A the sidebar is %q, want A under Done", lines)
	}
}

func TestReloadCursorStaysOnSprintClosedElsewhere(t *testing.T) {
	m, path := newJSONModel(t, sprintJSON(t, nil,
		[]seedSprint{
			{ID: "s00000a", Name: "A", Status: "open"},
			{ID: "s00000b", Name: "B", Status: "open"},
			{ID: "s00000d", Name: "D", Status: "done"},
		}))
	m = press(m, "s") // on A
	if _, err := openSecond(t, path).CloseSprint(context.Background(), "s00000a"); err != nil {
		t.Fatal(err)
	}
	m = pollOnce(t, m)
	if s := m.sprints.selected(m.doc); s == nil || s.ID != "s00000a" || m.view.sprint != "s00000a" {
		t.Errorf("selected %v board %q, want A kept and shown under Done", s, m.view.sprint)
	}
}

func TestDeletingLastOpenSprintGoesToNewestDone(t *testing.T) {
	m, _ := newJSONModel(t, sprintJSON(t, nil,
		[]seedSprint{{ID: "s00000a", Name: "A", Status: "open"}, {ID: "s00000d", Name: "D", Status: "done"}}))
	m = press(m, "s", "d", "y")
	if s := m.sprints.selected(m.doc); s == nil || s.ID != "s00000d" || m.view.sprint != "s00000d" || !m.listFocus {
		t.Errorf("selected %v board %q listFocus %v, want D, now first, shown with the sidebar focused", s, m.view.sprint, m.listFocus)
	}
}

// A reload that deletes the shown sprint, leaving only done ones, shows the
// newest done sprint, the sidebar's first.
func TestReloadGoneSprintFallsBackToNewestDone(t *testing.T) {
	m, path := newJSONModel(t, sprintJSON(t, nil,
		[]seedSprint{
			{ID: "s00000a", Name: "A", Status: "open"},
			{ID: "s00000b", Name: "B", Status: "done"},
			{ID: "s00000d", Name: "D", Status: "done"},
		}))
	m = press(m, "s") // on A
	deleteSprintOnDisk(t, path, "s00000a")
	m = pollOnce(t, m)
	if s := m.sprints.selected(m.doc); s == nil || s.ID != "s00000d" || m.view.sprint != "s00000d" || !m.listFocus {
		t.Errorf("selected %v board %q listFocus %v, want D on both with the sidebar focused", s, m.view.sprint, m.listFocus)
	}
}

// An empty list has no sprint to edit, close, delete or show, so its footer
// offers only n.
func TestSidebarFooterDropsDeadHints(t *testing.T) {
	empty, _ := newTestModel(t, "# Work\n- [ ] a\n")
	empty = press(empty, "s")
	f := ansi.Strip(empty.footer(120))
	for _, gone := range []string{"e:", "c:", "d:", "tab:"} {
		if strings.Contains(f, gone) {
			t.Errorf("empty list footer = %q, must not offer %q", f, gone)
		}
	}
	for _, want := range []string{"n: New", "?: Help", "q: Quit"} {
		if !strings.Contains(f, want) {
			t.Errorf("empty list footer = %q, misses %q", f, want)
		}
	}
}

// At 60 cells the sidebar's and Planning's footers drop whole hints, Help
// first, never cutting one.
func TestSidebarAndPlanFootersFitWholeHints(t *testing.T) {
	check := func(name, f string, w int) {
		t.Helper()
		if ansi.StringWidth(f) > w || !strings.HasPrefix(f, " ?: Help") {
			t.Errorf("%s footer at %d = %q, want it to fit and start with Help", name, w, f)
		}
		for _, part := range strings.Split(strings.TrimSpace(f), "  ") {
			if !strings.Contains(part, ": ") {
				t.Errorf("%s footer at %d = %q holds a cut hint %q", name, w, f, part)
			}
		}
	}
	m, _ := threeSprints(t)
	m = press(m, "s")
	check("sidebar", ansi.Strip(m.footer(60)), 60)
	m = press(m, "p")
	check("planning left", ansi.Strip(m.footer(60)), 60)
	if f := ansi.Strip(m.footer(200)); !strings.Contains(f, "q: Quit") {
		t.Errorf("planning footer = %q, misses q: Quit", f)
	}
	m = press(m, "tab")
	check("planning right", ansi.Strip(m.footer(60)), 60)
}

// The board footer leads with Help and the section keys; the columns layout
// and its L are gone.
func TestBoardFooterSectionHints(t *testing.T) {
	m, _ := twoSprints(t)
	m = send(m, tea.WindowSizeMsg{Width: 80, Height: 30})
	m = press(m, "s", "tab")
	f := ansi.Strip(m.footer(200))
	for _, want := range []string{"[ ] 1-3: Section", "a/A: Move task"} {
		if !strings.Contains(f, want) {
			t.Errorf("footer at 80 = %q, misses %q", f, want)
		}
	}
	if strings.Contains(f, "Next/prev column") {
		t.Errorf("footer at 80 = %q, the old a/A label is gone", f)
	}
	if !strings.HasPrefix(f, " ?: Help  tab: Sprints") {
		t.Errorf("footer at 80 = %q, want Help then tab Sprints first", f)
	}
	wide := send(m, tea.WindowSizeMsg{Width: 140, Height: 30})
	if f := ansi.Strip(wide.footer(200)); strings.Contains(f, "L:") {
		t.Errorf("footer at 140 = %q, want no L", f)
	}
}

func TestClosedSprintBoardFooterHasNoMoveHint(t *testing.T) {
	m, _ := threeSprints(t)
	m = send(m, tea.WindowSizeMsg{Width: 80, Height: 30})
	m = press(m, "K", "down", "down", "enter", "tab") // the closed sprint B
	if m.view.sprint != "s00000b" {
		t.Fatalf("board = %q, want the closed sprint B", m.view.sprint)
	}
	f := ansi.Strip(m.footer(200))
	if strings.Contains(f, "a/A:") || !strings.HasPrefix(f, " ?: Help") {
		t.Errorf("closed board footer = %q, want no a/A hint and Help first", f)
	}
}
