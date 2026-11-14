// Package todotest helps tests seed a store from markdown and read it back.
package todotest

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/candy-tools/todo/internal/todo"
)

// Seed imports md into a fresh store at <t.TempDir()>/.todo.db and returns its
// path. It fails the test on an import error or any import warning.
func Seed(t testing.TB, md string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), ".todo.db")
	mdPath := filepath.Join(t.TempDir(), "seed.md")
	if err := writeFile(mdPath, md); err != nil {
		t.Fatalf("todotest.Seed: %v", err)
	}
	s, rep, err := todo.ImportFile(path, mdPath, false)
	if err != nil {
		t.Fatalf("todotest.Seed: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("todotest.Seed: close: %v", err)
	}
	if len(rep.Warnings) > 0 {
		t.Fatalf("todotest.Seed: import warnings: %q", rep.Warnings)
	}
	return path
}

// Dump opens the store at dbPath and returns its content as canonical markdown.
func Dump(t testing.TB, dbPath string) string {
	t.Helper()
	s, err := todo.OpenStore(dbPath)
	if err != nil {
		t.Fatalf("todotest.Dump: %v", err)
	}
	defer func() { _ = s.Close() }()
	doc, _, err := s.Load(context.Background())
	if err != nil {
		t.Fatalf("todotest.Dump: %v", err)
	}
	return Render(doc)
}

// Parse turns md into a Document through todo.ImportMarkdown, failing the test
// on an error. Tasks outside every heading land in an Uncategorized category,
// and import warnings are ignored.
func Parse(t testing.TB, md string) *todo.Document {
	t.Helper()
	doc, _, err := todo.ImportMarkdown([]byte(md))
	if err != nil {
		t.Fatalf("todotest.Parse: %v", err)
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
	for _, it := range items {
		if it.Kind == todo.Category {
			lines = append(lines, "", strings.Repeat("#", it.Level)+" "+it.Title, "")
			lines = appendItems(lines, it.Children, 0)
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
