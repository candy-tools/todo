// Package todotest helps tests seed a store from markdown and read it back.
package todotest

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gofrs/flock"

	"github.com/candy-tools/todo/internal/todo"
	"github.com/candy-tools/todo/libs/jsonfile"
)

// Seed imports md into a fresh list at <t.TempDir()>/.todo.json and returns
// its path. It fails the test on an import error or any import warning.
func Seed(t testing.TB, md string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), ".todo.json")
	mdPath := filepath.Join(t.TempDir(), "seed.md")
	if err := writeFile(mdPath, md); err != nil {
		t.Fatalf("todotest.Seed: %v", err)
	}
	_, rep, err := todo.ImportFile(path, mdPath, false)
	if err != nil {
		t.Fatalf("todotest.Seed: %v", err)
	}
	if len(rep.Warnings) > 0 {
		t.Fatalf("todotest.Seed: import warnings: %q", rep.Warnings)
	}
	return path
}

// SeedJSON writes json, a list file's content, as is to
// <t.TempDir()>/.todo.json and returns its path. It lets a test give kanban
// states, fixed IDs and plans directly, which markdown cannot express. It
// fails the test when the content does not load as a list.
func SeedJSON(t testing.TB, json string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), ".todo.json")
	if err := writeFile(path, json); err != nil {
		t.Fatalf("todotest.SeedJSON: %v", err)
	}
	if _, _, err := todo.NewStore(path).Load(context.Background()); err != nil {
		t.Fatalf("todotest.SeedJSON: %v", err)
	}
	return path
}

// Dump reads the list at path and returns its content as canonical markdown.
func Dump(t testing.TB, path string) string {
	t.Helper()
	doc, _, err := todo.NewStore(path).Load(context.Background())
	if err != nil {
		t.Fatalf("todotest.Dump: %v", err)
	}
	return Render(doc)
}

// HoldWriteLock takes the write lock of the list at path (the lock file
// jsonfile.LockPath names, beside the file a symlinked path points at),
// standing in for another process in the middle of a write. The lock is held
// until the test ends.
func HoldWriteLock(t testing.TB, path string) {
	t.Helper()
	fl := flock.New(jsonfile.LockPath(path))
	if err := fl.Lock(); err != nil {
		t.Fatalf("todotest.HoldWriteLock: %v", err)
	}
	t.Cleanup(func() { _ = fl.Unlock() })
}

// ImportedSprintID is the ID of the open sprint Parse puts the top-level tasks
// in a sprint state ([/], [x], ...) in, as an import into a list does.
const ImportedSprintID = "0000000"

// Parse turns md into a Document through todo.ImportMarkdown, failing the test
// on an error. Tasks outside every heading land in an Uncategorized category,
// and import warnings are ignored. As an import into a list, it puts every
// top-level task in a sprint state in an open sprint named
// todo.ImportedSprintName (ID ImportedSprintID), so the document keeps the
// membership rule the operations check.
func Parse(t testing.TB, md string) *todo.Document {
	t.Helper()
	doc, _, err := todo.ImportMarkdown([]byte(md))
	if err != nil {
		t.Fatalf("todotest.Parse: %v", err)
	}
	for _, it := range doc.Enumerate() {
		if !it.IsTask() || it.IsSubtask() || it.Sprint != "" || !it.Status.InSprint() {
			continue
		}
		if _, ok := doc.SprintByID(ImportedSprintID); !ok {
			doc.Sprints = append(doc.Sprints, &todo.Sprint{ID: ImportedSprintID, Name: todo.ImportedSprintName})
		}
		it.Sprint = ImportedSprintID
	}
	return doc
}

// Render returns doc as canonical markdown: each header with a blank line on
// either side, task lists tight, a task's description indented under its
// checkbox line before any sub-tasks, and a single trailing newline. An empty
// document renders as "".
func Render(doc *todo.Document) string {
	lines := collapseBlanks(appendItems(nil, doc.Roots, 0))
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n") + "\n"
}

// appendItems renders a sibling list. taskDepth is the task-nesting depth of a
// task in this list (0 directly under a category, +1 under each parent task).
// A blank line is emitted before every header; collapseBlanks later folds any
// doubled-up blanks and trims a leading one, so callers can be liberal here.
func appendItems(lines []string, items []*todo.Item, taskDepth int) []string {
	// Markdown puts a heading's tasks before its subheadings (a task after a
	// subheading belongs to it), so the tasks of a list are written first,
	// wherever the document puts them.
	var cats []*todo.Item
	for _, it := range items {
		if it.Kind == todo.Category {
			cats = append(cats, it)
			continue
		}
		indent := strings.Repeat("  ", taskDepth)
		// States without a checkbox marker render as their name: a test-only
		// form that is never parsed back.
		mark := it.Status.Marker()
		if mark == "" {
			mark = it.Status.String()
		}
		lines = append(lines, indent+"- ["+mark+"] "+it.Title)
		if it.Description != "" {
			for _, dl := range strings.Split(it.Description, "\n") {
				if dl == "" {
					lines = append(lines, "")
				} else {
					lines = append(lines, indent+"  "+dl)
				}
			}
		}
		lines = appendItems(lines, it.Children, taskDepth+1)
	}
	for _, it := range cats {
		lines = append(lines, "", strings.Repeat("#", it.Level)+" "+it.Title, "")
		lines = appendItems(lines, it.Children, 0)
	}
	return lines
}

// collapseBlanks removes a leading blank line and folds any run of consecutive
// blank lines down to one, so the header spacing rules above never produce
// doubled blanks.
func collapseBlanks(lines []string) []string {
	out := lines[:0:0]
	prevBlank := false
	for _, l := range lines {
		blank := strings.TrimSpace(l) == ""
		if blank && (prevBlank || len(out) == 0) {
			continue
		}
		out = append(out, l)
		prevBlank = blank
	}
	for len(out) > 0 && strings.TrimSpace(out[len(out)-1]) == "" {
		out = out[:len(out)-1]
	}
	return out
}

// writeFile writes content to path.
func writeFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o600)
}
