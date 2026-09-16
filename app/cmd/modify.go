package cmd

import (
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
			path := fileArg(cmd)
			doc, err := todo.Load(path)
			if err != nil {
				return err
			}
			it, err := resolveSelector(cmd, doc, "")
			if err != nil {
				return err
			}
			title := it.Title
			if !doc.Remove(it) {
				return &cliError{exitGeneric, "could not remove the item"}
			}
			if err := doc.Save(path); err != nil {
				return err
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "rm: %s\n", title)
			return nil
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
			path := fileArg(cmd)
			doc, err := todo.Load(path)
			if err != nil {
				return err
			}
			n := doc.RemoveDone()
			if err := doc.Save(path); err != nil {
				return err
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "prune: removed %d task(s)\n", n)
			return nil
		},
	}
}
