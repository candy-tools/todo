package tui

import (
	"os"
	"time"

	"github.com/candy-tools/todo/internal/todo"
	tea "github.com/charmbracelet/bubbletea"
)

// pollInterval is how often todo re-reads the open file to pick up edits made
// outside the app (by a human in an editor, or an agent). It is short enough to
// feel live but far longer than a save takes, so it never fights the app's own
// writes.
const pollInterval = time.Second

// reloadTickMsg fires on every poll interval; handling it kicks off a file
// check and re-arms the next tick.
type reloadTickMsg struct{}

// fileReloadedMsg carries a document freshly parsed from disk after its content
// changed, together with the raw content it was parsed from (used to tell our
// own writes apart from external edits).
type fileReloadedMsg struct {
	doc     *todo.Document
	content string
}

// pollCmd schedules the next reload tick.
func pollCmd() tea.Cmd {
	return tea.Tick(pollInterval, func(time.Time) tea.Msg { return reloadTickMsg{} })
}

// reloadCheckCmd reads and diffs the file off the UI goroutine.
func (m model) reloadCheckCmd() tea.Cmd {
	path, last := m.path, m.lastContent
	return func() tea.Msg { return reloadCheck(path, last) }
}

// reloadCheck reads the file at path and returns a fileReloadedMsg when its
// content differs from last; it returns nil when nothing changed or the file
// can't be read (missing/permissions) — a transient read failure must never
// wipe the current view.
func reloadCheck(path, last string) tea.Msg {
	b, err := os.ReadFile(path) //nolint:gosec // path is the user-provided file the app exists to read
	if err != nil {
		return nil
	}
	content := string(b)
	if content == last {
		return nil
	}
	return fileReloadedMsg{doc: todo.Parse(content), content: content}
}

// handleReload applies an external change, unless it matches our own last write
// (nothing to do) or a modal is open (defer — the next tick re-detects it once
// the modal closes, so an edit is never yanked out from under the user).
func (m model) handleReload(msg fileReloadedMsg) (tea.Model, tea.Cmd) {
	if msg.content == m.lastContent {
		return m, nil
	}
	// Defer while a modal is open or an item is grabbed for moving, so an edit is
	// never yanked out from under the user; the next tick re-detects it once they
	// return to plain navigation.
	if m.mode != modeMain || m.tree.grabbed {
		return m, nil
	}
	m.applyReload(msg)
	return m, nil
}

// applyReload swaps in the reloaded document and rebuilds the tree, restoring
// the selection and the set of folded items by identity (title path), since the
// reparsed tree has fresh pointers. The in-memory undo snapshots are dropped —
// they reference the old pointers, and the file just changed under us.
func (m *model) applyReload(msg fileReloadedMsg) {
	onPH := m.tree.onPlaceholder()
	var selPath []string
	if !onPH {
		if sel := m.tree.selected(); sel != nil {
			selPath = sel.Path()
		}
	}
	var collapsedPaths [][]string
	for it, folded := range m.tree.collapsed {
		if folded {
			collapsedPaths = append(collapsedPaths, it.Path())
		}
	}

	m.doc = msg.doc
	m.lastContent = msg.content
	m.snapshots = map[*todo.Item]todo.DoneStates{}

	t := newTree(msg.doc)
	// Carry over the viewport geometry the fresh tree doesn't know about: its
	// height (only ever set by WindowSizeMsg) and the scroll position. Without
	// this the new tree keeps viewHeight 0, so reconcileOffset treats the window
	// as one row tall and re-pins the scroll to the cursor — silently undoing the
	// scroll/cursor decoupling until the next terminal resize.
	t.viewHeight = m.tree.viewHeight
	t.offset = m.tree.offset
	for _, p := range collapsedPaths {
		if it := msg.doc.FindByPath(p); it != nil {
			t.collapsed[it] = true
		}
	}
	t.rebuild()
	switch {
	case onPH:
		t.cursor = len(t.rows) - 1
	case selPath != nil:
		if it := msg.doc.FindByPath(selPath); it != nil {
			t.selectItem(it)
		}
	}
	m.tree = t
	m.status = "reloaded — file changed on disk"
}
