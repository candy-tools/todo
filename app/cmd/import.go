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
		Short: "import a markdown TODO file into the JSON list",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path, isDefault, err := resolvePath(cmd, nil)
			if err != nil {
				return err
			}
			notice, err := legacyDBNotice(path, isDefault)
			if err != nil {
				return err
			}
			printNotice(cmd, notice)
			force, _ := cmd.Flags().GetBool("force")
			_, rep, err := todo.ImportFile(path, args[0], force)
			if errors.Is(err, todo.ErrNotEmpty) {
				return &cliError{exitGeneric, fmt.Sprintf("%s already has items; use --force to replace them", path)}
			}
			if err != nil {
				return storeErr(err)
			}
			if rep.Backup != "" {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "backed up the replaced items to %s\n", rep.Backup)
			}
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), importNotice(rep, args[0]))
			return nil
		},
	}
	c.Flags().Bool("force", false, "replace the items already in the list")
	return c
}
