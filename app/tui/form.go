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
	focusDesc
	focusPlan
	focusSave
	focusCancel
)

// form is the add/edit modal: a title input plus, for a task, a multi-line
// description; a category form shows the title only. Save/Cancel sit below.
type form struct {
	title   textinput.Model
	desc    textarea.Model
	plan    textinput.Model // a task's plan link: a single line
	focus   formFocus
	isTask  bool // false → category form (title only)
	editing bool // false → adding (only affects the window title)
	width   int  // terminal width, for sizing
	// meta is the read-only header shown above the fields when editing an existing
	// item — its status and subtask progress, carried over from the old details
	// view. Empty when adding (there is no item yet).
	meta string
	// start holds the field values the form opened with, as values() reads them,
	// so an edit can send only the fields the user changed.
	start [3]string
}

func newForm(titleVal, descVal, planVal string, isTask, editing bool) form {
	ti := textinput.New()
	ti.Prompt = ""
	ti.CharLimit = todo.MaxTitleLen
	ti.SetValue(titleVal)

	ta := textarea.New()
	ta.Prompt = ""
	ta.ShowLineNumbers = false
	ta.SetValue(descVal)
	ta.Blur()

	pi := textinput.New()
	pi.Prompt = ""
	pi.CharLimit = 500
	pi.SetValue(planVal)

	f := form{title: ti, desc: ta, plan: pi, isTask: isTask, editing: editing, focus: focusTitle}
	f.start[0], f.start[1], f.start[2] = f.values()
	f.title.Focus()
	return f
}

// ring is the focus order: the title, then (for a task) the description and plan, then
// the Save and Cancel buttons.
func (f form) ring() []formFocus {
	if f.isTask {
		return []formFocus{focusTitle, focusDesc, focusPlan, focusSave, focusCancel}
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
	case focusDesc:
		f.desc, cmd = f.desc.Update(msg)
	case focusPlan:
		f.plan, cmd = f.plan.Update(msg)
	}
	return f, cmd
}

// values returns the trimmed title, the description and the trimmed plan entered.
func (f form) values() (title, desc, plan string) {
	return strings.TrimSpace(f.title.Value()), strings.Trim(f.desc.Value(), "\n"), strings.TrimSpace(f.plan.Value())
}

// changes names the fields whose values differ from the ones the form opened
// with (a category form has the title only), so saving an edit leaves alone a
// field someone else changed while the form was open.
func (f form) changes() todo.Changes {
	title, desc, plan := f.values()
	var c todo.Changes
	if title != f.start[0] {
		c.Title = &title
	}
	if f.isTask && desc != f.start[1] {
		c.Description = &desc
	}
	if f.isTask && plan != f.start[2] {
		c.Plan = &plan
	}
	return c
}

func (f form) view() string {
	title := "Add task"
	switch {
	case !f.isTask && f.editing:
		title = "Rename category"
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
	b.WriteString(fieldLabel("Title", f.focus == focusTitle))
	b.WriteString("\n")
	b.WriteString(f.underline(f.title.View(), f.focus == focusTitle))
	if f.isTask {
		b.WriteString("\n\n")
		b.WriteString(fieldLabel("Description", f.focus == focusDesc))
		b.WriteString("\n")
		b.WriteString(f.descBox())
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
