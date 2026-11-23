package todo_test

import (
	"errors"
	"testing"

	"github.com/candy-tools/todo/internal/todo"
)

// sprintRef returns the reference of sprint id.
func sprintRef(t *testing.T, d *todo.Document, id string) string {
	t.Helper()
	s, ok := d.SprintByID(id)
	if !ok {
		t.Fatalf("no sprint %s", id)
	}
	return s.Ref
}

func TestAddSprintGetsRefFromFolder(t *testing.T) {
	d := &todo.Document{FolderPrefix: "TODO"}
	a := must(d.AddSprint("A", ""))
	b := must(d.AddSprint("B", ""))
	if got := sprintRef(t, d, a); got != "TODO-001" {
		t.Errorf("first sprint ref = %q, want TODO-001", got)
	}
	if got := sprintRef(t, d, b); got != "TODO-002" {
		t.Errorf("second sprint ref = %q, want TODO-002", got)
	}
}

func TestSprintPrefixSettingWinsOverFolder(t *testing.T) {
	d := &todo.Document{FolderPrefix: "TODO"}
	if _, err := d.UpdateSettings(todo.Settings{SprintPrefix: new(" sp ")}); err != nil {
		t.Fatal(err)
	}
	if d.SprintPrefix != "SP" {
		t.Errorf("SprintPrefix = %q, want SP", d.SprintPrefix)
	}
	if got := sprintRef(t, d, must(d.AddSprint("A", ""))); got != "SP-001" {
		t.Errorf("ref = %q, want SP-001", got)
	}
}

func TestSprintPrefixWithoutFolder(t *testing.T) {
	d := &todo.Document{}
	if got := sprintRef(t, d, must(d.AddSprint("A", ""))); got != "SPRINT-001" {
		t.Errorf("ref = %q, want SPRINT-001", got)
	}
}

func TestSprintRefNumberNotReused(t *testing.T) {
	d := &todo.Document{FolderPrefix: "TODO"}
	a := must(d.AddSprint("A", ""))
	if err := d.DeleteSprint(a); err != nil {
		t.Fatal(err)
	}
	if got := sprintRef(t, d, must(d.AddSprint("B", ""))); got != "TODO-002" {
		t.Errorf("ref after delete = %q, want TODO-002", got)
	}
}

func TestSprintPrefixChangeKeepsRefs(t *testing.T) {
	d := &todo.Document{FolderPrefix: "TODO"}
	a := must(d.AddSprint("A", ""))
	if _, err := d.UpdateSettings(todo.Settings{SprintPrefix: new("SP")}); err != nil {
		t.Fatal(err)
	}
	b := must(d.AddSprint("B", ""))
	if got := sprintRef(t, d, a); got != "TODO-001" {
		t.Errorf("old sprint ref = %q, want TODO-001", got)
	}
	if got := sprintRef(t, d, b); got != "SP-001" {
		t.Errorf("new sprint ref = %q, want SP-001", got)
	}
}

func TestSprintNamesMayRepeat(t *testing.T) {
	d := &todo.Document{}
	must(d.AddSprint("Sprint", ""))
	b, err := d.AddSprint("sprint", "")
	if err != nil {
		t.Fatalf("second sprint with the same name: %v", err)
	}
	c := must(d.AddSprint("Other", ""))
	if _, err := d.EditSprint(c, todo.SprintChanges{Name: new("SPRINT")}); err != nil {
		t.Fatalf("rename to a taken name: %v", err)
	}
	if s, _ := d.SprintByID(b); s.Name != "sprint" {
		t.Errorf("name = %q", s.Name)
	}
}

func TestResolveSprint(t *testing.T) {
	d := &todo.Document{FolderPrefix: "TODO"}
	a := must(d.AddSprint("Same", ""))
	b := must(d.AddSprint("same", ""))
	c := must(d.AddSprint("Unique", ""))
	ids := func(ss []*todo.Sprint) []string {
		var out []string
		for _, s := range ss {
			out = append(out, s.ID)
		}
		return out
	}
	for _, tc := range []struct {
		key  string
		want []string
	}{
		{a, []string{a}},
		{"todo-002", []string{b}},
		{"TODO-003", []string{c}},
		{"unique", []string{c}},
		{"SAME", []string{a, b}},
		{"nope", nil},
	} {
		got := ids(d.ResolveSprint(tc.key))
		if len(got) != len(tc.want) {
			t.Errorf("ResolveSprint(%q) = %v, want %v", tc.key, got, tc.want)
			continue
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("ResolveSprint(%q) = %v, want %v", tc.key, got, tc.want)
			}
		}
	}
}

func TestResolveSprintRefBeforeName(t *testing.T) {
	d := &todo.Document{FolderPrefix: "TODO"}
	a := must(d.AddSprint("A", ""))
	must(d.AddSprint("TODO-001", ""))
	if got := d.ResolveSprint("TODO-001"); len(got) != 1 || got[0].ID != a {
		t.Errorf("ResolveSprint(ref) = %v, want only sprint %s", got, a)
	}
}

func TestSprintPrefixClashRefused(t *testing.T) {
	d := &todo.Document{}
	must(d.SetRefPrefix("TODO"))
	must(d.AppendCategoryWith(todo.Fields{Title: "Work", Prefix: "WORK"}, ""))
	for _, p := range []string{"todo", "WORK"} {
		if _, err := d.UpdateSettings(todo.Settings{SprintPrefix: new(p)}); !errors.Is(err, todo.ErrPrefixClash) {
			t.Errorf("sprint prefix %q: err = %v, want ErrPrefixClash", p, err)
		}
	}
	if d.SprintPrefix != "" {
		t.Errorf("SprintPrefix = %q after refusals", d.SprintPrefix)
	}
}

