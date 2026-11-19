package tui

import (
	"context"
	"fmt"

	"github.com/candy-tools/todo/internal/todo"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// formTarget records what submitting the open form should do.
type formTarget int

const (
	// targetAddSibling adds a task as a sibling of the selection — at the end of
	// the current level (after its peer tasks). On a category, where a task can't
	// be a sibling, it falls back to adding the task inside the category (like
	// targetAddChild). Tasks always live under a category, so this needs a
	// selection (there are no root-level tasks).
	targetAddSibling formTarget = iota
	// targetAddChild adds a task nested under the selection: a subtask of a
	// selected task, or a task inside a selected category.
	targetAddChild
	// targetAddCategory adds a category: a top-level one on the root
	// placeholder, or a subcategory of the enclosing category when a category
	// (or a task inside one) is selected.
	targetAddCategory
	targetEdit // rename/re-describe the selected item
	// targetAddSprint adds a sprint, from the Sprint list.
	targetAddSprint
	// targetEditSprint renames or re-words the goal of the sprint in m.editingSprint.
	targetEditSprint
)

// msgPickCategory is the footer hint for adding a task in the split view with
// no category to add it to: the sidebar is on All (or "+ new category") and
// the task list has no task to add next to.
const msgPickCategory = "pick a category in the sidebar to add a task"

// taskAnchor returns the item n/N add a task next to or under: the focused
// tree's selected item; in the split view's task list with nothing selected,
// the category picked in the sidebar. nil when there is none, e.g. on the
// "+ new category" row or with the sidebar on All.
func (m *model) taskAnchor() *todo.Item {
	t := m.focused()
	if t.onPlaceholder() {
		return nil
	}
	if sel := t.selected(); sel != nil {
		return sel
	}
	if m.split && !m.sideFocus {
		return m.tree.scopeItem()
	}
	return nil
}

// openForm prepares and opens the add/edit modal for the given target. Outside
// Backlog it refuses to add, rename or delete a category, and it refuses a
// subtask under a subtask and a task that would join a closed sprint.
func (m model) openForm(t formTarget) (tea.Model, tea.Cmd) {
	sel := m.focused().selected()
	switch t {
	case targetAddSibling, targetAddChild:
		var ok bool
		if m, ok = m.prepareAddForm(t); !ok {
			return m, nil
		}
	case targetEdit:
		if sel == nil || m.focused().onPlaceholder() {
			return m, nil
		}
		if m.categoryLocked(sel) {
			m.status = msgCategoriesInBacklog
			return m, nil
		}
		m.editing = sel
		// The form shows the stored text made safe to draw, as the rows do: a
		// legacy value may hold control or bidi characters. The form compares
		// against what it opened with, so an untouched field is not rewritten.
		m.form = newForm(todo.Fields{
			Title: todo.Printable(sel.Title), Ref: todo.Printable(sel.Ref),
			Description: todo.PrintableLines(sel.Description), Plan: todo.Printable(sel.Plan),
		}, formKindOf(sel), true)
		m.form.meta = itemMeta(sel)
	case targetAddCategory:
		if m.split && !m.sideFocus {
			m.status = "categories are managed in the sidebar"
			return m, nil
		}
		if !m.view.editsCategories() {
			m.status = msgCategoriesInBacklog
			return m, nil
		}
		m.form = newForm(todo.Fields{}, formCategory, false)
	}
	m.target = t
	m.form.setWidth(m.width)
	m.mode = modeForm
	return m, textinput.Blink
}

// prepareAddForm builds the add-task / add-subtask form for openForm. When the
// add is refused it sets the footer message and reports false.
func (m model) prepareAddForm(t formTarget) (model, bool) {
	if m.view.onBoard() && m.sprintClosed(m.view.sprint) {
		m.status = todo.ErrSprintClosed.Error()
		return m, false
	}
	sel := m.taskAnchor()
	if sel == nil && m.split {
		m.status = msgPickCategory
		return m, false
	}
	if sel == nil {
		// A sprint board shows only categories holding its tasks, and c is
		// refused there, so point at the Backlog, where a task is added and then
		// assigned.
		m.status = "Add a category first (press c) — tasks live under a category."
		if !m.view.editsCategories() {
			m.status = "add it in the Backlog (b, n), then assign it with p"
		}
		return m, false
	}
	if t == targetAddChild && sel.IsSubtask() {
		m.status = "subtasks can't have subtasks"
		return m, false
	}
	// A new subtask has no reference, so its form shows none.
	kind := formTask
	m.target = t
	if p := m.addParent(sel); p != nil && p.IsTask() {
		if m.sprintClosed(p.SprintID()) { // under a closed sprint's task, from the unified view
			m.status = todo.ErrSprintClosed.Error()
			return m, false
		}
		kind = formSubtask
	}
	m.form = newForm(todo.Fields{}, kind, false)
	return m, true
}

func (m model) updateForm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = modeMain
		return m, nil
	case "tab":
		return m, m.form.focusStep(1)
	case "shift+tab":
		return m, m.form.focusStep(-1)
	case "left":
		if m.form.focus == focusCancel {
			m.form.focus = focusSave
			return m, m.form.applyFocus()
		}
	case "right":
		if m.form.focus == focusSave {
			m.form.focus = focusCancel
			return m, m.form.applyFocus()
		}
	case "enter":
		// Enter in the description inserts a newline (not in a sprint's goal,
		// which is one line); on the title or Save it submits, on Cancel it
		// closes.
		if m.form.focus == focusDesc && m.form.kind != formSprint {
			break
		}
		if m.form.focus == focusCancel {
			m.mode = modeMain
			return m, nil
		}
		return m.submitForm()
	}
	var cmd tea.Cmd
	m.form, cmd = m.form.update(msg)
	return m, cmd
}

