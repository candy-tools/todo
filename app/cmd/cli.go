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

// flagError marks an error from flag parsing (see the root command's
// SetFlagErrorFunc). It wraps the usage cliError, so errorCode still sees it.
type flagError struct{ error }

func (e *flagError) Unwrap() error { return e.error }

func usageErr(msg string) error { return &cliError{exitUsage, msg} }
func notFound(msg string) error { return &cliError{exitNotFound, msg} }

// errorNames are the JSON error code names, indexed by exit code.
var errorNames = map[int]string{
	exitGeneric:   "error",
	exitUsage:     "usage",
	exitNotFound:  "not_found",
	exitAmbiguous: "ambiguous",
	exitWrongKind: "wrong_kind",
	exitConflict:  "conflict",
}

// errorCode returns err's exit code (a cliError's, else 1) and its name.
func errorCode(err error) (exit int, name string) {
	exit = exitGeneric
	var ce *cliError
	if errors.As(err, &ce) {
		exit = ce.code
	}
	if name, ok := errorNames[exit]; ok {
		return exit, name
	}
	return exit, errorNames[exitGeneric]
}

// jsonError is the structured error printed to stderr under --json or --flat.
type jsonError struct {
	Error struct {
		Code    string `json:"code"`
		Exit    int    `json:"exit"`
		Message string `json:"message"`
	} `json:"error"`
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

// addSelectorFlags registers the number/id/title trio under an optional prefix
// (e.g. "parent-").
func addSelectorFlags(cmd *cobra.Command, prefix string) {
	cmd.Flags().Int(prefix+"number", 0, "address the item by its list number")
	cmd.Flags().String(prefix+"id", "", "address the item by its 7-character id (see list --json) or a task's reference")
	cmd.Flags().String(prefix+"title", "", "address the item by its exact title")
}

// resolveSelector reads the number/id/title flags (optionally prefixed) and
// returns the item they identify. Exactly one flag must be set. The id flag
// also takes a task's reference (ignoring case); an item ID wins over a
// reference that happens to be spelled the same.
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
		if it, ok := doc.ByRef(n); ok {
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

// resolveSprint reads --sprint and returns the sprint it names, by reference
// (TODO-001), ID or name, ignoring case (see Document.ResolveSprint). A missing
// flag is a usage error and an unknown sprint is not-found. Names may repeat:
// a name several sprints share is ambiguous, and the error lists their
// references.
func resolveSprint(cmd *cobra.Command, doc *todo.Document) (*todo.Sprint, error) {
	key, _ := cmd.Flags().GetString("sprint")
	if key == "" {
		return nil, usageErr("specify --sprint <ref|id|name>")
	}
	matches := doc.ResolveSprint(key)
	switch len(matches) {
	case 0:
		return nil, notFound("no sprint " + key)
	case 1:
		return matches[0], nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%q matches %d sprints; name one by its reference:\n", key, len(matches))
	for _, sp := range matches {
		fmt.Fprintf(&b, "  --sprint %s  (%s, %s)\n", sp.Ref, todo.Printable(sp.Name), sp.Status)
	}
	return nil, &cliError{exitAmbiguous, b.String()}
}

// sprintLabel is how a sprint is named in text output: its reference, then its
// name ("TODO-001 Sprint 13"), or the name alone when it has no reference.
func sprintLabel(sp *todo.Sprint) string {
	if sp.Ref == "" {
		return todo.Printable(sp.Name)
	}
	return sp.Ref + " " + todo.Printable(sp.Name)
}
