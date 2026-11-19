package cmd

import (
	"context"
	"fmt"

	"github.com/candy-tools/todo/internal/todo"
	"github.com/spf13/cobra"
)

func assignCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "assign --sprint <id|name>",
		Short: "put a task in a sprint (a backlog task becomes new; one in another sprint keeps its state)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return withStore(cmd, func(ctx context.Context, s *todo.Store, doc *todo.Document) error {
				it, err := resolveSelector(cmd, doc, "")
				if err != nil {
					return err
				}
				sp, err := resolveSprint(cmd, doc)
				if err != nil {
					return err
				}
				res, err := s.Assign(ctx, it.ID, sp.ID)
				if err != nil {
					return wrongKindIn(err, "assign", it)
				}
				if now, ok := res.Doc.ByID(it.ID); ok {
					it = now
				}
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "assign: [%s] %s  (%s)\n", it.Status.String(), todo.Printable(it.DisplayTitle()), todo.Printable(sp.Name))
				return nil
			})
		},
	}
	addSelectorFlags(c, "")
	c.Flags().String("sprint", "", "the sprint, by id or name (required)")
	return c
}

func unassignCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "unassign",
		Short: "return a task to the backlog as open",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return withStore(cmd, func(ctx context.Context, s *todo.Store, doc *todo.Document) error {
				it, err := resolveSelector(cmd, doc, "")
				if err != nil {
					return err
				}
				res, err := s.Unassign(ctx, it.ID)
				if err != nil {
					return wrongKindIn(err, "unassign", it)
				}
				if now, ok := res.Doc.ByID(it.ID); ok {
					it = now
				}
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "unassign: [%s] %s\n", it.Status.String(), todo.Printable(it.DisplayTitle()))
				return nil
			})
		},
	}
	addSelectorFlags(c, "")
	return c
}
