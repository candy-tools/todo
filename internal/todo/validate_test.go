package todo_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/candy-tools/todo/internal/todo"
)

func TestValidate(t *testing.T) {
	const a, b, c, d = "aaaaaaa", "bbbbbbb", "ccccccc", "ddddddd"
	good := func() todo.DiskList {
		return todo.DiskList{
			Version:    9,
			Categories: []todo.DiskCategory{{ID: a, Title: "A"}},
			Tasks: []todo.DiskTask{{ID: b, Category: a, Title: "T", Status: "open",
				Subtasks: []todo.DiskSubtask{{ID: c, Title: "S"}}}},
		}
	}
	cases := []struct {
		name string
		mut  func(l *todo.DiskList)
		want string // "" means valid
	}{
		{"valid", func(*todo.DiskList) {}, ""},
		{"version zero", func(l *todo.DiskList) { l.Version = 0 }, "invalid list file: unsupported format version 0"},
		{"version negative", func(l *todo.DiskList) { l.Version = -2 }, "invalid list file: unsupported format version -2"},
		{"version older", func(l *todo.DiskList) { l.Version = 2 }, "invalid list file: unsupported format version 2"},
		{"malformed category id", func(l *todo.DiskList) { l.Categories[0].ID = "ABC" }, `invalid list file: category "A": malformed id "ABC": an id is 7 characters from 0-9 and a-z except i, l, o and u`},
		{"malformed task id", func(l *todo.DiskList) { l.Tasks[0].ID = "" }, `invalid list file: task "T": malformed id "": an id is 7 characters from 0-9 and a-z except i, l, o and u`},
		{"malformed subtask id", func(l *todo.DiskList) { l.Tasks[0].Subtasks[0].ID = "zzzzzzzz" }, `invalid list file: subtask "S" of task "T": malformed id "zzzzzzzz": an id is 7 characters from 0-9 and a-z except i, l, o and u`},
		{"malformed id of an untitled category", func(l *todo.DiskList) { l.Categories[0].ID, l.Categories[0].Title = "", "" },
			`invalid list file: categories[0]: malformed id "": an id is 7 characters from 0-9 and a-z except i, l, o and u`},
		{"malformed id of an untitled task", func(l *todo.DiskList) { l.Tasks[0].ID, l.Tasks[0].Title = "x", "" },
			`invalid list file: tasks[0]: malformed id "x": an id is 7 characters from 0-9 and a-z except i, l, o and u`},
		{"malformed id of an untitled subtask", func(l *todo.DiskList) { l.Tasks[0].Subtasks[0].ID, l.Tasks[0].Subtasks[0].Title = "x", "" },
			`invalid list file: tasks[0].subtasks[0]: malformed id "x": an id is 7 characters from 0-9 and a-z except i, l, o and u`},
		{"malformed subtask id under an untitled task", func(l *todo.DiskList) { l.Tasks[0].Subtasks[0].ID, l.Tasks[0].Title = "x", "" },
			`invalid list file: subtask "S" of tasks[0]: malformed id "x": an id is 7 characters from 0-9 and a-z except i, l, o and u`},
		{"duplicate id", func(l *todo.DiskList) { l.Tasks[0].ID = a }, "invalid list file: duplicate id " + a},
		{"duplicate subtask id", func(l *todo.DiskList) { l.Tasks[0].Subtasks[0].ID = b }, "invalid list file: duplicate id " + b},
		{"missing parent", func(l *todo.DiskList) { l.Categories[0].Parent = d }, "invalid list file: category " + a + `: parent "` + d + `" does not exist`},
		{"self parent cycle", func(l *todo.DiskList) { l.Categories[0].Parent = a }, "invalid list file: category " + a + ": parent cycle"},
		{"two category cycle", func(l *todo.DiskList) {
			l.Categories[0].Parent = d
			l.Categories = append(l.Categories, todo.DiskCategory{ID: d, Title: "D", Parent: a})
		}, "invalid list file: category " + a + ": parent cycle"},
		{"missing task category", func(l *todo.DiskList) { l.Tasks[0].Category = d }, "invalid list file: task " + b + `: category "` + d + `" does not exist`},
		{"empty task category", func(l *todo.DiskList) { l.Tasks[0].Category = "" }, "invalid list file: task " + b + `: category "" does not exist`},
		{"unknown status", func(l *todo.DiskList) { l.Tasks[0].Status = "bogus" }, "invalid list file: task " + b + `: unknown status "bogus"`},
		{"alias status", func(l *todo.DiskList) { l.Tasks[0].Status = "progress" }, "invalid list file: task " + b + `: unknown status "progress"`},
		{"category empty title", func(l *todo.DiskList) { l.Categories[0].Title = " " }, "invalid list file: category " + a + ": invalid title: it must not be empty"},
		{"task newline title", func(l *todo.DiskList) { l.Tasks[0].Title = "x\ny" },
			"invalid list file: task " + b + ": invalid title: it must not hold control characters such as newlines, tabs, escapes or bidi overrides"},
		{"subtask long title", func(l *todo.DiskList) { l.Tasks[0].Subtasks[0].Title = strings.Repeat("x", 201) },
			"invalid list file: subtask " + c + ": invalid title: it must be at most 200 characters"},
		{"task untrimmed title", func(l *todo.DiskList) { l.Tasks[0].Title = " T" }, "invalid list file: task " + b + ": title has leading or trailing whitespace"},
		{"category untrimmed title", func(l *todo.DiskList) { l.Categories[0].Title = "A " }, "invalid list file: category " + a + ": title has leading or trailing whitespace"},
		{"subtask untrimmed title", func(l *todo.DiskList) { l.Tasks[0].Subtasks[0].Title = "S " }, "invalid list file: subtask " + c + ": title has leading or trailing whitespace"},
		{"unknown view screen", func(l *todo.DiskList) { l.View = &todo.DiskView{Screen: "x"} },
			`invalid list file: view: unknown screen "x"`},
		{"valid sprint", func(l *todo.DiskList) {
			l.Sprints = []todo.DiskSprint{{ID: d, Name: "S1"}}
			l.Tasks[0].Sprint, l.Tasks[0].Status, l.Tasks[0].Subtasks[0].Done = d, "done", true
		}, ""},
		{"sprint state without a sprint", func(l *todo.DiskList) { l.Tasks[0].Status = "working" },
			"invalid list file: task " + b + `: status "working" needs a sprint`},
		{"backlog state in a sprint", func(l *todo.DiskList) {
			l.Sprints = []todo.DiskSprint{{ID: d, Name: "S1"}}
			l.Tasks[0].Sprint, l.Tasks[0].Status = d, "deferred"
		}, "invalid list file: task " + b + `: backlog status "deferred" in sprint ` + d},
		// Up to version 8 a sprint had a stored status; it is read and ignored,
		// so it need not match the sprint's tasks.
		{"version 8 done sprint holding a task not done", func(l *todo.DiskList) {
			l.Sprints = []todo.DiskSprint{{ID: d, Name: "S1", Status: "done"}}
			l.Tasks[0].Sprint, l.Tasks[0].Status = d, "review"
		}, ""},
		{"malformed sprint id", func(l *todo.DiskList) { l.Sprints = []todo.DiskSprint{{ID: "BAD", Name: "S1"}} },
			`invalid list file: sprint "S1": malformed id "BAD": an id is 7 characters from 0-9 and a-z except i, l, o and u`},
		{"duplicate sprint id", func(l *todo.DiskList) {
			l.Sprints = []todo.DiskSprint{{ID: d, Name: "S1"}, {ID: d, Name: "S2"}}
		}, "invalid list file: duplicate sprint id " + d},
		{"empty sprint name", func(l *todo.DiskList) { l.Sprints = []todo.DiskSprint{{ID: d}} },
			"invalid list file: sprint " + d + ": invalid title: it must not be empty"},
		{"untrimmed sprint name", func(l *todo.DiskList) { l.Sprints = []todo.DiskSprint{{ID: d, Name: " S1"}} },
			"invalid list file: sprint " + d + ": name has leading or trailing whitespace"},
		{"sprint names may repeat", func(l *todo.DiskList) {
			l.Sprints = []todo.DiskSprint{{ID: d, Name: "Sprint"}, {ID: "eeeeeee", Name: "sPRINT"}}
		}, ""},
		{"untrimmed sprint reference", func(l *todo.DiskList) {
			l.Sprints = []todo.DiskSprint{{ID: d, Ref: " TODO-001", Name: "S"}}
		}, "invalid list file: sprint " + d + ": reference has leading or trailing whitespace"},
		{"duplicate sprint reference ignoring case", func(l *todo.DiskList) {
			l.Sprints = []todo.DiskSprint{{ID: d, Ref: "TODO-001", Name: "S"}, {ID: "eeeeeee", Ref: "todo-001", Name: "T"}}
		}, `invalid list file: sprint eeeeeee: reference "todo-001" is already used by sprint ` + d},
		{"sprint reference of a task", func(l *todo.DiskList) {
			l.Tasks[0].Ref = "TODO-001"
			l.Sprints = []todo.DiskSprint{{ID: d, Ref: "todo-001", Name: "S"}}
		}, `invalid list file: sprint ` + d + `: reference "todo-001" is already used by task ` + b},
		{"version 8 sprint status of any value ignored", func(l *todo.DiskList) { l.Sprints = []todo.DiskSprint{{ID: d, Name: "S1", Status: "bogus"}} },
			""},
		{"task in a missing sprint", func(l *todo.DiskList) { l.Tasks[0].Sprint = d },
			"invalid list file: task " + b + ": sprint \"" + d + "\" does not exist"},
		{"task with a reference", func(l *todo.DiskList) { l.Tasks[0].Ref = "APP-001" }, ""},
		{"untrimmed reference", func(l *todo.DiskList) { l.Tasks[0].Ref = "APP-001 " }, "invalid list file: task " + b + ": reference has leading or trailing whitespace"},
		{"reference with a newline", func(l *todo.DiskList) { l.Tasks[0].Ref = "A\nB" },
			"invalid list file: task " + b + ": invalid reference: it must not hold control characters such as newlines, tabs, escapes or bidi overrides"},
		{"long reference", func(l *todo.DiskList) { l.Tasks[0].Ref = strings.Repeat("x", 51) },
			"invalid list file: task " + b + ": invalid reference: it must be at most 50 characters"},
		{"duplicate reference ignoring case", func(l *todo.DiskList) {
			l.Tasks[0].Ref = "APP-001"
			l.Tasks = append(l.Tasks, todo.DiskTask{ID: d, Category: a, Ref: "app-001", Title: "U", Status: "open"})
		}, "invalid list file: task " + d + `: reference "app-001" is already used by task ` + b},
		{"valid refs", func(l *todo.DiskList) {
			l.Refs = &todo.DiskRefs{Prefix: "TODO", SprintPrefix: "SP", Last: map[string]int{"SP": 1, "TODO": 1, "W2": 5}}
			l.Categories[0].Prefix = "WORK"
			l.Sprints = []todo.DiskSprint{{ID: d, Ref: "SP-001", Name: "S1"}}
		}, ""},
		{"sprint prefix setting invalid", func(l *todo.DiskList) { l.Refs = &todo.DiskRefs{SprintPrefix: "sp"} },
			"invalid list file: refs: invalid reference prefix \"sp\""},
		{"sprint prefix setting is the list prefix", func(l *todo.DiskList) { l.Refs = &todo.DiskRefs{Prefix: "TODO", SprintPrefix: "TODO"} },
			"invalid list file: refs: reference prefix TODO is both the list's and the sprint reference prefix"},
		{"sprint prefix setting is a category prefix", func(l *todo.DiskList) {
			l.Refs = &todo.DiskRefs{SprintPrefix: "WORK"}
			l.Categories[0].Prefix = "WORK"
		}, "invalid list file: category " + a + ": reference prefix WORK is also the sprint reference prefix"},
		{"refs prefix lower case", func(l *todo.DiskList) { l.Refs = &todo.DiskRefs{Prefix: "todo"} },
			"invalid list file: refs: invalid reference prefix \"todo\""},
		{"refs counter key invalid", func(l *todo.DiskList) { l.Refs = &todo.DiskRefs{Last: map[string]int{"wo rk": 1}} },
			"invalid list file: refs: invalid reference prefix \"wo rk\""},
		{"refs counter zero", func(l *todo.DiskList) { l.Refs = &todo.DiskRefs{Last: map[string]int{"WORK": 0}} },
			"invalid list file: refs: counter for WORK must be at least 1"},
		{"category prefix invalid", func(l *todo.DiskList) { l.Categories[0].Prefix = "1A" },
			"invalid list file: category " + a + ": invalid reference prefix \"1A\""},
		{"version 7 sprint prefix ignored", func(l *todo.DiskList) {
			l.Sprints = []todo.DiskSprint{{ID: d, Name: "S1", Prefix: "x-"}}
		}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			l := good()
			tc.mut(&l)
			err := todo.ValidateDisk(&l)
			if tc.want == "" {
				if err != nil {
					t.Fatalf("err = %v, want nil", err)
				}
				return
			}
			if !errors.Is(err, todo.ErrInvalidList) {
				t.Fatalf("err = %v, want ErrInvalidList", err)
			}
			if err.Error() != tc.want {
				t.Errorf("err = %q\nwant  %q", err, tc.want)
			}
		})
	}
}
