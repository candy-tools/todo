package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/candy-tools/todo/internal/todo/todotest"
)

func TestRootArgs(t *testing.T) {
	c := newRootCommand()
	if err := c.Args(c, nil); err != nil {
		t.Errorf("no argument should be valid (defaults to .todo.json): %v", err)
	}
	if err := c.Args(c, []string{"a.json"}); err != nil {
		t.Errorf("one argument should be valid: %v", err)
	}
	if err := c.Args(c, []string{"a.json", "b.json"}); err == nil {
		t.Error("expected an error with two arguments")
	}
}

func TestResolvePath(t *testing.T) {
	c := newRootCommand()
	if p, def, err := resolvePath(c, nil); err != nil || p != defaultFile || !def {
		t.Errorf("default = %q, %v, %v", p, def, err)
	}
	if p, def, err := resolvePath(c, []string{"custom.json"}); err != nil || p != "custom.json" || def {
		t.Errorf("arg = %q, %v, %v", p, def, err)
	}
	if err := c.ParseFlags([]string{"-f", "flag.json"}); err != nil {
		t.Fatal(err)
	}
	if p, def, err := resolvePath(c, nil); err != nil || p != "flag.json" || def {
		t.Errorf("flag = %q, %v, %v", p, def, err)
	}
	if p, _, err := resolvePath(c, []string{"arg.json"}); err != nil || p != "arg.json" {
		t.Errorf("arg should beat the flag: %q, %v", p, err)
	}
}

func TestRootMeta(t *testing.T) {
	c := newRootCommand()
	if c.Use != "todo [list-file]" || c.Short != "A minimalistic terminal TODO manager" {
		t.Errorf("unexpected Use/Short: %q / %q", c.Use, c.Short)
	}
}

func TestMarkdownPathIsUsageError(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.WriteFile("TODO.md", []byte("# A\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := runCmd(t, "list", "-f", "TODO.md")
	if codeOf(t, err) != exitUsage || err.Error() == "" || !contains(err.Error(), "todo import") {
		t.Errorf("want a usage error mentioning todo import, got %v", err)
	}
}

// runExecute runs execute and returns the exit code with stdout and stderr.
func runExecute(args ...string) (code int, stdout, stderr string) {
	var out, errb bytes.Buffer
	code = execute(args, &out, &errb)
	return code, out.String(), errb.String()
}

// decodeJSONError parses the single-line JSON error object from stderr.
func decodeJSONError(t *testing.T, stderr string) jsonError {
	t.Helper()
	if strings.Count(stderr, "\n") != 1 || !strings.HasSuffix(stderr, "\n") {
		t.Fatalf("want exactly one line on stderr, got %q", stderr)
	}
	var je jsonError
	if err := json.Unmarshal([]byte(stderr), &je); err != nil {
		t.Fatalf("stderr is not a JSON error: %v: %q", err, stderr)
	}
	return je
}

func TestExecuteJSONErrorNotFound(t *testing.T) {
	p := todotest.Seed(t, "# Work\n\n- [ ] a\n")
	code, stdout, stderr := runExecute("show", "--file", p, "--id", "zzzzzzz", "--json")
	if code != exitNotFound || stdout != "" {
		t.Fatalf("code=%d stdout=%q", code, stdout)
	}
	je := decodeJSONError(t, stderr)
	if je.Error.Code != "not_found" || je.Error.Exit != 3 || je.Error.Message != "no item with --id zzzzzzz" {
		t.Errorf("unexpected error object: %+v", je.Error)
	}
}

func TestExecuteJSONErrorAmbiguous(t *testing.T) {
	p := todotest.Seed(t, "# Work\n\n- [ ] x\n- [ ] x\n")
	code, _, stderr := runExecute("done", "--file", p, "--title", "x", "--json")
	if code != exitAmbiguous {
		t.Fatalf("code=%d", code)
	}
	je := decodeJSONError(t, stderr)
	if je.Error.Code != "ambiguous" || je.Error.Exit != 4 ||
		!strings.Contains(je.Error.Message, "--number 2") || !strings.Contains(je.Error.Message, "--number 3") {
		t.Errorf("unexpected error object: %+v", je.Error)
	}
}

func TestExecuteJSONErrorOnFlagError(t *testing.T) {
	p := todotest.Seed(t, "# Work\n\n- [ ] a\n")
	code, _, stderr := runExecute("list", "--file", p, "--json", "--bogus")
	if code != exitUsage {
		t.Fatalf("code=%d", code)
	}
	if je := decodeJSONError(t, stderr); je.Error.Code != "usage" || je.Error.Exit != 2 {
		t.Errorf("unexpected error object: %+v", je.Error)
	}
}

func TestExecuteJSONFalseGivesPlainError(t *testing.T) {
	p := todotest.Seed(t, "# Work\n\n- [ ] a\n")
	code, _, stderr := runExecute("show", "--file", p, "--id", "zzzzzzz", "--json=false")
	if code != exitNotFound || stderr != "no item with --id zzzzzzz\n" {
		t.Errorf("code=%d stderr=%q", code, stderr)
	}
}

func TestExecuteJSONTrueOnFlagError(t *testing.T) {
	p := todotest.Seed(t, "# Work\n\n- [ ] a\n")
	code, _, stderr := runExecute("list", "--file", p, "--bogus", "--json=true")
	if code != exitUsage {
		t.Fatalf("code=%d", code)
	}
	if je := decodeJSONError(t, stderr); je.Error.Code != "usage" || je.Error.Exit != 2 {
		t.Errorf("unexpected error object: %+v", je.Error)
	}
}

// --json taken as --set-title's value does not turn JSON errors on: the
// error comes after flag parsing, so only the parsed flags count.
func TestExecuteJSONAsFlagValueGivesPlainError(t *testing.T) {
	p := todotest.Seed(t, "# Work\n\n- [ ] a\n")
	code, _, stderr := runExecute("edit", "--file", p, "--set-title", "--json")
	want := "specify one of --number, --id or --title\n"
	if code != exitUsage || stderr != want {
		t.Errorf("code=%d stderr=%q, want %q", code, stderr, want)
	}
}

func TestExecutePlainErrorWithoutJSON(t *testing.T) {
	p := todotest.Seed(t, "# Work\n\n- [ ] a\n")
	code, _, stderr := runExecute("show", "--file", p, "--id", "zzzzzzz")
	if code != exitNotFound || stderr != "no item with --id zzzzzzz\n" {
		t.Errorf("code=%d stderr=%q", code, stderr)
	}
}

func TestExecuteSuccess(t *testing.T) {
	p := todotest.Seed(t, "# Work\n\n- [ ] a\n")
	if code, _, stderr := runExecute("list", "--file", p); code != 0 || stderr != "" {
		t.Errorf("code=%d stderr=%q", code, stderr)
	}
}

func TestErrorCodeNames(t *testing.T) {
	want := map[int]string{1: "error", 2: "usage", 3: "not_found", 4: "ambiguous", 5: "wrong_kind", 6: "conflict"}
	for exit, name := range want {
		if got, n := errorCode(&cliError{exit, "m"}); got != exit || n != name {
			t.Errorf("exit %d: got %d %q, want %q", exit, got, n, name)
		}
	}
	if got, n := errorCode(os.ErrNotExist); got != 1 || n != "error" {
		t.Errorf("plain error: %d %q", got, n)
	}
}
