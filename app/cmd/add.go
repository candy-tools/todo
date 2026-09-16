package cmd

import (
	"fmt"
	"strings"

	"github.com/candy-tools/todo/internal/todo"
	"github.com/spf13/cobra"
)

func addCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "add",
		Short: "add a task under a category (or as a subtask of a task)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			path := fileArg(cmd)
			doc, err := todo.Load(path)
			if err != nil {
				return err
			}
			parent, err := resolveSelector(cmd, doc, "parent-")
			if err != nil {
				return err
			}
			title, _ := cmd.Flags().GetString("title")
			if strings.TrimSpace(title) == "" {
				return usageErr("--title is required")
			}
			desc, _ := cmd.Flags().GetString("desc")
			statusName, _ := cmd.Flags().GetString("status")
			st, ok := todo.StatusFromName(statusName)
			if !ok {
				return usageErr(fmt.Sprintf("invalid --status %q (open|progress|deferred|done)", statusName))
			}
			task := &todo.Item{Kind: todo.Task, Title: title, Description: desc, Status: st}
			switch parent.Kind {
			case todo.Category:
				parent.AppendTask(task)
			default: // Task
				parent.AppendChild(task)
			}
			if err := doc.Save(path); err != nil {
				return err
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "add: [%s] %s  (%s)\n", task.Status.Marker(), task.Title, categoryPath(task))
			return nil
		},
	}
	c.Flags().String("title", "", "the new task's title (required)")
	c.Flags().String("desc", "", "the new task's description")
	c.Flags().String("status", "open", "initial status: open|progress|deferred|done")
	addSelectorFlags(c, "parent-")
	return c
}

func addCategoryCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "add-category",
		Short: "add a category (top-level, or nested under a parent category)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			path := fileArg(cmd)
			doc, err := todo.Load(path)
			if err != nil {
				return err
			}
			title, _ := cmd.Flags().GetString("title")
			if strings.TrimSpace(title) == "" {
				return usageErr("--title is required")
			}
			cat := &todo.Item{Kind: todo.Category, Title: title, Level: 1}
			hasParent := cmd.Flags().Changed("parent-number") ||
				cmd.Flags().Changed("parent-line") || cmd.Flags().Changed("parent-title")
			if hasParent {
				parent, err := resolveSelector(cmd, doc, "parent-")
				if err != nil {
					return err
				}
				if parent.Kind != todo.Category {
					return wrongKind("nest a category under", parent, "category")
				}
				cat.Level = parent.Level + 1
				if cat.Level > 6 {
					return usageErr("categories nest at most 6 levels deep")
				}
				parent.AppendChild(cat)
			} else {
				doc.AppendRoot(cat)
			}
			if err := doc.Save(path); err != nil {
				return err
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "add-category: %s %s\n", strings.Repeat("#", cat.Level), cat.Title)
			return nil
		},
	}
	c.Flags().String("title", "", "the category title (required)")
	addSelectorFlags(c, "parent-")
	return c
}
