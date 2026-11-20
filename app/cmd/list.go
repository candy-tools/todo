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
			_, doc, err := loadList(cmd, path, isDefault)
			if err != nil {
				return err
			}
			filter, _ := cmd.Flags().GetString("filter")
			visible := doc.VisibleItems(filter) // nil => show everything
			colName, _ := cmd.Flags().GetString("column")
			backlog, _ := cmd.Flags().GetBool("backlog")
			inSprint := cmd.Flags().Changed("sprint")
			switch {
			case backlog && inSprint:
				return usageErr("--backlog and --sprint are mutually exclusive")
			case backlog && colName != "":
				return usageErr("--backlog and --column are mutually exclusive")
			case inSprint && colName == todo.ColBacklog.String():
				return usageErr("--sprint and --column backlog are mutually exclusive: a sprint has no backlog column")
			case backlog:
				colName = todo.ColBacklog.String()
			}
			sprintID := ""
			if inSprint {
				sp, err := resolveSprint(cmd, doc)
				if err != nil {
					return err
				}
				sprintID = sp.ID
			}
			if colName != "" || inSprint {
				cols := todo.SprintColumns()
				if colName != "" {
					col, ok := todo.ColumnFromName(colName)
					if !ok {
						return usageErr(fmt.Sprintf("invalid --column %q (%s)", colName, columnNames))
					}
					cols = []todo.Column{col}
				}
				// Search within the columns and sprint, as the TUI tab does.
				visible = doc.ColumnMatches(filter, sprintID, cols...)
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
	c.Flags().String("sprint", "", "only show this sprint's tasks, by id or name")
	c.Flags().Bool("backlog", false, "only show tasks not in a sprint (same as --column backlog)")
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

// jsonItem is the shape emitted by `list --json` and by mutating commands under
// --json.
type jsonItem struct {
	Number      int        `json:"number"`
	ID          string     `json:"id"`
	Kind        string     `json:"kind"`
	Ref         string     `json:"ref,omitempty"`
	Prefix      string     `json:"prefix,omitempty"` // a category's reference prefix
	Title       string     `json:"title"`
	Status      string     `json:"status,omitempty"`
	Column      string     `json:"column,omitempty"`
	Sprint      string     `json:"sprint,omitempty"` // the sprint's id; a subtask shows its parent's
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
		ji.Column = it.Column().String() // a subtask sits in its parent's column
		ji.Description = it.Description
		ji.Plan = it.Plan
		ji.Sprint = it.SprintID()
		ji.Ref = it.Ref
	} else {
		ji.Kind = "category"
		ji.Level = it.Level
		ji.Prefix = it.Prefix
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
