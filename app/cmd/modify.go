package cmd

import (
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

func editCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "edit",
		Short: "change an item's title and/or a task's description",
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
			setTitle := cmd.Flags().Changed("set-title")
			setDesc := cmd.Flags().Changed("set-desc")
			if !setTitle && !setDesc {
				return usageErr("specify --set-title and/or --set-desc")
			}
			if setTitle {
				newTitle, _ := cmd.Flags().GetString("set-title")
				if strings.TrimSpace(newTitle) == "" {
					return usageErr("--set-title must not be empty")
				}
				it.Title = newTitle
			}
			if setDesc {
				if !it.IsTask() {
					return wrongKind("set a description on", it, "task")
				}
				newDesc, _ := cmd.Flags().GetString("set-desc")
				it.Description = newDesc
			}
			if err := doc.Save(path); err != nil {
				return err
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "edit: %s\n", it.Title)
			return nil
		},
	}
	addSelectorFlags(c, "")
	c.Flags().String("set-title", "", "the new title")
	c.Flags().String("set-desc", "", "the new description (tasks only)")
	return c
}
