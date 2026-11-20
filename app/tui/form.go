package tui

import (
	"strings"

	"github.com/candy-tools/todo/internal/todo"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type formFocus int

const (
	focusTitle formFocus = iota
	focusRef
	focusDesc
	focusPlan
	focusSave
	focusCancel
)

// formKind is what the form adds or edits, which decides its fields.
type formKind int

const (
	formCategory formKind = iota // the title and the reference prefix
	formSubtask                  // title, description and plan
	formTask                     // a top-level task: also its reference
	formSprint                   // a sprint: its name (the title) and goal (the description)
	formSettings                 // the list's settings: its task reference prefix (the title input) and sprint reference prefix (the ref input)
)

// formKindOf returns the formKind for editing it.
func formKindOf(it *todo.Item) formKind {
	switch {
	case !it.IsTask():
		return formCategory
	case it.IsSubtask():
		return formSubtask
	}
	return formTask
}

// form is the add/edit modal: a title input plus, for a task, a multi-line
// description and a plan, and for a top-level task a reference; a category form
// shows the title and the reference prefix, a sprint form the name and the
// goal. The settings form shows the list's task and sprint reference prefixes.
// Save/Cancel sit below.
type form struct {
	title textinput.Model // the title; the settings form's task reference prefix
	// ref is a top-level task's reference, a category's reference prefix, or
	// the settings form's sprint reference prefix: a single line.
	ref     textinput.Model
	desc    textarea.Model
	plan    textinput.Model // a task's plan link: a single line
	focus   formFocus
	kind    formKind
	isTask  bool // a task or subtask form (false: a category, sprint or settings form)
	editing bool // false → adding (only affects the window title)
	width   int  // terminal width, for sizing
	// meta is the read-only header shown above the fields when editing an existing
	// item — its status and subtask progress, carried over from the old details
	// view. Empty when adding (there is no item yet).
	meta string
	// start holds the field values the form opened with, as values() reads them,
	// so an edit can send only the fields the user changed.
	start todo.Fields
	// autoRef is the reference prefix a new top-level task gets its reference
	// from, when one applies: the add form then shows the reference as
	// generated instead of a field. "" otherwise, and always when editing.
	autoRef string
}

// hasPrefix reports whether a form of kind k uses its ref input for a
// category's reference prefix rather than a task's reference.
func (k formKind) hasPrefix() bool { return k == formCategory }

func newForm(v todo.Fields, kind formKind, editing bool) form {
	ti := textinput.New()
	ti.Prompt = ""
	ti.CharLimit = todo.MaxTitleLen
	ti.SetValue(v.Title)
	if kind == formSettings {
		ti.CharLimit = todo.MaxPrefixLen
		ti.SetValue(v.Prefix)
	}

	ri := textinput.New()
	ri.Prompt = ""
	ri.CharLimit = todo.MaxRefLen
	ri.SetValue(v.Ref)
	if kind.hasPrefix() {
		ri.CharLimit = todo.MaxPrefixLen
		ri.SetValue(v.Prefix)
	}
	if kind == formSettings {
		ri.CharLimit = todo.MaxPrefixLen // the sprint prefix, in v.Ref
	}

	// The description and plan take no length limits of their own, so any stored
	// value round-trips: textarea.New caps the line count at 99 (MaxHeight) and a
	// limit would otherwise cut what an edit writes back. (CharLimit is 0, none.)
	ta := textarea.New()
	ta.Prompt = ""
	ta.MaxHeight = 0
	ta.ShowLineNumbers = false
	ta.SetValue(v.Description)
	ta.Blur()

	pi := textinput.New()
	pi.Prompt = ""
	pi.SetValue(v.Plan)

	f := form{title: ti, ref: ri, desc: ta, plan: pi, kind: kind, isTask: kind == formTask || kind == formSubtask, editing: editing, focus: focusTitle}
	f.start = f.values()
	f.title.Focus()
	return f
}

// ring is the focus order: the title, then (for a top-level task whose
// reference is not generated) the reference, (for a category) the reference
// prefix or (for the settings) the sprint reference prefix, (for a task or
// sprint) the description and (for a task) the plan, then the Save and Cancel
// buttons.
func (f form) ring() []formFocus {
	switch f.kind {
	case formTask:
		if f.autoRef != "" {
			return []formFocus{focusTitle, focusDesc, focusPlan, focusSave, focusCancel}
		}
		return []formFocus{focusTitle, focusRef, focusDesc, focusPlan, focusSave, focusCancel}
	case formSubtask:
		return []formFocus{focusTitle, focusDesc, focusPlan, focusSave, focusCancel}
	case formSprint:
		return []formFocus{focusTitle, focusDesc, focusSave, focusCancel}
	case formCategory, formSettings:
		return []formFocus{focusTitle, focusRef, focusSave, focusCancel}
	}
	return []formFocus{focusTitle, focusSave, focusCancel}
}

// focusStep moves focus dir steps around the ring, wrapping, and returns the
// blink command of whatever input gained focus.
func (f *form) focusStep(dir int) tea.Cmd {
	r := f.ring()
	idx := 0
	for i, x := range r {
		if x == f.focus {
			idx = i
			break
		}
	}
	f.focus = r[((idx+dir)%len(r)+len(r))%len(r)]
	return f.applyFocus()
}

// applyFocus focuses the input matching f.focus and blurs the rest.
func (f *form) applyFocus() tea.Cmd {
	var cmd tea.Cmd
	if f.focus == focusTitle {
		cmd = f.title.Focus()
	} else {
		f.title.Blur()
	}
	if f.focus == focusRef {
		cmd = f.ref.Focus()
	} else {
		f.ref.Blur()
	}
	if f.focus == focusDesc {
		cmd = f.desc.Focus()
	} else {
		f.desc.Blur()
	}
	if f.focus == focusPlan {
		cmd = f.plan.Focus()
	} else {
		f.plan.Blur()
	}
	return cmd
}

func (f form) modalWidth() int {
	w := f.width - 8
	if w > 88 {
		w = 88
	}
	if w < 30 {
		w = 30
	}
	return w
}

// contentWidth is the usable width inside the modal border and padding.
func (f form) contentWidth() int { return f.modalWidth() - 4 }

func (f *form) setWidth(w int) {
	f.width = w
	iw := f.contentWidth()
	f.title.Width = iw - 1
	f.ref.Width = iw - 1
	f.plan.Width = iw - 1
	f.desc.SetWidth(iw - 2) // the description box's own border adds 2
	f.desc.SetHeight(9)
}

// update feeds a message to the focused text input (typing, cursor motion).
func (f form) update(msg tea.Msg) (form, tea.Cmd) {
	var cmd tea.Cmd
	switch f.focus {
	case focusTitle:
		f.title, cmd = f.title.Update(msg)
	case focusRef:
		f.ref, cmd = f.ref.Update(msg)
	case focusDesc:
		f.desc, cmd = f.desc.Update(msg)
	case focusPlan:
		f.plan, cmd = f.plan.Update(msg)
	}
	return f, cmd
}

// values returns the fields entered: the trimmed title, reference, prefix and
// plan, and the description. A field the form does not show is "", and so is
// the reference when it is generated. The settings form returns the task
// reference prefix in Prefix and the sprint reference prefix in Ref.
func (f form) values() todo.Fields {
	if f.kind == formSettings {
		return todo.Fields{Prefix: strings.TrimSpace(f.title.Value()), Ref: strings.TrimSpace(f.ref.Value())}
	}
	v := todo.Fields{Title: strings.TrimSpace(f.title.Value())}
	if f.isTask || f.kind == formSprint {
		v.Description = strings.Trim(f.desc.Value(), "\n")
	}
	if f.isTask {
		v.Plan = strings.TrimSpace(f.plan.Value())
	}
	if f.kind == formTask && f.autoRef == "" {
		v.Ref = strings.TrimSpace(f.ref.Value())
	}
	if f.kind.hasPrefix() {
		v.Prefix = strings.TrimSpace(f.ref.Value())
	}
	return v
}

// changes names the fields whose values differ from the ones the form opened
// with (a category form has the title and prefix only), so saving an edit
// leaves alone a field someone else changed while the form was open.
func (f form) changes() todo.Changes {
	v := f.values()
	var c todo.Changes
	if v.Title != f.start.Title {
		c.Title = &v.Title
	}
	if v.Ref != f.start.Ref {
		c.Ref = &v.Ref
	}
	if v.Description != f.start.Description {
		c.Description = &v.Description
	}
	if v.Plan != f.start.Plan {
		c.Plan = &v.Plan
	}
	if v.Prefix != f.start.Prefix {
		c.Prefix = &v.Prefix
	}
	return c
}

// sprintChanges names the sprint fields (the title is the name, the description
// the goal) whose values differ from the ones the form opened with.
func (f form) sprintChanges() todo.SprintChanges {
	v := f.values()
	var c todo.SprintChanges
	if v.Title != f.start.Title {
		c.Name = &v.Title
	}
	if v.Description != f.start.Description {
		c.Goal = &v.Description
	}
	return c
}

func (f form) view() string {
	title := "Add task"
	switch {
	case f.kind == formSettings:
		title = "Settings"
	case f.kind == formSprint && f.editing:
		title = "Edit sprint"
	case f.kind == formSprint:
		title = "Add sprint"
	case !f.isTask && f.editing:
		title = "Edit category"
	case !f.isTask:
		title = "Add category"
	case f.editing:
		title = "Edit task"
	}

	var b strings.Builder
	if f.meta != "" {
		b.WriteString(f.meta)
		b.WriteString("\n\n")
	}
	titleLabel := "Title"
	switch f.kind {
	case formSprint:
		titleLabel = "Name"
	case formSettings:
		titleLabel = "Reference prefix"
	}
	b.WriteString(fieldLabel(titleLabel, f.focus == focusTitle))
	b.WriteString("\n")
	b.WriteString(f.underline(f.title.View(), f.focus == focusTitle))
	b.WriteString(f.refField())
	if f.isTask || f.kind == formSprint {
		descLabel := "Description"
		if f.kind == formSprint {
			descLabel = "Goal"
		}
		b.WriteString("\n\n")
		b.WriteString(fieldLabel(descLabel, f.focus == focusDesc))
		b.WriteString("\n")
		b.WriteString(f.descBox())
	}
	if f.isTask {
		b.WriteString("\n\n")
		b.WriteString(fieldLabel("Plan", f.focus == focusPlan))
		b.WriteString("\n")
		b.WriteString(f.underline(f.plan.View(), f.focus == focusPlan))
	}

	b.WriteString("\n\n")
	buttons := lipgloss.JoinHorizontal(lipgloss.Top,
		button("Save", f.focus == focusSave), "   ", button("Cancel", f.focus == focusCancel))
	b.WriteString(lipgloss.NewStyle().Width(f.contentWidth()).Align(lipgloss.Center).Render(buttons))

	b.WriteString("\n\n")
	sep := helpTextStyle.Render(" · ")
	b.WriteString(hint("tab", "Move") + sep + hint("enter", "Save") + sep + hint("esc", "Cancel"))

	body := lipgloss.NewStyle().Padding(0, 1).Render(b.String())
	return titledBox(title, body, f.modalWidth(), lipgloss.Height(body)+2, true)
}

// refField renders the field below the title that the ref input backs, with
// the blank line before it: a top-level task's reference (shown, not typed,
// when it is generated), a category's reference prefix or the settings'
// sprint reference prefix. It is "" for a form without one.
func (f form) refField() string {
	label := ""
	switch {
	case f.kind == formTask && f.autoRef != "":
		// The reference is generated when the task is saved: shown, not typed.
		return "\n\n" + fieldLabel("Reference", false) + "\n" + helpTextStyle.Render(f.autoRef+"-… (automatic)")
	case f.kind == formTask:
		label = "Reference"
	case f.kind.hasPrefix():
		label = "Reference prefix"
	case f.kind == formSettings:
		label = "Sprint reference prefix"
	default:
		return ""
	}
	return "\n\n" + fieldLabel(label, f.focus == focusRef) + "\n" + f.underline(f.ref.View(), f.focus == focusRef)
}

// underline renders a value over a single accent/dim bottom border — the
// title field's affordance.
func (f form) underline(view string, focused bool) string {
	c := colDim
	if focused {
		c = colAccent
	}
	return lipgloss.NewStyle().
		Border(lipgloss.NormalBorder(), false, false, true, false).
		BorderForeground(c).
		Width(f.contentWidth()).
		Render(view)
}

// descBox wraps the description textarea in a rounded border, accented when
// focused.
func (f form) descBox() string {
	c := colDim
	if f.focus == focusDesc {
		c = colAccent
	}
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(c).
		Render(f.desc.View())
}

func fieldLabel(s string, focused bool) string {
	if focused {
		return focusLabelStyle.Render(s)
	}
	return labelStyle.Render(s)
}

func button(label string, focused bool) string {
	st := lipgloss.NewStyle().Foreground(colDim)
	if focused {
		st = lipgloss.NewStyle().Foreground(colAccent).Bold(true)
	}
	return st.Render("[ " + label + " ]")
}
