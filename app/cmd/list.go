package cmd

import (
	"encoding/json"
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
			doc, err := openForRead(cmd, path, isDefault)
			if err != nil {
				return err
			}
			filter, _ := cmd.Flags().GetString("filter")
			visible := doc.VisibleItems(filter) // nil => show everything
			if colName, _ := cmd.Flags().GetString("column"); colName != "" {
				col, ok := todo.ColumnFromName(colName)
				if !ok {
					return usageErr(fmt.Sprintf("invalid --column %q (%s)", colName, columnNames))
				}
				visible = doc.ColumnMatches(filter, col) // search within the column, as the TUI tab does
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
	return c
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
	_, _ = fmt.Fprintln(w, "  #    id  state     item")
	for i, it := range doc.Enumerate() {
		if visible != nil && !visible[it] {
			continue
		}
		st := "--"
		if it.IsTask() {
			st = it.Status.String()
		}
		indent := strings.Repeat("  ", depthOf(it))
		_, _ = fmt.Fprintf(w, "%3d %5d  %-9s %s%s\n", i+1, it.ID, st, indent, todo.Printable(it.Title))
	}
	return nil
}

// jsonItem is the shape emitted by `list --json` and by mutating commands under
// --json.
type jsonItem struct {
	Number      int        `json:"number"`
	ID          uint       `json:"id"`
	Kind        string     `json:"kind"`
	Title       string     `json:"title"`
	Status      string     `json:"status,omitempty"`
	Column      string     `json:"column,omitempty"`
	Level       int        `json:"level,omitempty"`
	Path        []string   `json:"path"`
	Description string     `json:"description,omitempty"`
	Plan        string     `json:"plan,omitempty"`
	DoneCount   int        `json:"doneCount"`
	TotalCount  int        `json:"totalCount"`
	Children    []jsonItem `json:"children"`
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
func toJSONItem(it *todo.Item, nums map[*todo.Item]int, visible map[*todo.Item]bool) jsonItem {
	ji := jsonItem{
		Number: nums[it], ID: it.ID, Title: it.Title, Path: pathOf(it),
		Children: []jsonItem{},
	}
	done, total := it.TaskCounts()
	ji.DoneCount, ji.TotalCount = done, total
	if it.IsTask() {
		ji.Kind = "task"
		ji.Status = it.Status.String()
		// A subtask sits in its parent's column.
		col := it.Status.Column()
		if it.IsSubtask() {
			col = it.Parent.Status.Column()
		}
		ji.Column = col.String()
		ji.Description = it.Description
		ji.Plan = it.Plan
	} else {
		ji.Kind = "category"
		ji.Level = it.Level
	}
	for _, c := range it.Children {
		if visible != nil && !visible[c] {
			continue
		}
		ji.Children = append(ji.Children, toJSONItem(c, nums, visible))
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
		roots = append(roots, toJSONItem(r, nums, visible))
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(roots)
}
