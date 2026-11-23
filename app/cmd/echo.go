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
	r, err := writtenDetail(doc, id)
	if err != nil {
		return err
	}
	return writeJSON(cmd.OutOrStdout(), r)
}

// writtenDetail is the record (with its subtasks) of the item id in doc, the
// list a write left; an item missing from doc is an error.
func writtenDetail(doc *todo.Document, id string) (record, error) {
	it, ok := doc.ByID(id)
	if !ok {
		return record{}, fmt.Errorf("item %s is missing from the written list", id)
	}
	return toDetail(doc, it, numberMap(doc)), nil
}

// sprintOf is the jsonSprint for sp as doc has it.
func sprintOf(doc *todo.Document, sp *todo.Sprint) jsonSprint {
	return jsonSprint{sp.ID, sp.Ref, sp.Name, sp.Goal, doc.SprintStatus(sp.ID).String(), sprintCounts(doc, sp.ID)}
}
