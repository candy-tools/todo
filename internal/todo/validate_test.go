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
			Version:    3,
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
