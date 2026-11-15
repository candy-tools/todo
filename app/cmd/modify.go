package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/candy-tools/todo/internal/todo"
	"github.com/spf13/cobra"
)

func rmCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "rm",
		Short: "remove an item and its subtree",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return withStore(cmd, func(ctx context.Context, open storeOpener, doc *todo.Document) error {
				it, err := resolveSelector(cmd, doc, "")
				if err != nil {
					return err
				}
				s, err := open()
				if err != nil {
					return err
				}
				if err := s.Delete(ctx, it.ID); err != nil {
					return err
				}
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "rm: %s\n", todo.Printable(it.Title))
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
			return withStore(cmd, func(ctx context.Context, open storeOpener, _ *todo.Document) error {
				s, err := open()
				if err != nil {
					return err
				}
				n, err := s.Prune(ctx)
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
		Short: "change an item's title and/or a task's description or plan",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return withStore(cmd, func(ctx context.Context, open storeOpener, doc *todo.Document) error {
				it, err := resolveSelector(cmd, doc, "")
				if err != nil {
					return err
				}
				setTitle := cmd.Flags().Changed("set-title")
				setDesc := cmd.Flags().Changed("set-desc")
				setPlan := cmd.Flags().Changed("set-plan")
				if !setTitle && !setDesc && !setPlan {
					return usageErr("specify --set-title, --set-desc and/or --set-plan")
				}
				// Only the flags given are written, so a field another process
				// changed since the read is kept.
				var ch todo.Changes
				title := it.Title
				if setTitle {
					title, _ = cmd.Flags().GetString("set-title")
					if strings.TrimSpace(title) == "" {
						return usageErr("--set-title must not be empty")
					}
					if title, err = todo.CleanTitle(title); err != nil {
						return usageErr("--set-title: " + err.Error())
					}
					ch.Title = &title
				}
				if setDesc {
					if !it.IsTask() {
						return wrongKind("set a description on", it, "task")
					}
					desc, _ := cmd.Flags().GetString("set-desc")
					ch.Description = &desc
				}
				if setPlan {
					if !it.IsTask() {
						return wrongKind("set a plan on", it, "task")
					}
					plan, _ := cmd.Flags().GetString("set-plan")
					ch.Plan = &plan
				}
				s, err := open()
				if err != nil {
					return err
				}
				if err := s.Edit(ctx, it.ID, ch); err != nil {
					return err
				}
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "edit: %s\n", title)
				return nil
			})
		},
	}
	addSelectorFlags(c, "")
	c.Flags().String("set-title", "", "the new title")
	c.Flags().String("set-desc", "", "the new description (tasks only)")
	c.Flags().String("set-plan", "", "the new plan path or reference; empty clears it (tasks only)")
	return c
}
