package cmd

import (
	"encoding/json"
	"io"

	"github.com/candy-tools/todo/internal/todo"
)

// record is the one JSON shape for a single item, shared by every command that
// prints items as JSON (show, list --flat, every list --json node, next,
// mutations).
type record struct {
	Number      int      `json:"number"`
	ID          string   `json:"id"`
	Kind        string   `json:"kind"` // task | category
	Ref         string   `json:"ref,omitempty"`
	Prefix      string   `json:"prefix,omitempty"` // category only: its reference prefix
	Title       string   `json:"title"`
	Status      string   `json:"status,omitempty"`
	Column      string   `json:"column,omitempty"`
	Sprint      string   `json:"sprint,omitempty"`    // the sprint's id; a subtask shows its parent's
	SprintRef   string   `json:"sprintRef,omitempty"` // the sprint's reference (TODO-001)
	SprintName  string   `json:"sprintName,omitempty"`
	Path        []string `json:"path"`
	ParentID    string   `json:"parentId,omitempty"`
	ParentTitle string   `json:"parentTitle,omitempty"`
	Level       int      `json:"level,omitempty"` // category only
	Description string   `json:"description,omitempty"`
	Plan        string   `json:"plan,omitempty"`
	DoneCount   int      `json:"doneCount"`
	TotalCount  int      `json:"totalCount"`
	Subtasks    []brief  `json:"subtasks,omitempty"`
}

// brief is one direct child task of a record.
type brief struct {
	Number int    `json:"number"`
	ID     string `json:"id"`
	Title  string `json:"title"`
	Status string `json:"status"`
}

// toRecord builds the record for it, without its subtasks. nums supplies the
// list numbers.
func toRecord(doc *todo.Document, it *todo.Item, nums map[*todo.Item]int) record {
	r := record{
		Number: nums[it], ID: it.ID, Title: it.Title, Path: pathOf(it),
	}
	r.DoneCount, r.TotalCount = it.TaskCounts()
	if it.IsTask() {
		r.Kind = "task"
		r.Ref = it.Ref
		r.Status = it.Status.String()
		r.Column = it.Column().String()
		r.Sprint = it.SprintID()
		if sp, ok := doc.SprintByID(r.Sprint); ok {
			r.SprintRef, r.SprintName = sp.Ref, sp.Name
		}
		r.Description = it.Description
		r.Plan = it.Plan
	} else {
		r.Kind = "category"
		r.Level = it.Level
		r.Prefix = it.Prefix
	}
	if it.Parent != nil {
		r.ParentID = it.Parent.ID
		r.ParentTitle = it.Parent.Title
	}
	return r
}

// toDetail is toRecord plus, for a task, one brief per direct child task in
// tree order (a category has none).
func toDetail(doc *todo.Document, it *todo.Item, nums map[*todo.Item]int) record {
	r := toRecord(doc, it, nums)
	if !it.IsTask() {
		return r
	}
	for _, c := range it.Children {
		if c.IsTask() {
			r.Subtasks = append(r.Subtasks, brief{Number: nums[c], ID: c.ID, Title: c.Title, Status: c.Status.String()})
		}
	}
	return r
}

// writeJSON writes v to w as indented JSON.
func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
