package cmd

import (
	"context"
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
			return withStore(cmd, func(ctx context.Context, s *todo.Store, doc *todo.Document) error {
				parent, err := resolveSelector(cmd, doc, "parent-")
				if err != nil {
					return err
				}
				title, _ := cmd.Flags().GetString("title")
				if title, err = todo.CleanTitle(title); err != nil {
					return usageErr("--title: " + err.Error())
				}
				desc, _ := cmd.Flags().GetString("desc")
				statusName, _ := cmd.Flags().GetString("status")
				st, ok := todo.StatusFromName(statusName)
				if !ok {
					return usageErr(fmt.Sprintf("invalid --status %q (%s)", statusName, statusNames))
				}
				plan, _ := cmd.Flags().GetString("plan")
				// The store places the task after the parent's last task as the tree
				// is when it writes, so a sibling changed meanwhile does not matter.
				id, res, err := s.AppendTask(ctx, todo.Fields{Title: title, Description: desc, Plan: plan}, st, parent.ID)
				if err != nil {
					return err
				}
				if task, ok := res.Doc.ByID(id); ok {
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "add: [%s] %s  (%s)\n", task.Status.String(), task.Title, categoryPath(task))
				}
				return nil
			})
		},
	}
	c.Flags().String("title", "", "the new task's title (required)")
	c.Flags().String("desc", "", "the new task's description")
	c.Flags().String("plan", "", "a path or reference to the task's plan")
	c.Flags().String("status", "open", "initial state: "+statusNames)
	addSelectorFlags(c, "parent-")
	return c
}

func addCategoryCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "add-category",
		Short: "add a category (top-level, or nested under a parent category)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return withStore(cmd, func(ctx context.Context, s *todo.Store, doc *todo.Document) error {
				title, _ := cmd.Flags().GetString("title")
				title, err := todo.CleanTitle(title)
				if err != nil {
					return usageErr("--title: " + err.Error())
				}
				var parent *todo.Item
				if cmd.Flags().Changed("parent-number") || cmd.Flags().Changed("parent-id") || cmd.Flags().Changed("parent-title") {
					if parent, err = resolveSelector(cmd, doc, "parent-"); err != nil {
						return err
					}
				}
				// The store places the category last under its parent as it
				// finds the tree when it writes, so a concurrent change cannot
				// make the add fail, and decides whether the parent can hold a
				// category.
				parentID := ""
				if parent != nil {
					parentID = parent.ID
				}
				id, res, err := s.AppendCategory(ctx, title, parentID)
				if err != nil {
					if parent != nil {
						return wrongKindIn(err, "nest a category under", parent)
					}
					return err
				}
				if cat, ok := res.Doc.ByID(id); ok {
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "add-category: %s %s\n", strings.Repeat("#", depthOf(cat)+1), title)
				}
				return nil
			})
		},
	}
	c.Flags().String("title", "", "the category title (required)")
	addSelectorFlags(c, "parent-")
	return c
}
