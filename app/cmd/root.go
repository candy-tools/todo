// Package cmd is the command-line entry point: it parses `todo [list-file]` and
// launches the interactive TUI on that list file.
package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
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
	if wantsJSONErrors(c, args, err) {
		var je jsonError
		je.Error.Code, je.Error.Exit, je.Error.Message = name, exit, strings.TrimRight(err.Error(), "\n")
		b, _ := json.Marshal(je)
		_, _ = fmt.Fprintln(stderr, string(b))
	} else {
		_, _ = fmt.Fprintln(stderr, err)
	}
	return exit
}

// wantsJSONErrors reports whether err is printed as a jsonError: c ran in JSON
// mode (see jsonMode), or err is a flag-parse error and args holds --json or
// --flat, bare or with a true value as strconv.ParseBool reads it (cobra stops
// at the bad flag, so a later one is never set). Any other error trusts the parsed flags alone, so a --json taken as
// another flag's value gives a plain error.
func wantsJSONErrors(c *cobra.Command, args []string, err error) bool {
	if jsonMode(c) {
		return true
	}
	var fe *flagError
	if !errors.As(err, &fe) {
		return false
	}
	for _, a := range args {
		if a == "--json" || a == "--flat" {
			return true
		}
		v, ok := strings.CutPrefix(a, "--json=")
		if !ok {
			v, ok = strings.CutPrefix(a, "--flat=")
		}
		if on, err := strconv.ParseBool(v); ok && err == nil && on {
			return true
		}
	}
	return false
}

// jsonMode reports whether c's json or flat flag is true (so --json=false is
// off): its output, and any error, is meant for an agent.
func jsonMode(c *cobra.Command) bool {
	if c == nil {
		return false
	}
	for _, name := range []string{"json", "flat"} {
		if v, err := c.Flags().GetBool(name); err == nil && v {
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
			// first); opening an existing one does not write it. Its writes
			// wait only tui.BusyTimeout for another writer, so a stuck one
			// elsewhere does not freeze the UI.
			st, notice, err := openList(path, isDefault, todo.WithBusyTimeout(tui.BusyTimeout))
			if err != nil {
				return err
			}
			return tui.Run(st, notice)
		},
	}
	cmd.PersistentFlags().StringP("file", "f", defaultFile, "the todo list file to operate on")
	cmd.SetFlagErrorFunc(func(_ *cobra.Command, e error) error {
		return &flagError{usageErr(e.Error())}
	})
	cmd.AddCommand(versionCmd(), listCmd(), statusCmd(), doneCmd(), progressCmd(), deferCmd(), reopenCmd(), addCmd(), addCategoryCmd(), rmCmd(), pruneCmd(), editCmd(), moveCmd(), importCmd(), sprintCmd(), assignCmd(), unassignCmd(), settingsCmd(), showCmd(), nextCmd())
	argsAsUsage(cmd)
	return cmd
}

// argsAsUsage makes a wrong number of positional arguments a usage error
// (exit 2) on c and every command under it: cobra's argument validators
// return plain errors. Flags are parsed by then, so jsonMode decides the
// error's format.
func argsAsUsage(c *cobra.Command) {
	if args := c.Args; args != nil {
		c.Args = func(cmd *cobra.Command, a []string) error {
			if err := args(cmd, a); err != nil {
				return usageErr(err.Error())
			}
			return nil
		}
	}
	for _, sub := range c.Commands() {
		argsAsUsage(sub)
	}
}
