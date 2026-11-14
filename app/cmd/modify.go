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
			return withStore(cmd, func(ctx context.Context, s *todo.Store, doc *todo.Document) error {
				it, err := resolveSelector(cmd, doc, "")
				if err != nil {
					return err
				}
				if err := s.Delete(ctx, it.ID); err != nil {
					return err
				}
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "rm: %s\n", it.Title)
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
		Short: "change an item's title and/or a task's description",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return withStore(cmd, func(ctx context.Context, s *todo.Store, doc *todo.Document) error {
				it, err := resolveSelector(cmd, doc, "")
				if err != nil {
					return err
				}
				setTitle := cmd.Flags().Changed("set-title")
				setDesc := cmd.Flags().Changed("set-desc")
				if !setTitle && !setDesc {
					return usageErr("specify --set-title and/or --set-desc")
				}
				title, desc := it.Title, it.Description
				if setTitle {
					title, _ = cmd.Flags().GetString("set-title")
					if strings.TrimSpace(title) == "" {
						return usageErr("--set-title must not be empty")
					}
				}
				if setDesc {
					if !it.IsTask() {
						return wrongKind("set a description on", it, "task")
					}
					desc, _ = cmd.Flags().GetString("set-desc")
				}
				if err := s.Edit(ctx, it.ID, title, desc); err != nil {
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
	return c
}
