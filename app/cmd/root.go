// Package cmd is the command-line entry point: it parses `todo <file.md>` and
// launches the interactive TUI on that file.
package cmd

import (
	"errors"
	"fmt"
	"os"

	"github.com/candy-tools/todo/app/metainfo"
	"github.com/candy-tools/todo/app/tui"
	"github.com/spf13/cobra"
)

// defaultFile is opened when no file argument is given.
const defaultFile = "TODO.md"

// Execute is the entry point for the command line.
func Execute() {
	err := newRootCommand().Execute()
	if err == nil {
		return
	}
	fmt.Fprintln(os.Stderr, err)
	var ce *cliError
	if errors.As(err, &ce) {
		os.Exit(ce.code)
	}
	os.Exit(exitGeneric)
}

func newRootCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "todo [file.md]",
		Short:         "A minimalistic markdown TODO manager",
		Long:          "todo opens a markdown file as a keyboard-driven task list: headers are\ncategories, `- [ ]` items are tasks, and nested items are subtasks.\n\nWith no argument it opens TODO.md in the current directory.",
		Version:       metainfo.Version,
		Args:          cobra.MaximumNArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(c *cobra.Command, args []string) error {
			if len(args) == 0 && c.Flags().Changed("file") {
				f, _ := c.Flags().GetString("file")
				return tui.Run(f)
			}
			return tui.Run(filePath(args))
		},
	}
	cmd.PersistentFlags().StringP("file", "f", defaultFile, "the todo file to operate on")
	cmd.SetFlagErrorFunc(func(_ *cobra.Command, e error) error {
		return &cliError{exitUsage, e.Error()}
	})
	cmd.AddCommand(versionCmd(), listCmd(), doneCmd(), progressCmd(), deferCmd(), reopenCmd(), addCmd(), addCategoryCmd())
	return cmd
}

// filePath resolves the file to open: the given argument, or TODO.md when none
// is provided.
func filePath(args []string) string {
	if len(args) == 1 {
		return args[0]
	}
	return defaultFile
}
