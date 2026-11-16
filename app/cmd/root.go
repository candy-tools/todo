// Package cmd is the command-line entry point: it parses `todo [list-file]` and
// launches the interactive TUI on that list file.
package cmd

import (
	"errors"
	"fmt"
	"os"

	"github.com/candy-tools/todo/app/metainfo"
	"github.com/candy-tools/todo/app/tui"
	"github.com/candy-tools/todo/internal/todo"
	"github.com/spf13/cobra"
)

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
		Use:           "todo [list-file]",
		Short:         "A minimalistic terminal TODO manager",
		Long:          "todo opens a JSON list file as a keyboard-driven task list of categories,\ntasks and subtasks, shown as a kanban board with Backlog, Next, In Progress and\nDone tabs. Each task has a state; set one from the command line with\n`todo status --to <state>`.\n\nWith no argument it uses .todo.json in the current directory. When that file\ndoes not exist yet and a TODO.md is present, TODO.md is imported on first run.\nUse `todo import <file.md>` to import a markdown file explicitly.",
		Version:       metainfo.Version,
		Args:          cobra.MaximumNArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(c *cobra.Command, args []string) error {
			path, isDefault, err := resolvePath(c, args)
			if err != nil {
				return err
			}
			// Opening does no I/O: a missing list file shows as an empty list,
			// and nothing is created until the first write (unless TODO.md is
			// imported first). Its writes wait only tui.BusyTimeout for another
			// writer, so a stuck one elsewhere does not freeze the UI.
			st, notice, err := openStore(c, path, isDefault, todo.WithBusyTimeout(tui.BusyTimeout))
			if err != nil {
				return err
			}
			return tui.Run(st, notice)
		},
	}
	cmd.PersistentFlags().StringP("file", "f", defaultFile, "the todo list file to operate on")
	cmd.SetFlagErrorFunc(func(_ *cobra.Command, e error) error {
		return &cliError{exitUsage, e.Error()}
	})
	cmd.AddCommand(versionCmd(), listCmd(), statusCmd(), doneCmd(), progressCmd(), deferCmd(), reopenCmd(), addCmd(), addCategoryCmd(), rmCmd(), pruneCmd(), editCmd(), importCmd())
	return cmd
}
