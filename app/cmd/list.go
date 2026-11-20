package cmd

import (
	"fmt"
	"io"
	"strings"

	"github.com/candy-tools/todo/internal/todo"
	"github.com/spf13/cobra"
)

// columnNames lists the accepted board column names for help and errors.
const columnNames = "backlog|next|progress|done"

func listCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "list",
		Short: "list categories and tasks with their number and id",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			path, isDefault, err := resolvePath(cmd, nil)
			if err != nil {
				return err
			}
			_, doc, err := loadList(cmd, path, isDefault)
			if err != nil {
				return err
			}
			visible, board, err := listVisible(cmd, doc)
			if err != nil {
				return err
			}
			flat, _ := cmd.Flags().GetBool("flat")
			kind, _ := cmd.Flags().GetString("kind")
			statusCSV, _ := cmd.Flags().GetString("status")
			if !flat && (kind != "" || statusCSV != "") {
				return usageErr("--kind and --status need --flat")
			}
			if flat {
				// A board selection lists tasks; only a search adds the path
				// categories of its matches.
				filter, _ := cmd.Flags().GetString("filter")
				tasksOnly := board && strings.TrimSpace(filter) == ""
				keep, err := listKeep(kind, statusCSV, tasksOnly)
				if err != nil {
					return err
				}
				return writeListFlat(cmd.OutOrStdout(), doc, visible, keep)
			}
			if asJSON, _ := cmd.Flags().GetBool("json"); asJSON {
				return writeListJSON(cmd.OutOrStdout(), doc, visible)
			}
			return writeListTable(cmd.OutOrStdout(), doc, visible)
		},
	}
	c.Flags().Bool("json", false, "output the tree as JSON")
	c.Flags().String("filter", "", "only show items matching this text (and their path)")
	c.Flags().String("column", "", "only show tasks in this board column: "+columnNames)
	c.Flags().String("sprint", "", "only show this sprint's tasks, by reference, id or name")
	c.Flags().Bool("backlog", false, "only show tasks not in a sprint (same as --column backlog)")
	c.Flags().Bool("flat", false, "output a flat JSON array of records, one per item, in list order; implies --json")
	c.Flags().String("kind", "", "with --flat, only this kind: task|category")
	c.Flags().String("status", "", "with --flat, only tasks in these states (comma-separated): "+statusNames)
	return c
}

// listVisible returns the items `list` shows (nil => everything): those matching
// --filter, narrowed to the board selection given by --column, --sprint or
// --backlog; board reports whether such a selection was given. A conflicting
// selection or an unknown column or sprint is an error.
func listVisible(cmd *cobra.Command, doc *todo.Document) (visible map[*todo.Item]bool, board bool, err error) {
	filter, _ := cmd.Flags().GetString("filter")
	colName, _ := cmd.Flags().GetString("column")
	backlog, _ := cmd.Flags().GetBool("backlog")
	inSprint := cmd.Flags().Changed("sprint")
	switch {
	case backlog && inSprint:
		return nil, false, usageErr("--backlog and --sprint are mutually exclusive")
	case backlog && colName != "":
		return nil, false, usageErr("--backlog and --column are mutually exclusive")
	case inSprint && colName == todo.ColBacklog.String():
		return nil, false, usageErr("--sprint and --column backlog are mutually exclusive: a sprint has no backlog column")
	case backlog:
		colName = todo.ColBacklog.String()
	}
	if colName == "" && !inSprint {
		return doc.VisibleItems(filter), false, nil
	}
	sprintID := ""
	if inSprint {
		sp, err := resolveSprint(cmd, doc)
		if err != nil {
			return nil, false, err
		}
		sprintID = sp.ID
	}
	cols := todo.SprintColumns()
	if colName != "" {
		col, ok := todo.ColumnFromName(colName)
		if !ok {
			return nil, false, usageErr(fmt.Sprintf("invalid --column %q (%s)", colName, columnNames))
		}
		cols = []todo.Column{col}
	}
	// Search within the columns and sprint, as the TUI tab does.
	return doc.ColumnMatches(filter, sprintID, cols...), true, nil
}

