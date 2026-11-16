package cmd

import (
	"errors"
	"fmt"
	"strings"

	"github.com/candy-tools/todo/internal/todo"
	"github.com/spf13/cobra"
)

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
	exitConflict  = 6
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
		return todo.Printable(c.Title)
	}
	return ""
}

// wrongKindIn names the action and the item a store ErrWrongKind refused, as
// in `cannot nest a category under "a": <the store's rule>`, so the message
// says which item was of the wrong kind; it returns any other error
// unchanged. The store decides the rule; storeErr maps the class.
func wrongKindIn(err error, action string, it *todo.Item) error {
	if errors.Is(err, todo.ErrWrongKind) {
		return fmt.Errorf("cannot %s %q: %w", action, it.Title, err)
	}
	return err
}

func wrongKind(action string, it *todo.Item, want string) error {
	return &cliError{exitWrongKind, fmt.Sprintf(
		"cannot %s %q: it is a %s, not a %s", action, it.Title, kindName(it), want)}
}

// addSelectorFlags registers the number/id/title trio under an optional prefix
// (e.g. "parent-").
func addSelectorFlags(cmd *cobra.Command, prefix string) {
	cmd.Flags().Int(prefix+"number", 0, "address the item by its list number")
	cmd.Flags().String(prefix+"id", "", "address the item by its 7-character id (see list --json)")
	cmd.Flags().String(prefix+"title", "", "address the item by its exact title")
}

// resolveSelector reads the number/id/title flags (optionally prefixed) and
// returns the item they identify. Exactly one flag must be set.
func resolveSelector(cmd *cobra.Command, doc *todo.Document, prefix string) (*todo.Item, error) {
	numF, idF, titleF := prefix+"number", prefix+"id", prefix+"title"
	set := 0
	for _, name := range []string{numF, idF, titleF} {
		if cmd.Flags().Changed(name) {
			set++
		}
	}
	switch {
	case set == 0:
		return nil, usageErr(fmt.Sprintf("specify one of --%s, --%s or --%s", numF, idF, titleF))
	case set > 1:
		return nil, usageErr(fmt.Sprintf("--%s, --%s and --%s are mutually exclusive", numF, idF, titleF))
	case cmd.Flags().Changed(numF):
		n, _ := cmd.Flags().GetInt(numF)
		if it, ok := doc.ByNumber(n); ok {
			return it, nil
		}
		return nil, notFound(fmt.Sprintf("no item with --%s %d", numF, n))
	case cmd.Flags().Changed(idF):
		n, _ := cmd.Flags().GetString(idF)
		if it, ok := doc.ByID(n); ok {
			return it, nil
		}
		return nil, notFound(fmt.Sprintf("no item with --%s %s", idF, n))
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
			fmt.Fprintf(&b, "%q matches %d items; disambiguate with --%s or --%s:\n", title, len(matches), numF, idF)
			for _, it := range matches {
				fmt.Fprintf(&b, "  --%s %d  (--%s %s, %s)\n", numF, nums[it], idF, it.ID, categoryPath(it))
			}
			return nil, &cliError{exitAmbiguous, b.String()}
		}
	}
}
