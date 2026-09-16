package cmd

import (
	"fmt"
	"strings"

	"github.com/candy-tools/todo/internal/todo"
	"github.com/spf13/cobra"
)

// fileArg returns the file a subcommand operates on (the --file flag, default
// TODO.md).
func fileArg(cmd *cobra.Command) string {
	f, _ := cmd.Flags().GetString("file")
	return f
}

// numberMap maps each item to its 1-based list number (see Document.Enumerate).
func numberMap(d *todo.Document) map[*todo.Item]int {
	m := map[*todo.Item]int{}
	for i, it := range d.Enumerate() {
		m[it] = i + 1
	}
	return m
}

// Exit codes for the agent-facing CLI (see the design spec).
const (
	exitGeneric   = 1
	exitUsage     = 2
	exitNotFound  = 3
	exitAmbiguous = 4
	exitWrongKind = 5
)

// cliError carries an exit code alongside its message so Execute can exit with
// a stable, agent-parseable status.
type cliError struct {
	code int
	msg  string
}

func (e *cliError) Error() string { return e.msg }

func usageErr(msg string) error { return &cliError{exitUsage, msg} }
func notFound(msg string) error { return &cliError{exitNotFound, msg} }

func kindName(it *todo.Item) string {
	if it.IsTask() {
		return "task"
	}
	return "category"
}

// categoryPath returns the enclosing category's title, for confirmation lines.
func categoryPath(it *todo.Item) string {
	if c := it.EnclosingCategory(); c != nil {
		return c.Title
	}
	return ""
}

func wrongKind(action string, it *todo.Item, want string) error {
	return &cliError{exitWrongKind, fmt.Sprintf(
		"cannot %s %q: it is a %s, not a %s", action, it.Title, kindName(it), want)}
}

// addSelectorFlags registers the number/line/title trio under an optional prefix
// (e.g. "parent-").
func addSelectorFlags(cmd *cobra.Command, prefix string) {
	cmd.Flags().Int(prefix+"number", 0, "address the item by its list number")
	cmd.Flags().Int(prefix+"line", 0, "address the item by its file line")
	cmd.Flags().String(prefix+"title", "", "address the item by its exact title")
}

// resolveSelector reads the number/line/title flags (optionally prefixed) and
// returns the item they identify. Exactly one flag must be set.
func resolveSelector(cmd *cobra.Command, doc *todo.Document, prefix string) (*todo.Item, error) {
	numF, lineF, titleF := prefix+"number", prefix+"line", prefix+"title"
	set := 0
	for _, name := range []string{numF, lineF, titleF} {
		if cmd.Flags().Changed(name) {
			set++
		}
	}
	switch {
	case set == 0:
		return nil, usageErr(fmt.Sprintf("specify one of --%s, --%s or --%s", numF, lineF, titleF))
	case set > 1:
		return nil, usageErr(fmt.Sprintf("--%s, --%s and --%s are mutually exclusive", numF, lineF, titleF))
	case cmd.Flags().Changed(numF):
		n, _ := cmd.Flags().GetInt(numF)
		if it, ok := doc.ByNumber(n); ok {
			return it, nil
		}
		return nil, notFound(fmt.Sprintf("no item with --%s %d", numF, n))
	case cmd.Flags().Changed(lineF):
		n, _ := cmd.Flags().GetInt(lineF)
		if it, ok := doc.ByLine(n); ok {
			return it, nil
		}
		return nil, notFound(fmt.Sprintf("no item on --%s %d", lineF, n))
	default:
		title, _ := cmd.Flags().GetString(titleF)
		matches := doc.ByTitle(title)
		switch len(matches) {
		case 0:
			return nil, notFound(fmt.Sprintf("no item titled %q", title))
		case 1:
			return matches[0], nil
		default:
			nums := numberMap(doc)
			var b strings.Builder
			fmt.Fprintf(&b, "%q matches %d items; disambiguate with --%s or --%s:\n", title, len(matches), numF, lineF)
			for _, it := range matches {
				fmt.Fprintf(&b, "  --%s %d  (--%s %d, %s)\n", numF, nums[it], lineF, it.Line, categoryPath(it))
			}
			return nil, &cliError{exitAmbiguous, b.String()}
		}
	}
}
