package tui

import (
	"strings"

	"github.com/candy-tools/todo/internal/todo"
	"github.com/charmbracelet/lipgloss"
)

// pickRow is one line of a state picker: a selectable state, or (header set) a
// non-selectable column heading that groups the states under it.
type pickRow struct {
	header string
	state  todo.Status
}

// picker is the state dialog behind "t" (pick one state for the task) and "f"
// (tick the states the tab shows). It lists states, optionally grouped under
// column headings; the cursor only ever rests on a state row.
type picker struct {
	title   string
	rows    []pickRow
	cursor  int
	multi   bool                 // tick several states (f) instead of choosing one (t)
	subtask bool                 // render glyphs as a subtask's (open/done only)
	checked map[todo.Status]bool // the ticked states, for multi
}

// newStatePicker builds the "t" dialog for task it: a subtask may only be open
// or done; a top-level task may take any state, grouped by column. The cursor
// starts on the task's current state.
func newStatePicker(it *todo.Item) picker {
	p := picker{title: "Set state", subtask: it.IsSubtask()}
	if p.subtask {
		p.rows = []pickRow{{state: todo.Open}, {state: todo.Done}}
	} else {
		for _, c := range todo.Columns() {
			p.rows = append(p.rows, pickRow{header: c.Title()})
			for _, st := range c.States() {
				p.rows = append(p.rows, pickRow{state: st})
			}
		}
	}
	p.cursor = p.firstState()
	p.focus(it.Status)
	return p
}

// focus moves the cursor onto state st, when the picker offers it.
func (p *picker) focus(st todo.Status) {
	for i, r := range p.rows {
		if r.header == "" && r.state == st {
			p.cursor = i
			return
		}
	}
}

// newFilterPicker builds the "f" dialog titled title over the states of cols,
// with those in current ticked: one column lists its states, several group
// them under column headings (the unified view).
func newFilterPicker(title string, cols []todo.Column, current map[todo.Status]bool) picker {
	p := picker{title: title, multi: true, checked: map[todo.Status]bool{}}
	for _, c := range cols {
		if len(cols) > 1 {
			p.rows = append(p.rows, pickRow{header: c.Title()})
		}
		for _, st := range c.States() {
			p.rows = append(p.rows, pickRow{state: st})
			if current[st] {
				p.checked[st] = true
			}
		}
	}
	p.cursor = p.firstState()
	return p
}

// firstState is the index of the first selectable row.
func (p picker) firstState() int {
	for i, r := range p.rows {
		if r.header == "" {
			return i
		}
	}
	return 0
}

// move steps the cursor by delta (±1) to the next state row, skipping headings
// and stopping at either end.
func (p *picker) move(delta int) {
	for i := p.cursor + delta; i >= 0 && i < len(p.rows); i += delta {
		if p.rows[i].header == "" {
			p.cursor = i
			return
		}
	}
}

// selected is the state under the cursor.
func (p picker) selected() todo.Status { return p.rows[p.cursor].state }

// toggle ticks or unticks the state under the cursor (multi only).
func (p *picker) toggle() {
	st := p.selected()
	if p.checked[st] {
		delete(p.checked, st)
	} else {
		p.checked[st] = true
	}
}

// view renders the dialog: heading rows, then one row per state with its glyph
// and label (a tick box first when multi), the cursor row accented.
func (p picker) view() string {
	var lines []string
	for i, r := range p.rows {
		if r.header != "" {
			if i > 0 {
				lines = append(lines, "")
			}
			lines = append(lines, categoryStyle.Render(r.header))
			continue
		}
		glyph, st := statusGlyph(r.state, p.subtask)
		box := ""
		if p.multi {
			box = "[ ] "
			if p.checked[r.state] {
				box = "[x] "
			}
		}
		if i == p.cursor {
			lines = append(lines, selectedRowStyle.Render("❯ "+box+glyph+" "+r.state.Label()))
		} else {
			lines = append(lines, "  "+box+st.Render(glyph)+" "+r.state.Label())
		}
	}
	sep := helpTextStyle.Render(" · ")
	help := hint("enter", "Set") + sep + hint("esc", "Cancel")
	if p.multi {
		help = hint("space", "Toggle") + sep + hint("enter", "Apply") + sep + hint("esc", "Cancel")
	}
	lines = append(lines, "", help)

	body := padBody(strings.Join(lines, "\n"), helpPad)
	width := max(lipgloss.Width(body)+2+helpPad, lipgloss.Width(p.title)+8)
	return titledBox(p.title, body, width, lipgloss.Height(body)+2, true)
}
