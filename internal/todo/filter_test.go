package todo_test

import (
	"testing"

	"github.com/candy-tools/todo/internal/todo"
)

// buildFilterTree returns a small document for the filter tests:
//
//	# Work
//	  - Task A
//	      - Explore catacombs
//	      - Buy supplies
//	  - Task B
//	# Home
//	  - Groceries
func buildFilterTree() (d *todo.Document, work, taskA, cata, buy, taskB, home, groc *todo.Item) {
	work = &todo.Item{Kind: todo.Category, Level: 1, Title: "Work"}
	taskA = newTask("Task A", "", todo.Open)
	cata = newTask("Explore catacombs", "", todo.Open)
	buy = newTask("Buy supplies", "", todo.Open)
	taskB = newTask("Task B", "", todo.Open)
	home = &todo.Item{Kind: todo.Category, Level: 1, Title: "Home"}
	groc = newTask("Groceries", "", todo.Open)

	work.AppendChild(taskA)
	taskA.AppendChild(cata)
	taskA.AppendChild(buy)
	work.AppendChild(taskB)
	home.AppendChild(groc)

	d = &todo.Document{}
	d.Roots = append(d.Roots, work)
	d.Roots = append(d.Roots, home)
	return
}

func TestMatchesTitleCaseInsensitive(t *testing.T) {
	it := newTask("Explore Catacombs", "", todo.Open)
	if !it.Matches("cat") {
		t.Errorf("lower-case query should match a mixed-case title")
	}
	if !it.Matches("CATA") {
		t.Errorf("upper-case query should match a mixed-case title")
	}
	if it.Matches("dog") {
		t.Errorf("a query not present should not match")
	}
}

func TestMatchesDescription(t *testing.T) {
	it := newTask("Groceries", "remember the cat food", todo.Open)
	if !it.Matches("cat food") {
		t.Errorf("a query in the description should match a task")
	}
}

// An agent prints raw ids, so searching one finds the item. Only the whole id
// matches: ids are short random strings, and a partial query hitting them
// would drag unrelated items into every search.
func TestMatchesWholeID(t *testing.T) {
	task := newTask("Groceries", "", todo.Open)
	task.ID = "xsjawph"
	cat := &todo.Item{Kind: todo.Category, Level: 1, Title: "Home", ID: "fhsbxnt"}

	for _, q := range []string{"xsjawph", "XSJAWPH", " xsjawph "} {
		if !task.Matches(q) {
			t.Errorf("task: query %q should match its id", q)
		}
	}
	if !cat.Matches("fhsbxnt") {
		t.Errorf("category: a query equal to its id should match")
	}
	for _, q := range []string{"xsj", "jawp", "x"} {
		if task.Matches(q) {
			t.Errorf("task: partial id %q should not match", q)
		}
	}
	unsaved := newTask("Draft", "", todo.Open)
	if unsaved.Matches(" ") {
		t.Errorf("a blank query should not match an item without an id")
	}
}

func TestVisibleItemsEmptyQueryReturnsNil(t *testing.T) {
	d, _, _, _, _, _, _, _ := buildFilterTree()
	if d.VisibleItems("") != nil {
		t.Errorf("an empty query means no filter and should return nil")
	}
	if d.VisibleItems("   ") != nil {
		t.Errorf("a blank query should return nil")
	}
}

func TestVisibleItemsKeepsPathAndPrunesSiblings(t *testing.T) {
	d, work, taskA, cata, buy, taskB, home, groc := buildFilterTree()
	vis := d.VisibleItems("cat")

	for _, it := range []*todo.Item{work, taskA, cata} {
		if !vis[it] {
			t.Errorf("%q should be visible (a match or on the path to one)", it.Title)
		}
	}
	for _, it := range []*todo.Item{buy, taskB, home, groc} {
		if vis[it] {
			t.Errorf("%q should be hidden (not on a match path)", it.Title)
		}
	}
	if len(vis) != 3 {
		t.Errorf("want 3 visible items, got %d", len(vis))
	}
}

func TestVisibleItemsKeepsDescendantsOfMatch(t *testing.T) {
	d, _, _, cata, buy, _, _, _ := buildFilterTree()
	// "Task" matches Task A (and Task B); catacombs/buy do not match it, but they
	// sit under the matching Task A and so must stay visible.
	vis := d.VisibleItems("Task")
	for _, it := range []*todo.Item{cata, buy} {
		if !vis[it] {
			t.Errorf("%q should be visible as a descendant of a match", it.Title)
		}
	}
}

func TestVisibleItemsNoMatch(t *testing.T) {
	d, _, _, _, _, _, _, _ := buildFilterTree()
	if got := len(d.VisibleItems("zzz")); got != 0 {
		t.Errorf("a query that matches nothing should yield no visible items, got %d", got)
	}
}

func TestVisibleItemsMatchesDescription(t *testing.T) {
	work := &todo.Item{Kind: todo.Category, Level: 1, Title: "Work"}
	taskA := newTask("Task A", "", todo.Open)
	buy := newTask("Buy supplies", "need a flashlight", todo.Open)
	work.AppendChild(taskA)
	taskA.AppendChild(buy)
	d := &todo.Document{}
	d.Roots = append(d.Roots, work)

	vis := d.VisibleItems("flashlight")
	if !vis[buy] {
		t.Errorf("an item whose description matches should be visible")
	}
	if !vis[work] || !vis[taskA] {
		t.Errorf("the ancestors of a description match should be visible")
	}
}
