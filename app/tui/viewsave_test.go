package tui

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/candy-tools/todo/internal/todo"
)

// viewSaveList is a one-task list file whose view section is view (a JSON
// object, or "" for none).
func viewSaveList(view string) string {
	v := ""
	if view != "" {
		v = `"view": ` + view + `, `
	}
	return `{"version": 5, ` + v + `"categories": [{"id": "c000000", "title": "C"}],
		"tasks": [{"id": "t000001", "category": "c000000", "title": "o", "status": "open"}]}`
}

// storedView loads the list at path and returns the view it holds.
func storedView(t *testing.T, path string) todo.View {
	t.Helper()
	doc, _, err := todo.NewStore(path).Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return doc.View
}

// The TUI opens on the view the list file holds.
func TestOpensOnStoredView(t *testing.T) {
	m, _ := newJSONModel(t, viewSaveList(`{"unified": true, "split": true}`))
	if !m.view.all || !m.split {
		t.Errorf("opened with unified=%v split=%v, want both on", m.view.all, m.split)
	}
	if m.tree.pane != paneTasks || !hasRow(m, "o") {
		t.Errorf("the task list should show the task, pane=%v rows=%v", m.tree.pane, rowTitles(m.tree))
	}
}

// Without a view section the TUI opens on the Backlog, without the split.
func TestOpensOnDefaultView(t *testing.T) {
	m, _ := newJSONModel(t, viewSaveList(""))
	if m.view.all || m.split || m.view.screen != screenBacklog {
		t.Errorf("opened with unified=%v split=%v screen=%v, want the Backlog", m.view.all, m.split, m.view.screen)
	}
}

// a stores the unified view in the list file, and a again the screen under it.
func TestUnifiedToggleIsStored(t *testing.T) {
	m, path := newJSONModel(t, viewSaveList(""))
	m = press(m, "u")
	if got := storedView(t, path); !got.Unified {
		t.Errorf("after a the file holds %+v, want unified", got)
	}
	press(m, "u")
	if got := storedView(t, path); got.Unified {
		t.Errorf("after a again the file holds %+v, want the Backlog", got)
	}
}

// v stores the split view in the list file, and v again its end.
func TestSplitToggleIsStored(t *testing.T) {
	m, path := newJSONModel(t, viewSaveList(""))
	m = press(m, "v")
	if got := storedView(t, path); !got.Split {
		t.Errorf("after v the file holds %+v, want split", got)
	}
	press(m, "v")
	if got := storedView(t, path); got.Split {
		t.Errorf("after v again the file holds %+v, want no split", got)
	}
}

// Storing the view is the app's own write: the next poll does not take it
// for an external change.
func TestViewSaveIsNotExternal(t *testing.T) {
	m, _ := newJSONModel(t, viewSaveList(""))
	m = press(m, "u")
	m = pollCmdOnce(m)
	if m.status != "" || m.err != nil {
		t.Errorf("after storing the view: status=%q err=%v, want neither", m.status, m.err)
	}
}

// lookOnlyList is an older (version 6) list file with the open sprints A and B
// whose view section is view, for the open-and-quit tests.
func lookOnlyList(t *testing.T, view map[string]any) string {
	t.Helper()
	return sprintJSON(t, view,
		[]seedSprint{
			{ID: "s00000a", Name: "A", Status: "open"},
			{ID: "s00000b", Name: "B", Status: "open"},
		},
		seedTask{ID: "t000001", Title: "inA", Status: "new", Sprint: "s00000a"},
	)
}

// Opening a list and quitting without a key that changes the view leaves the
// file as it was, even when the app opens on another view than the stored one
// names: the sprint list opens on the first open sprint, and a sprint that is
// gone opens on it too. Leaving through Planning does not store it either.
func TestOpenAndQuitLeavesFileAlone(t *testing.T) {
	cases := []struct {
		name string
		view map[string]any
		keys []string
	}{
		{"sprint list", map[string]any{"screen": "sprints"}, []string{"q", "enter"}},
		{"gone sprint", map[string]any{"screen": "sprint", "sprint": "s00000z"}, []string{"q", "enter"}},
		{"gone sprint, split", map[string]any{"screen": "sprint", "sprint": "s00000z", "split": true}, []string{"q", "enter"}},
		{"sprint list, via Planning", map[string]any{"screen": "sprints"}, []string{"p", "q", "enter"}},
		{"no view", nil, []string{"q", "enter"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			seed := lookOnlyList(t, c.view)
			m, path := newJSONModel(t, seed)
			m = press(m, c.keys...)
			if m.quitErr != nil || m.err != nil {
				t.Fatalf("quit: quitErr %v err %v", m.quitErr, m.err)
			}
			got, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != seed {
				t.Errorf("opening and quitting rewrote the list:\n%s\nwant it as it was\n%s", got, seed)
			}
		})
	}
}

// A view the user changed without storing it, a sprint browsed to in the
// Sprints workspace's sidebar, is stored on quit, also when the list's view
// names a sprint that is gone.
func TestQuitStoresBrowsedSprint(t *testing.T) {
	for _, view := range []map[string]any{
		{"screen": "sprints"},
		{"screen": "sprint", "sprint": "s00000z"},
	} {
		m, path := newJSONModel(t, lookOnlyList(t, view))
		press(m, "down", "q", "enter")
		want := todo.View{Screen: todo.ScreenSprint, Sprint: "s00000b"}
		if got := storedView(t, path); got != want {
			t.Errorf("view %v: after browsing to B and quitting the file holds %+v, want %+v", view, got, want)
		}
	}
}

// A sprint browsed back to is stored on quit when the list's view names
// another: the file's view would open on that one.
func TestQuitStoresSprintBrowsedBackTo(t *testing.T) {
	m, path := newJSONModel(t, lookOnlyList(t, map[string]any{"screen": "sprints"}))
	m = press(m, "down", "b", "s") // B, stored by s
	if got := storedView(t, path); got.Sprint != "s00000b" {
		t.Fatalf("after s the file holds %+v, want sprint B", got)
	}
	press(m, "up", "q", "enter")
	want := todo.View{Screen: todo.ScreenSprint, Sprint: "s00000a"}
	if got := storedView(t, path); got != want {
		t.Errorf("after browsing back to A and quitting the file holds %+v, want %+v", got, want)
	}
}

// A quit-time save that fails is kept on the final model, so Run can say the
// view was not stored.
func TestQuitKeepsTheViewSaveError(t *testing.T) {
	m, path := twoSprints(t)
	m = press(m, "s", "down") // browsing to another sprint is not stored until quit
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	m = press(m, "q", "enter")
	if m.quitErr == nil {
		t.Fatalf("quit with the file removed: quitErr nil, err %v", m.err)
	}
	if got := quitWarning(m); !strings.HasPrefix(got, "todo: the view was not saved: ") {
		t.Errorf("warning = %q", got)
	}
}

// A quit that stores the view without trouble has nothing to say.
func TestQuitWithoutSaveErrorIsSilent(t *testing.T) {
	m, _ := twoSprints(t)
	m = press(m, "s", "down", "q", "enter")
	if m.quitErr != nil || quitWarning(m) != "" {
		t.Errorf("quitErr %v warning %q, want none", m.quitErr, quitWarning(m))
	}
}
