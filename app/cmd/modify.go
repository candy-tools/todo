package cmd

import (
	"context"
	"fmt"

	"github.com/candy-tools/todo/internal/todo"
	"github.com/spf13/cobra"
)

func rmCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "rm",
		Short: "remove an item and its subtree",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return withStore(cmd, func(ctx context.Context, s *todo.Store, doc *todo.Document) error {
				it, err := resolveSelector(cmd, doc, "")
				if err != nil {
					return err
				}
				if _, err := s.Delete(ctx, it.ID); err != nil {
					return err
				}
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "rm: %s\n", todo.Printable(it.DisplayTitle()))
				return nil
			})
		},
	}
	addSelectorFlags(c, "")
	return c
}

func pruneCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "prune",
		Short: "remove every fully-completed task",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return withStore(cmd, func(ctx context.Context, s *todo.Store, _ *todo.Document) error {
				n, _, err := s.Prune(ctx)
				if err != nil {
					return err
				}
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "prune: removed %d task(s)\n", n)
				return nil
			})
		},
	}
}

func editCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "edit",
		Short: "change an item's title and/or a task's description, plan or reference",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return withStore(cmd, func(ctx context.Context, s *todo.Store, doc *todo.Document) error {
				it, err := resolveSelector(cmd, doc, "")
				if err != nil {
					return err
				}
				setTitle := cmd.Flags().Changed("set-title")
				setDesc := cmd.Flags().Changed("set-desc")
				setPlan := cmd.Flags().Changed("set-plan")
				setRef := cmd.Flags().Changed("set-ref")
				if !setTitle && !setDesc && !setPlan && !setRef {
					return usageErr("specify --set-title, --set-desc, --set-plan and/or --set-ref")
				}
				// Only the flags given are written, so a field another process
				// changed since the read is kept.
				var ch todo.Changes
				title := it.Title
				if setTitle {
					title, _ = cmd.Flags().GetString("set-title")
					if title, err = todo.CleanTitle(title); err != nil {
						return usageErr("--set-title: " + err.Error())
					}
					ch.Title = &title
				}
				if setDesc {
					desc, _ := cmd.Flags().GetString("set-desc")
					ch.Description = &desc
				}
				if setPlan {
					plan, _ := cmd.Flags().GetString("set-plan")
					ch.Plan = &plan
				}
				if setRef {
					ref, _ := cmd.Flags().GetString("set-ref")
					if ref, err = todo.CleanRef(ref); err != nil {
						return usageErr("--set-ref: " + err.Error())
					}
					ch.Ref = &ref
				}
				// The store refuses a description, plan or reference for a
				// category, and a reference for a subtask or one another task
				// holds.
				res, err := s.Edit(ctx, it.ID, ch)
				if err != nil {
					return wrongKindIn(err, "edit", it)
				}
				shown := title
				if now, ok := res.Doc.ByID(it.ID); ok {
					shown = now.DisplayTitle()
				}
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "edit: %s\n", todo.Printable(shown))
				return nil
			})
		},
	}
	addSelectorFlags(c, "")
	c.Flags().String("set-title", "", "the new title")
	c.Flags().String("set-desc", "", "the new description (tasks only)")
	c.Flags().String("set-plan", "", "the new plan path or reference; empty clears it (tasks only)")
	c.Flags().String("set-ref", "", "the new reference, such as APP-001; unique in the list, empty clears it (tasks, not subtasks)")
	return c
}
