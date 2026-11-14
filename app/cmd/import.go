package cmd

import (
	"errors"
	"fmt"

	"github.com/candy-tools/todo/internal/todo"
	"github.com/spf13/cobra"
)

func importCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "import <file.md>",
		Short: "import a markdown TODO file into the database",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			db, _, err := resolvePath(cmd, nil)
			if err != nil {
				return err
			}
			force, _ := cmd.Flags().GetBool("force")
			s, rep, err := todo.ImportFile(db, args[0], force)
			if errors.Is(err, todo.ErrNotEmpty) {
				return &cliError{exitGeneric, fmt.Sprintf("%s already has items; use --force to replace them", db)}
			}
			if err != nil {
				return err
			}
			_ = s.Close()
			if rep.Skipped {
				return &cliError{exitGeneric, fmt.Sprintf("%s was created by another todo during the import; nothing was imported", db)}
			}
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), importNotice(rep, args[0]))
			return nil
		},
	}
	c.Flags().Bool("force", false, "replace the items already in the database")
	return c
}
