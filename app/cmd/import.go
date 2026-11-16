package cmd

import (
	"errors"
	"fmt"

	"github.com/candy-tools/todo/internal/todo"
	"github.com/spf13/cobra"
)

// importFile imports a markdown file into a store; tests replace it to make
// the import fail the way a store can.
var importFile = todo.ImportFile

func importCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "import <file.md>",
		Short: "import a markdown TODO file into the JSON list",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			db, isDefault, err := resolvePath(cmd, nil)
			if err != nil {
				return err
			}
			notice, err := legacyDBNotice(db, isDefault)
			if err != nil {
				return err
			}
			printNotice(cmd, notice)
			force, _ := cmd.Flags().GetBool("force")
			s, rep, err := importFile(db, args[0], force)
			if errors.Is(err, todo.ErrNotEmpty) {
				return &cliError{exitGeneric, fmt.Sprintf("%s already has items; use --force to replace them", db)}
			}
			if c := conflictErr(err); c != nil {
				return c
			}
			if err != nil {
				return err
			}
			_ = s.Close()
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
