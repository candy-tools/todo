package cmd

import (
	"context"
	"fmt"

	"github.com/candy-tools/todo/internal/todo"
	"github.com/spf13/cobra"
)

// setStatus resolves the selector to a task, sets its status in the store, and
// prints a confirmation. When cascade is true the whole subtree is set (only done and
// reopen expose --cascade).
func setStatus(cmd *cobra.Command, verb string, target func(*todo.Item) todo.Status, cascade bool) error {
	return withStore(cmd, func(ctx context.Context, open storeOpener, doc *todo.Document) error {
		it, err := resolveSelector(cmd, doc, "")
		if err != nil {
			return err
		}
		if !it.IsTask() {
			return wrongKind("set the status of", it, "task")
		}
		st := target(it)
		s, err := open()
		if err != nil {
			return err
		}
		if cascade {
			err = s.CascadeStatus(ctx, it.ID, st)
		} else {
			err = s.SetStatuses(ctx, map[uint]todo.Status{it.ID: st})
		}
		if err != nil {
			return err
		}
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s: [%s] %s  (%s)\n", verb, st.String(), it.Title, categoryPath(it))
		return nil
	})
}

// statusNames lists the accepted state names for flag help and error messages.
const statusNames = "open|working|deferred|done|new|ready|review|followup"

// fixed is a target that does not depend on the item.
func fixed(st todo.Status) func(*todo.Item) todo.Status {
	return func(*todo.Item) todo.Status { return st }
}

func statusCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "status --to <state>",
		Short: "set a task's state (" + statusNames + ")",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			name, _ := cmd.Flags().GetString("to")
			st, ok := todo.StatusFromName(name)
			if !ok {
				return usageErr(fmt.Sprintf("invalid --to %q (%s)", name, statusNames))
			}
			return setStatus(cmd, "status", fixed(st), false)
		},
	}
	addSelectorFlags(c, "")
	c.Flags().String("to", "", "the new state: "+statusNames+" (progress = working)")
	return c
}

func doneCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "done",
		Short: "mark a task done",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cascade, _ := cmd.Flags().GetBool("cascade")
			return setStatus(cmd, "done", fixed(todo.Done), cascade)
		},
	}
	addSelectorFlags(c, "")
	c.Flags().Bool("cascade", false, "also complete the whole subtree")
	return c
}

func progressCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "progress",
		Short: "mark a task in progress",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return setStatus(cmd, "progress", fixed(todo.Working), false)
		},
	}
	addSelectorFlags(c, "")
	return c
}

func deferCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "defer",
		Short: "mark a task deferred",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return setStatus(cmd, "defer", fixed(todo.Deferred), false)
		},
	}
	addSelectorFlags(c, "")
	return c
}

func reopenCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "reopen",
		Short: "reopen a task: back to working (a subtask: open)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cascade, _ := cmd.Flags().GetBool("cascade")
			// A subtask can only be open or done, so reopening one opens it.
			return setStatus(cmd, "reopen", func(it *todo.Item) todo.Status {
				if it.IsSubtask() {
					return todo.Open
				}
				return todo.Working
			}, cascade)
		},
	}
	addSelectorFlags(c, "")
	c.Flags().Bool("cascade", false, "also open the subtasks")
	return c
}
