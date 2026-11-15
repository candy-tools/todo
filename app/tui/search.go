package tui

import (
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// openSearch focuses the "/" search bar. It seeds the input with the current
// filter so pressing "/" again refines the existing query instead of starting
// over; the filtered view stays put while typing.
func (m model) openSearch() (tea.Model, tea.Cmd) {
	ti := textinput.New()
	ti.Prompt = ""
	ti.CharLimit = 100
	ti.Width = m.searchWidth()
	ti.SetValue(m.view.filter)
	ti.CursorEnd()
	m.search = ti
	m.searching = true
	return m, tea.Batch(m.search.Focus(), textinput.Blink)
}

// updateSearch handles keys while the search bar is focused: esc cancels (and
// clears the filter), enter confirms (keeps the filter, returns to navigation),
// up/down (and PgUp/PgDn) move the selection through the results, and everything
// else edits the query and re-filters live.
func (m model) updateSearch(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.clearFilter()
		return m, nil
	case "enter":
		m.searching = false
		m.search.Blur()
		return m, nil
	case "up":
		m.focused().moveUp()
		return m, nil
	case "down":
		m.focused().moveDown()
		return m, nil
	case "pgup":
		m.focused().pageUp()
		return m, nil
	case "pgdown":
		m.focused().pageDown()
		return m, nil
	}
	var cmd tea.Cmd
	m.search, cmd = m.search.Update(msg)
	m.setFilter(m.search.Value())
	return m, cmd
}

// clearFilter closes the search bar and drops the active filter, restoring the
// full tree (and its collapse state). The item under the cursor stays selected,
// so returning from a search leaves you on the item you navigated to rather than
// back at the top.
func (m *model) clearFilter() {
	m.searching = false
	sel := m.focused().selected()
	m.setFilter("")
	if sel != nil {
		m.focused().revealItem(sel)
	}
}

// searchWidth sizes the search input to the footer width.
func (m model) searchWidth() int {
	w := m.width - 6
	if w < 10 {
		w = 10
	}
	return w
}
