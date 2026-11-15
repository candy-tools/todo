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
// subtask under a subtask.
func (m model) openForm(t formTarget) (tea.Model, tea.Cmd) {
	sel := m.focused().selected()
	switch t {
	case targetAddSibling, targetAddChild:
		sel = m.taskAnchor()
		if sel == nil && m.split {
			m.status = msgPickCategory
			return m, nil
		}
		if sel == nil {
			// Outside Backlog a tab shows only categories holding its tasks, and c
			// is refused there, so point at a tab that shows the category instead.
			m.status = "Add a category first (press c) — tasks live under a category."
			if !m.view.editsCategories() {
				m.status = "add tasks from a tab that shows their category (Backlog shows all)"
			}
			return m, nil
		}
		if t == targetAddChild && sel.IsSubtask() {
			m.status = "subtasks can't have subtasks"
			return m, nil
		}
		m.form = newForm("", "", "", true, false)
	case targetEdit:
		if sel == nil || m.focused().onPlaceholder() {
			return m, nil
		}
		if m.categoryLocked(sel) {
			m.status = msgCategoriesInBacklog
			return m, nil
		}
		m.editing = sel
		m.form = newForm(sel.Title, sel.Description, sel.Plan, sel.IsTask(), true)
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
		m.form = newForm("", "", "", false, false)
	}
	m.target = t
	m.form.setWidth(m.width)
	m.mode = modeForm
	return m, textinput.Blink
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
		// Enter in the description inserts a newline; on the title or Save it
		// submits, on Cancel it closes.
		if m.form.focus == focusDesc {
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

// submitForm applies the open form: editing the selected item, or inserting a
// new task/subtask/category at the right spot. The insert is first made in
// memory, which works out its parent (and a category's placement), and then
// stored there; the store appends a task after its parent's last task. An empty title
// is ignored (the dialog stays open), and an edit that changed nothing closes it
// without a write. Any structural or content change clears the undo snapshots —
// the accidental-complete window is meant to be immediate.
func (m model) submitForm() (tea.Model, tea.Cmd) {
	title, desc, plan := m.form.values()
	if title == "" {
		return m, nil
	}
	m.mode = modeMain
	if m.target == targetEdit {
		// Only the fields the user changed are written: the reload is deferred
		// while the form is open, so another field may have changed in the store.
		ch := m.form.changes()
		if ch == (todo.Changes{}) {
			return m, nil
		}
		m.snapshots = map[uint]todo.DoneStates{}
		applyChanges(m.editing, ch)
		id := m.editing.ID
		m.commit(id, func(ctx context.Context) error {
			return m.store.Edit(ctx, id, ch)
		})
		return m, nil
	}

	m.snapshots = map[uint]todo.DoneStates{}
	sel := m.focused().selected()
	if m.target != targetAddCategory {
		sel = m.taskAnchor()
	}
	var added *todo.Item
	switch m.target {
	case targetAddCategory:
		added = &todo.Item{Kind: todo.Category, Title: title, Level: 1}
		// A category — or a task inside one — selected → subcategory of the
		// enclosing category; the root placeholder → a new top-level category.
		var parentCat *todo.Item
		if !m.focused().onPlaceholder() && sel != nil {
			parentCat = sel.EnclosingCategory()
		}
		if parentCat != nil {
			added.Level = parentCat.Level + 1
			parentCat.AppendChild(added)
			delete(m.focused().collapsed, parentCat.ID)
		} else {
			m.doc.AppendRoot(added)
		}
	case targetAddChild: // a subtask of a task, or a task inside a category
		added = todo.NewTask(title, desc, false)
		added.Plan = plan
		sel.AppendTask(added)
		delete(m.focused().collapsed, sel.ID)
	default: // targetAddSibling: a task at the end of the selection's level
		added = todo.NewTask(title, desc, false)
		added.Plan = plan
		parent := sel
		if sel.IsTask() && sel.Parent != nil {
			parent = sel.Parent // a task's siblings live under its parent
		}
		parent.AppendTask(added)
		delete(m.focused().collapsed, parent.ID)
	}
	// A new task enters the current tab in its column's first state, so it shows
	// where it was added; a new subtask starts open (subtasks are open or done).
	if added.Kind == todo.Task && !added.IsSubtask() {
		added.Status = m.view.newTaskState()
	}

	// A task goes after its parent's last task, a category after its parent's
	// last child, as the store finds them when it writes, so a sibling another
	// process moved or deleted does not matter.
	parentID, _ := m.doc.Placement(added)
	var newID uint
	m.commit(0, func(ctx context.Context) error {
		var err error
		if added.Kind == todo.Category {
			newID, err = m.store.AppendCategory(ctx, title, parentID)
		} else {
			newID, err = m.store.AppendTask(ctx, todo.Fields{Title: title, Description: desc, Plan: plan}, added.Status, parentID)
		}
		return err
	})
	// The ID is only known once the write ran. A task added from the sidebar
	// shows in the task list, so look there too.
	if !m.focused().selectID(newID) && m.split {
		m.tree.selectID(newID)
	}
	return m, nil
}

// applyChanges sets on it, in memory, the fields ch names.
func applyChanges(it *todo.Item, ch todo.Changes) {
	if ch.Title != nil {
		it.Title = *ch.Title
	}
	if ch.Description != nil {
		it.Description = *ch.Description
	}
	if ch.Plan != nil {
		it.Plan = *ch.Plan
	}
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
	col := it.Status.Column()
	if it.IsSubtask() && it.Parent != nil {
		col = it.Parent.Status.Column() // a subtask sits in its parent's column
	}
	status := st.Render(glyph + " " + col.Title() + " · " + it.Status.Label())
	if done, total := it.TaskCounts(); total > 0 {
		status += helpTextStyle.Render("  ·  ") + labelStyle.Render("Subtasks") + helpTextStyle.Render(fmt.Sprintf(" %d/%d done", done, total))
	}
	if it.NeedsPlan() {
		status += helpTextStyle.Render("  ·  ") + deferStyle.Render("⚠ no plan")
	}
	return status
}
