package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/candy-tools/todo/internal/todo"
	"github.com/spf13/cobra"
)

func listCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "list",
		Short: "list categories and tasks with their number and line",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			doc, err := todo.Load(fileArg(cmd))
			if err != nil {
				return err
			}
			filter, _ := cmd.Flags().GetString("filter")
			if asJSON, _ := cmd.Flags().GetBool("json"); asJSON {
				return writeListJSON(cmd.OutOrStdout(), doc, filter)
			}
			return writeListTable(cmd.OutOrStdout(), doc, filter)
		},
	}
	c.Flags().Bool("json", false, "output the tree as JSON")
	c.Flags().String("filter", "", "only show items matching this text (and their path)")
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

func writeListTable(w io.Writer, doc *todo.Document, filter string) error {
	visible := doc.VisibleItems(filter) // nil => show everything
	fmt.Fprintln(w, "  #  line  st   item")
	for i, it := range doc.Enumerate() {
		if visible != nil && !visible[it] {
			continue
		}
		st := "--"
		if it.IsTask() {
			st = "[" + it.Status.Marker() + "]"
		}
		indent := strings.Repeat("  ", depthOf(it))
		fmt.Fprintf(w, "%3d %5d  %-4s %s%s\n", i+1, it.Line, st, indent, it.Title)
	}
	return nil
}

// jsonItem is the shape emitted by `list --json` and by mutating commands under
// --json.
type jsonItem struct {
	Number      int        `json:"number"`
	Line        int        `json:"line"`
	Kind        string     `json:"kind"`
	Title       string     `json:"title"`
	Status      string     `json:"status,omitempty"`
	Level       int        `json:"level,omitempty"`
	Path        []string   `json:"path"`
	Description string     `json:"description,omitempty"`
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
		Number: nums[it], Line: it.Line, Title: it.Title, Path: pathOf(it),
		Children: []jsonItem{},
	}
	done, total := it.TaskCounts()
	ji.DoneCount, ji.TotalCount = done, total
	if it.IsTask() {
		ji.Kind = "task"
		ji.Status = it.Status.String()
		ji.Description = it.Description
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

func writeListJSON(w io.Writer, doc *todo.Document, filter string) error {
	visible := doc.VisibleItems(filter)
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