// listKeep builds the --flat predicate from the --kind and --status values. A
// status filter, or tasksOnly, keeps tasks only; a status filter checks each
// task's own state.
func listKeep(kind, statusCSV string, tasksOnly bool) (func(*todo.Item) bool, error) {
	if kind != "" && kind != "task" && kind != "category" {
		return nil, usageErr(fmt.Sprintf("invalid --kind %q (task|category)", kind))
	}
	statuses, err := parseStatuses(statusCSV)
	if err != nil {
		return nil, err
	}
	return func(it *todo.Item) bool {
		if kind == "task" && !it.IsTask() || kind == "category" && it.IsTask() || tasksOnly && !it.IsTask() {
			return false
		}
		return len(statuses) == 0 || it.IsTask() && statuses[it.Status]
	}, nil
}

// parseStatuses parses a comma-separated list of state names into a set; empty
// parts are skipped and an unknown name is a usage error.
func parseStatuses(csv string) (map[todo.Status]bool, error) {
	set := map[todo.Status]bool{}
	for _, name := range strings.Split(csv, ",") {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		st, ok := todo.StatusFromName(name)
		if !ok {
			return nil, usageErr(fmt.Sprintf("invalid --status %q (%s)", name, statusNames))
		}
		set[st] = true
	}
	return set, nil
}

// writeListFlat prints every visible (nil = all) item that keep accepts as a
// record, in list order, as a JSON array ([] when none).
func writeListFlat(w io.Writer, doc *todo.Document, visible map[*todo.Item]bool, keep func(*todo.Item) bool) error {
	nums := numberMap(doc)
	recs := []record{}
	for _, it := range doc.Enumerate() {
		if visible != nil && !visible[it] {
			continue
		}
		if keep != nil && !keep(it) {
			continue
		}
		recs = append(recs, toRecord(doc, it, nums))
	}
	return writeJSON(w, recs)
}

// depthOf is the item's nesting depth from the roots (0 = top level), used to
// indent the table.
func depthOf(it *todo.Item) int {
	n := 0
	for p := it.Parent; p != nil; p = p.Parent {
		n++
	}
	return n
}

// writeListTable prints the tree as a table; visible (nil => all) limits the rows.
func writeListTable(w io.Writer, doc *todo.Document, visible map[*todo.Item]bool) error {
	_, _ = fmt.Fprintln(w, "  #      id  state     item")
	for i, it := range doc.Enumerate() {
		if visible != nil && !visible[it] {
			continue
		}
		st := "--"
		if it.IsTask() {
			st = it.Status.String()
		}
		indent := strings.Repeat("  ", depthOf(it))
		_, _ = fmt.Fprintf(w, "%3d %7s  %-9s %s%s\n", i+1, it.ID, st, indent, todo.Printable(it.DisplayTitle()))
	}
	return nil
}

// jsonItem is one node of the tree `list --json` emits: a record plus its
// children.
type jsonItem struct {
	record
	Children []jsonItem `json:"children"`
}

// pathOf returns the titles of an item's ancestors, root-first (excluding the
// item itself).
func pathOf(it *todo.Item) []string {
	full := it.Path()
	if len(full) == 0 {
		return []string{}
	}
	return full[:len(full)-1]
}

// toJSONItem builds the JSON node for it. nums supplies numbers; visible (nil =>
// all) prunes to filtered items.
func toJSONItem(doc *todo.Document, it *todo.Item, nums map[*todo.Item]int, visible map[*todo.Item]bool) jsonItem {
	ji := jsonItem{record: toRecord(doc, it, nums), Children: []jsonItem{}}
	for _, c := range it.Children {
		if visible != nil && !visible[c] {
			continue
		}
		ji.Children = append(ji.Children, toJSONItem(doc, c, nums, visible))
	}
	return ji
}

func writeListJSON(w io.Writer, doc *todo.Document, visible map[*todo.Item]bool) error {
	nums := numberMap(doc)
	roots := []jsonItem{}
	for _, r := range doc.Roots {
		if visible != nil && !visible[r] {
			continue
		}
		roots = append(roots, toJSONItem(doc, r, nums, visible))
	}
	return writeJSON(w, roots)
}
