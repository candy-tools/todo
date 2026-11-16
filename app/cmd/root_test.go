package cmd

import (
	"os"
	"testing"
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
	if c.Use != "todo [db-file]" || c.Short != "A minimalistic terminal TODO manager" {
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
