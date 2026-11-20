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

// A quit-time save that fails is kept on the final model, so Run can say the
// view was not stored.
func TestQuitKeepsTheViewSaveError(t *testing.T) {
	m, path := twoSprints(t)
	m = press(m, "s", "down") // browsing to another sprint is not stored until quit
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	m = press(m, "q")
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
	m = press(m, "s", "down", "q")
	if m.quitErr != nil || quitWarning(m) != "" {
		t.Errorf("quitErr %v warning %q, want none", m.quitErr, quitWarning(m))
	}
}
