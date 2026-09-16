package todo_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/candy-tools/todo/internal/todo"
)

const repoURL = "github.com/candy-tools/todo"

// guideSample is a representative managed guide block: it starts with the
// recognised marker and ends with the HTML-comment close. The exact prose is
// the app's to define; the parser only has to recognise and strip it.
const guideSample = `<!-- todo:guide — managed by todo; this block is rewritten on save
This is a todo list for https://github.com/candy-tools/todo.
Keep to the format below so todo can parse it.
-->`

func TestParseStripsManagedGuide(t *testing.T) {
	src := guideSample + "\n\n# Work\n\n- [ ] a\n"
	d := todo.Parse(src)
	if d.Preamble != "" {
		t.Errorf("managed guide must be stripped from the preamble, got:\n%q", d.Preamble)
	}
	if len(d.Roots) != 1 || d.Roots[0].Title != "Work" {
		t.Fatalf("expected the Work category to parse, got %d roots", len(d.Roots))
	}
}

func TestParseKeepsUserPreambleAfterGuide(t *testing.T) {
	src := guideSample + "\n\nMy own intro\n\n# Work\n\n- [ ] a\n"
	d := todo.Parse(src)
	if d.Preamble != "My own intro" {
		t.Errorf("user preamble after the guide must be kept verbatim, got %q", d.Preamble)
	}
}

// A user's own HTML comment (not the managed marker) is left untouched.
func TestParseLeavesForeignCommentAlone(t *testing.T) {
	src := "<!-- just my note -->\n\n# Work\n\n- [ ] a\n"
	d := todo.Parse(src)
	if !strings.Contains(d.Preamble, "just my note") {
		t.Errorf("a non-managed comment must be preserved as preamble, got %q", d.Preamble)
	}
}

func TestFileContentLeadsWithGuide(t *testing.T) {
	d := todo.Parse("# Work\n\n- [ ] a\n")
	fc := d.FileContent()
	if !strings.HasPrefix(fc, "<!-- todo:guide") {
		t.Errorf("file content must start with the managed guide, got:\n%s", fc)
	}
	if !strings.Contains(fc, repoURL) {
		t.Errorf("guide must reference the repo %q, got:\n%s", repoURL, fc)
	}
	if !strings.Contains(fc, "- [ ] a") {
		t.Errorf("file content must still contain the rendered task, got:\n%s", fc)
	}
}

func TestGuideDocumentsAllMarkers(t *testing.T) {
	// The guide teaches editors the format todo parses, so it must mention every
	// checkbox marker the parser accepts.
	fc := (&todo.Document{}).FileContent()
	for _, marker := range []string{"[ ]", "[x]", "[/]", "[>]"} {
		if !strings.Contains(fc, marker) {
			t.Errorf("the guide should document the %q marker, got:\n%s", marker, fc)
		}
	}
}

func TestGuideMentionsCLI(t *testing.T) {
	fc := (&todo.Document{}).FileContent()
	if !strings.Contains(fc, "todo --help") {
		t.Errorf("the guide should point agents at the CLI:\n%s", fc)
	}
}

func TestFileContentOnEmptyDocStillHasGuide(t *testing.T) {
	fc := (&todo.Document{}).FileContent()
	if !strings.Contains(fc, "<!-- todo:guide") || !strings.Contains(fc, repoURL) {
		t.Errorf("an empty document must still carry the guide, got:\n%s", fc)
	}
}

func TestSaveReplacesOldGuideNoDuplicate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "todo.md")
	old := "<!-- todo:guide OLD GUIDE TEXT that must be replaced\n-->\n\n# Work\n\n- [ ] a\n"
	if err := os.WriteFile(path, []byte(old), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	d, err := todo.Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if err := d.Save(path); err != nil {
		t.Fatalf("save: %v", err)
	}

	got := readTestFile(t, path)
	if n := strings.Count(got, "<!-- todo:guide"); n != 1 {
		t.Errorf("guide must appear exactly once after save, found %d:\n%s", n, got)
	}
	if strings.Contains(got, "OLD GUIDE TEXT") {
		t.Errorf("the stale guide text must be replaced, got:\n%s", got)
	}
	if !strings.Contains(got, repoURL) || !strings.Contains(got, "- [ ] a") {
		t.Errorf("save must keep the current guide and the content, got:\n%s", got)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "todo.md")
	d := todo.Parse("# Work\n\n- [ ] a\n  - [x] a1\n\n# Personal\n\n- [ ] b\n")
	if err := d.Save(path); err != nil {
		t.Fatalf("save: %v", err)
	}
	reloaded, err := todo.Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if treeString(reloaded) != treeString(d) {
		t.Errorf("tree changed across a save/load round trip:\n%s\n---\n%s", treeString(d), treeString(reloaded))
	}
	if reloaded.Preamble != "" {
		t.Errorf("the guide must not survive as preamble, got %q", reloaded.Preamble)
	}
}

// Save must land exactly the target file — no temporary artifact left behind by
// the atomic write-and-rename.
func TestSaveLeavesNoTempFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "todo.md")
	d := todo.Parse("# Work\n\n- [ ] a\n")
	if err := d.Save(path); err != nil {
		t.Fatalf("save: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	if len(names) != 1 || names[0] != "todo.md" {
		t.Errorf("save must leave exactly the target file, found %v", names)
	}
}

func readTestFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	return string(b)
}
