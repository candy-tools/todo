package tui

import (
	"strings"
	"testing"

	"github.com/candy-tools/todo/internal/todo"
	"github.com/charmbracelet/lipgloss"
)

// rowTitles returns the titles of the real (non-placeholder) rows, in order.
func rowTitles(tr tree) []string {
	var out []string
	for _, r := range tr.rows {
		if r.placeholder {
			continue
		}
		out = append(out, r.item.Title)
	}
	return out
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

func TestHighlightWrapsMatch(t *testing.T) {
	base := lipgloss.NewStyle()
	got := highlight("Explore catacombs", "cat", base)
	if !strings.Contains(got, matchStyle.Render("cat")) {
		t.Errorf("the match should be wrapped in matchStyle, got %q", got)
	}
}

func TestHighlightNoQueryIsPlain(t *testing.T) {
	base := lipgloss.NewStyle()
	if got := highlight("abc", "", base); got != base.Render("abc") {
		t.Errorf("an empty query should render the text unchanged, got %q", got)
	}
}

func TestHighlightPreservesOriginalCase(t *testing.T) {
	got := highlight("Catacombs", "cat", lipgloss.NewStyle())
	if !strings.Contains(got, matchStyle.Render("Cat")) {
		t.Errorf("highlighting should keep the matched text's original case, got %q", got)
	}
}

func TestTreeFilterHidesNonMatches(t *testing.T) {
	d := todo.Parse("# Work\n\n- [ ] Task A\n  - [ ] Explore catacombs\n  - [ ] Buy supplies\n- [ ] Task B\n\n# Home\n\n- [ ] Groceries\n")
	tr := newTree(d)
	tr.setFilter("cat")
	got := rowTitles(tr)
	want := []string{"Work", "Task A", "Explore catacombs"}
	if !equalStrings(got, want) {
		t.Errorf("filtered rows = %v, want %v", got, want)
	}
}

func TestTreeFilterShowsMatchInsideCollapsedParent(t *testing.T) {
	d := todo.Parse("# Work\n\n- [ ] Task A\n  - [ ] Explore catacombs\n")
	tr := newTree(d)
	tr.collapsed[find(d, "Task A")] = true // collapse the parent of the match
	tr.setFilter("cat")
	if got := rowTitles(tr); !contains(got, "Explore catacombs") {
		t.Errorf("filtering should reveal matches inside a collapsed parent, got %v", got)
	}
}

func TestTreeFilterHidesPlaceholder(t *testing.T) {
	d := todo.Parse("# Work\n\n- [ ] catnip\n")
	tr := newTree(d)
	tr.setFilter("cat")
	for _, r := range tr.rows {
		if r.placeholder {
			t.Errorf("the + new category placeholder should be hidden while filtering")
		}
	}
}

func TestTreeClearFilterRestoresTree(t *testing.T) {
	d := todo.Parse("# Work\n\n- [ ] Task A\n  - [ ] Explore catacombs\n  - [ ] Buy supplies\n")
	tr := newTree(d)
	before := len(tr.rows)
	tr.setFilter("cat")
	tr.setFilter("") // clear
	if len(tr.rows) != before {
		t.Errorf("clearing the filter should restore all rows: got %d, want %d", len(tr.rows), before)
	}
}

func TestTreeRowHighlightsMatch(t *testing.T) {
	d := todo.Parse("# Work\n\n- [ ] Explore catacombs\n")
	tr := newTree(d)
	tr.setFilter("cat")
	got := tr.rowString(treeRow{item: find(d, "Explore catacombs")}, false)
	if !strings.Contains(got, matchStyle.Render("cat")) {
		t.Errorf("a filtered row should highlight the matched substring, got %q", got)
	}
}