// submitForm applies the open form: editing the selected item, or adding a
// new task/subtask/category under the parent the selection gives (see
// addParent); the store appends it there, a task after its parent's last
// task. An empty title is ignored (the dialog stays open), and an edit that
// changed nothing closes it without a write. Any structural or content change
// clears the undo snapshots — the accidental-complete window is meant to be
// immediate. A failed write (the store busy with another process's write,
// say) reopens the form with what was typed, the error in the footer, so
// saving again retries it: an edit goes to the same item, found again by ID in
// the reloaded list (the form stays closed when the item is gone), and an add
// goes next to or under the same item.
func (m model) submitForm() (tea.Model, tea.Cmd) {
	v := m.form.values()
	if v.Title == "" {
		return m, nil
	}
	m.mode = modeMain
	switch m.target {
	case targetEdit:
		m.submitEdit()
		return m, nil
	case targetAddSprint, targetEditSprint:
		m.submitSprint()
		return m, nil
	}

	m.snapshots = map[string]todo.DoneStates{}
	sel := m.focused().selected()
	if m.target != targetAddCategory {
		sel = m.taskAnchor()
	}
	// The anchor is selected again if the write fails, so a retry adds at the
	// same place even when the reload shifted the rows.
	var anchorID string
	if sel != nil && !m.focused().onPlaceholder() {
		anchorID = sel.ID
	}
	parent := m.addParent(sel)
	var parentID string
	if parent != nil {
		parentID = parent.ID
		delete(m.focused().collapsed, parentID) // the new item shows under its parent
	}
	// A new task enters the current tab in its column's first state (on a
	// sprint board, in the board's sprint), so it shows where it was added; a
	// new subtask starts open (subtasks are open or done) and follows its task.
	st, sprint := m.view.newTaskPlacement()
	if parent != nil && parent.IsTask() {
		st, sprint = todo.Open, ""
	}
	v.Sprint = sprint

	// A task goes after its parent's last task, a category after its parent's
	// last child, as the store finds them when it writes, so a sibling another
	// process moved or deleted does not matter.
	var newID string
	if !m.commit("", func(ctx context.Context, s *todo.Store) (res todo.WriteResult, err error) {
		if m.target == targetAddCategory {
			newID, res, err = s.AppendCategory(ctx, v.Title, parentID)
		} else {
			newID, res, err = s.AppendTask(ctx, v, st, parentID)
		}
		return res, err
	}) {
		if anchorID != "" {
			m.adoptAll(m.doc, anchorID, false)
		}
		m.mode = modeForm
		return m, nil
	}
	// The ID is only known once the write ran. A task added from the sidebar
	// shows in the task list, so look there too.
	if !m.focused().selectID(newID) && m.split {
		m.tree.selectID(newID)
	}
	return m, nil
}

// submitEdit stores the fields of m.editing the user changed in the open form,
// for submitForm. Only those are written: the reload is deferred while the form
// is open, so another field may have changed in the store. A failed write
// reopens the form on the item found again by ID in the reloaded list, unless
// it is gone.
func (m *model) submitEdit() {
	ch := m.form.changes()
	if ch == (todo.Changes{}) {
		return
	}
	m.snapshots = map[string]todo.DoneStates{}
	id := m.editing.ID
	if m.commit(id, func(ctx context.Context, st *todo.Store) (todo.WriteResult, error) {
		return st.Edit(ctx, id, ch)
	}) {
		return
	}
	if it, ok := m.doc.ByID(id); ok {
		m.editing = it
		m.mode = modeForm
	}
}

// addParent returns the item the open add form puts its new item under, from
// sel, the selection it was opened on: for a category, the category enclosing
// sel, or nil (a top-level category) on the "+ new category" row or with
// nothing selected; for a task, sel itself (N, or n on a category), or sel's
// parent for n on a task, whose siblings live there.
func (m *model) addParent(sel *todo.Item) *todo.Item {
	switch {
	case m.target == targetAddCategory:
		if sel == nil || m.focused().onPlaceholder() {
			return nil
		}
		return sel.EnclosingCategory()
	case m.target == targetAddSibling && sel.IsTask() && sel.Parent != nil:
		return sel.Parent
	}
	return sel
}

// itemMeta renders the read-only header of the edit dialog: a task's state glyph,
// column and state, subtask progress and a missing-plan flag, or a category label and its task
// count. It carries the old details view's at-a-glance info into the combined
// view/edit dialog. Empty when there is nothing to show.
func itemMeta(it *todo.Item) string {
	if it == nil {
		return ""
	}
	if it.Kind == todo.Category {
		done, total := it.TaskCounts()
		return labelStyle.Render("Category") + helpTextStyle.Render(fmt.Sprintf("  ·  %d of %d tasks done", done, total))
	}
	glyph, st := statusGlyph(it.Status, it.IsSubtask())
	status := st.Render(glyph + " " + it.Column().Title() + " · " + it.Status.Label())
	if done, total := it.TaskCounts(); total > 0 {
		status += helpTextStyle.Render("  ·  ") + labelStyle.Render("Subtasks") + helpTextStyle.Render(fmt.Sprintf(" %d/%d done", done, total))
	}
	if it.NeedsPlan() {
		status += helpTextStyle.Render("  ·  ") + deferStyle.Render("⚠ no plan")
	}
	return status
}
