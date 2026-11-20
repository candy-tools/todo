// Package cmd is the command-line entry point: it parses `todo [list-file]` and
// launches the interactive TUI on that list file.
package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/candy-tools/todo/app/metainfo"
	"github.com/candy-tools/todo/app/tui"
	"github.com/candy-tools/todo/internal/todo"
	"github.com/spf13/cobra"
)

// Execute is the entry point for the command line.
func Execute() {
	if code := execute(os.Args[1:], os.Stdout, os.Stderr); code != 0 {
		os.Exit(code)
	}
}

// execute runs the CLI with args and returns the process exit code (0 on
// success). A failure is written to stderr: as one compact jsonError line when
// the command ran in JSON mode (see wantsJSONErrors), else as the plain message.
func execute(args []string, stdout, stderr io.Writer) int {
	root := newRootCommand()
	root.SetArgs(args)
	root.SetOut(stdout)
	root.SetErr(stderr)
	c, err := root.ExecuteC()
	if err == nil {
		return 0
	}
	exit, name := errorCode(err)
	if wantsJSONErrors(c, args) {
		var je jsonError
		je.Error.Code, je.Error.Exit, je.Error.Message = name, exit, strings.TrimRight(err.Error(), "\n")
		b, _ := json.Marshal(je)
		fmt.Fprintln(stderr, string(b))
	} else {
		fmt.Fprintln(stderr, err)
	}
	return exit
}

// wantsJSONErrors reports whether c ran in JSON mode: its json or flat flag is
// true (so --json=false gives plain errors), or args holds --json/--flat, bare
// or as =true (a flag-parse error can stop cobra before the flag is set).
func wantsJSONErrors(c *cobra.Command, args []string) bool {
	if c != nil {
		for _, name := range []string{"json", "flat"} {
			if v, err := c.Flags().GetBool(name); err == nil && v {
				return true
			}
		}
	}
	for _, a := range args {
		switch a {
		case "--json", "--json=true", "--flat", "--flat=true":
			return true
		}
	}
	return false
}

func newRootCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "todo [list-file]",
		Short:         "A minimalistic terminal TODO manager",
		Long:          "todo opens a JSON list file as a keyboard-driven task list of categories,\ntasks and subtasks, organized into a Backlog and sprints. A task joins a sprint with\n`todo assign` and has a state; set one from the command line with\n`todo status --to <state>`.\n\nWith no argument it uses .todo.json in the current directory. When that file\ndoes not exist yet and a TODO.md is present, TODO.md is imported on first run.\nUse `todo import <file.md>` to import a markdown file explicitly.\n\nFor scripts and agents: `todo next` lists what to work on, `todo show` one\nitem, `todo list --flat` every item as flat JSON; add --json to any command\nthat changes an item or a sprint to get the result back as JSON.",
		Version:       metainfo.Version,
		Args:          cobra.MaximumNArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(c *cobra.Command, args []string) error {
			path, isDefault, err := resolvePath(c, args)
			if err != nil {
				return err
			}
			// A missing list file shows as an empty list, and nothing is
			// created until the first write (unless TODO.md is imported
			// first); an existing one gets todo's note at startup (see
			// tui.Run). Its writes wait only tui.BusyTimeout for another
			// writer, so a stuck one elsewhere does not freeze the UI.
			st, notice, err := openList(path, isDefault, todo.WithBusyTimeout(tui.BusyTimeout))
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
	cmd.AddCommand(versionCmd(), listCmd(), statusCmd(), doneCmd(), progressCmd(), deferCmd(), reopenCmd(), addCmd(), addCategoryCmd(), rmCmd(), pruneCmd(), editCmd(), moveCmd(), importCmd(), sprintCmd(), assignCmd(), unassignCmd(), settingsCmd(), showCmd(), nextCmd())
	return cmd
}
