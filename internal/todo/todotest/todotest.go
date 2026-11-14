// Package todotest helps tests seed a store from markdown and read it back.
package todotest

import (
	"context"
	"os"
	"path/filepath"
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

// Render returns doc as canonical markdown, without the guide block or any
// preamble.
func Render(doc *todo.Document) string {
	cp := *doc
	cp.Preamble = ""
	return cp.Render()
}

// writeFile writes content to path.
func writeFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o600)
}
