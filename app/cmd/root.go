// Package cmd is the command-line entry point: it parses `todo <file.md>` and
// launches the interactive TUI on that file.
package cmd

import (
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
	if err := newRootCommand().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
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
	cmd.AddCommand(versionCmd(), listCmd())
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