func TestTaskPrefixClashWithSprintPrefixRefused(t *testing.T) {
	d := &todo.Document{}
	if _, err := d.UpdateSettings(todo.Settings{SprintPrefix: new("SP")}); err != nil {
		t.Fatal(err)
	}
	if _, err := d.SetRefPrefix("sp"); !errors.Is(err, todo.ErrPrefixClash) {
		t.Errorf("list prefix: err = %v, want ErrPrefixClash", err)
	}
	if _, err := d.AppendCategoryWith(todo.Fields{Title: "C", Prefix: "SP"}, ""); !errors.Is(err, todo.ErrPrefixClash) {
		t.Errorf("new category prefix: err = %v, want ErrPrefixClash", err)
	}
	cat := must(d.AppendCategory("D", ""))
	if _, err := d.Edit(cat, todo.Changes{Prefix: new("sp")}); !errors.Is(err, todo.ErrPrefixClash) {
		t.Errorf("edited category prefix: err = %v, want ErrPrefixClash", err)
	}
}

func TestUpdateSettingsSwapsPrefixes(t *testing.T) {
	d := &todo.Document{}
	if _, err := d.UpdateSettings(todo.Settings{RefPrefix: new("A"), SprintPrefix: new("B")}); err != nil {
		t.Fatal(err)
	}
	changed, err := d.UpdateSettings(todo.Settings{RefPrefix: new("B"), SprintPrefix: new("A")})
	if err != nil || !changed {
		t.Fatalf("swap = %v, %v", changed, err)
	}
	if d.RefPrefix != "B" || d.SprintPrefix != "A" {
		t.Errorf("prefixes = %q, %q", d.RefPrefix, d.SprintPrefix)
	}
}

func TestUpdateSettingsRefusedChangesNothing(t *testing.T) {
	d := &todo.Document{}
	must(d.AppendCategoryWith(todo.Fields{Title: "Work", Prefix: "WORK"}, ""))
	if _, err := d.UpdateSettings(todo.Settings{RefPrefix: new("TODO"), SprintPrefix: new("WORK")}); !errors.Is(err, todo.ErrPrefixClash) {
		t.Fatalf("err = %v, want ErrPrefixClash", err)
	}
	if d.RefPrefix != "" {
		t.Errorf("RefPrefix = %q after a refused update", d.RefPrefix)
	}
}

func TestFolderFallbackShiftsOnClash(t *testing.T) {
	d := &todo.Document{FolderPrefix: "TODO"}
	must(d.SetRefPrefix("TODO"))
	if got := d.SprintRefPrefix(); got != "TODOS" {
		t.Errorf("SprintRefPrefix = %q, want TODOS", got)
	}
	if got := sprintRef(t, d, must(d.AddSprint("A", ""))); got != "TODOS-001" {
		t.Errorf("ref = %q, want TODOS-001", got)
	}
}

func TestFolderPrefix(t *testing.T) {
	for _, tc := range []struct{ path, want string }{
		{"/home/me/todo/.todo.json", "TODO"},
		{"/src/candy-tools/.todo.json", "CANDYTOOLS"},
		{"/src/42things/.todo.json", "THINGS"},
		{"/src/averyverylongname/.todo.json", "AVERYVERYL"},
		{"/src/ñandú/.todo.json", "AND"},
		{"/.todo.json", ""},
	} {
		if got := todo.FolderPrefix(tc.path); got != tc.want {
			t.Errorf("FolderPrefix(%q) = %q, want %q", tc.path, got, tc.want)
		}
	}
}

func TestPrefixForIgnoresSprint(t *testing.T) {
	d := &todo.Document{FolderPrefix: "TODO"}
	work := must(d.AppendCategoryWith(todo.Fields{Title: "Work", Prefix: "WORK"}, ""))
	sp := must(d.AddSprint("S", ""))
	task := must(d.AppendTask(todo.Fields{Title: "t", Sprint: sp}, todo.New, work))
	if got := refOf(t, d, task); got != "WORK-001" {
		t.Errorf("task in a sprint ref = %q, want WORK-001", got)
	}
}

func TestCloneCopiesSprintSettings(t *testing.T) {
	d := &todo.Document{FolderPrefix: "TODO", SprintPrefix: "SP"}
	must(d.AddSprint("A", ""))
	c := d.Clone()
	if c.FolderPrefix != "TODO" || c.SprintPrefix != "SP" || c.Sprints[0].Ref != "SP-001" {
		t.Errorf("clone = %q, %q, %q", c.FolderPrefix, c.SprintPrefix, c.Sprints[0].Ref)
	}
}

func TestTaskRefMayNotBeASprintRef(t *testing.T) {
	d := &todo.Document{FolderPrefix: "TODO"}
	must(d.AddSprint("S", ""))
	cat := must(d.AppendCategory("C", ""))
	if _, err := d.AppendTask(todo.Fields{Title: "t", Ref: "todo-001"}, todo.Open, cat); !errors.Is(err, todo.ErrDuplicateRef) {
		t.Errorf("new task err = %v, want ErrDuplicateRef", err)
	}
	task := must(d.AppendTask(todo.Fields{Title: "u"}, todo.Open, cat))
	if _, err := d.Edit(task, todo.Changes{Ref: new("TODO-001")}); !errors.Is(err, todo.ErrDuplicateRef) {
		t.Errorf("edited task err = %v, want ErrDuplicateRef", err)
	}
}
