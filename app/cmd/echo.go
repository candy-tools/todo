package cmd

import (
	"fmt"

	"github.com/candy-tools/todo/internal/todo"
	"github.com/spf13/cobra"
)

// addJSONFlag registers --json on a command that prints a result.
func addJSONFlag(c *cobra.Command) {
	c.Flags().Bool("json", false, "print the result as JSON")
}

// wantJSON reports whether --json was given.
func wantJSON(cmd *cobra.Command) bool {
	v, _ := cmd.Flags().GetBool("json")
	return v
}

// echoItem reports the result of a write on the item id. doc is the list the
// write left. Under --json it prints the item's record (with its subtasks);
// otherwise it calls text, which prints the command's confirmation line. Under
// --json an item missing from doc is an error; in text mode doc is not read, so
// nothing is checked.
func echoItem(cmd *cobra.Command, doc *todo.Document, id string, text func()) error {
	if !wantJSON(cmd) {
		text()
		return nil
	}
	it, ok := doc.ByID(id)
	if !ok {
		return fmt.Errorf("item %s is missing from the written list", id)
	}
	return writeJSON(cmd.OutOrStdout(), toDetail(doc, it, numberMap(doc)))
}

// sprintOf is the jsonSprint for sp as doc has it.
func sprintOf(doc *todo.Document, sp *todo.Sprint) jsonSprint {
	return jsonSprint{sp.ID, sp.Name, sp.Goal, sp.Prefix, sp.Status.String(), sprintCounts(doc, sp.ID)}
}
