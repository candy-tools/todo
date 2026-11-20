package cmd

import (
	"fmt"
	"io"
	"strings"

	"github.com/candy-tools/todo/internal/todo"
	"github.com/spf13/cobra"
)

// showCmd is `todo show`: one item with its path, sprint, plan, description and
// subtasks, as text or as one JSON record.
func showCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "show",
		Short: "show one item with its path, sprint, plan, description and subtasks",
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
			it, err := resolveSelector(cmd, doc, "")
			if err != nil {
				return err
			}
			r := toDetail(doc, it, numberMap(doc))
			if asJSON, _ := cmd.Flags().GetBool("json"); asJSON {
				return writeJSON(cmd.OutOrStdout(), r)
			}
			return writeShowText(cmd.OutOrStdout(), it, r)
		},
	}
	addSelectorFlags(c, "")
	c.Flags().Bool("json", false, "output the item as JSON")
	return c
}

// writeShowText prints the record r of it as indented text; lines for an empty
// sprint, plan, description or subtask list are left out.
func writeShowText(w io.Writer, it *todo.Item, r record) error {
	p := todo.Printable
	_, _ = fmt.Fprintln(w, p(it.DisplayTitle()))
	head := []string{"id " + r.ID, fmt.Sprintf("#%d", r.Number), r.Kind}
	if r.Kind == "task" {
		head = append(head, r.Status, "column "+r.Column)
		if r.SprintName != "" {
			head = append(head, "sprint "+p(r.SprintName))
		}
	}
	if r.Prefix != "" {
		head = append(head, "prefix "+p(r.Prefix))
	}
	_, _ = fmt.Fprintln(w, "  "+strings.Join(head, " · "))
	if len(r.Path) > 0 {
		parts := make([]string, len(r.Path))
		for i, s := range r.Path {
			parts[i] = p(s)
		}
		_, _ = fmt.Fprintln(w, "  path: "+strings.Join(parts, " / "))
	}
	if r.Plan != "" {
		_, _ = fmt.Fprintln(w, "  plan: "+p(r.Plan))
	}
	if len(r.Subtasks) > 0 {
		_, _ = fmt.Fprintf(w, "  subtasks: %d/%d done\n", r.DoneCount, r.TotalCount)
		for _, c := range it.Children {
			if c.IsTask() {
				_, _ = fmt.Fprintf(w, "    [%s] %s\n", c.Status.Marker(), p(c.Title))
			}
		}
	}
	if r.Description != "" {
		_, _ = fmt.Fprintln(w, "  description:")
		for _, line := range strings.Split(r.Description, "\n") {
			_, _ = fmt.Fprintln(w, "    "+p(line))
		}
	}
	return nil
}
