package cmd

import (
	"context"
	"fmt"
	"io"

	"github.com/candy-tools/todo/internal/todo"
	"github.com/spf13/cobra"
)

// statusWrite writes a new state for task it through s and returns the
// write's result.
type statusWrite func(ctx context.Context, s *todo.Store, it *todo.Item) (todo.WriteResult, error)

// setStatus resolves the selector to an item, writes its new state through
// write (the store refuses a category), and prints a confirmation with the
// state the write left.
func setStatus(cmd *cobra.Command, verb string, write statusWrite) error {
	return withStore(cmd, func(ctx context.Context, s *todo.Store, doc *todo.Document) error {
		it, err := resolveSelector(cmd, doc, "")
		if err != nil {
			return err
		}
		res, err := write(ctx, s, it)
		if err != nil {
			return wrongKindIn(err, "set the status of", it)
		}
		if now, ok := res.Doc.ByID(it.ID); ok {
			it = now
		}
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s: [%s] %s  (%s)\n", verb, it.Status.String(), todo.Printable(it.Title), categoryPath(it))
		return nil
	})
}

// statusNames lists the accepted state names for flag help and error messages.
const statusNames = "open|working|deferred|done|new|ready|review|followup|wontdo"

// set sets the task to st. The store decides under its lock whether that
// completes, reopens or just changes it (see todo.Store.SetState); the
// subtasks keep their states.
func set(st todo.Status) statusWrite {
	return func(ctx context.Context, s *todo.Store, it *todo.Item) (todo.WriteResult, error) {
		return s.SetState(ctx, it.ID, st, todo.StateOptions{})
	}
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
			return setStatus(cmd, "status", set(st))
		},
	}
	addSelectorFlags(c, "")
	c.Flags().String("to", "", "the new state: "+statusNames+" (progress = working)")
	return c
}

// openSubtasksHint is the stderr line `done` without --cascade prints when the
// task it completed still has n subtasks that are not done.
func openSubtasksHint(n int) string {
	if n == 1 {
		return "1 subtask still open; --cascade completes it"
	}
	return fmt.Sprintf("%d subtasks still open; --cascade completes them", n)
}

func doneCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "done",
		Short: "mark a task done",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cascade, _ := cmd.Flags().GetBool("cascade")
			return setStatus(cmd, "done", complete(cascade, cmd.ErrOrStderr()))
		},
	}
	addSelectorFlags(c, "")
	c.Flags().Bool("cascade", false, "also complete the whole subtree")
	return c
}

// complete completes the task, with cascade its whole subtree too. Without
// cascade it writes openSubtasksHint to hint when the list the write left
// still has subtasks of the task open.
func complete(cascade bool, hint io.Writer) statusWrite {
	return func(ctx context.Context, s *todo.Store, it *todo.Item) (todo.WriteResult, error) {
		res, err := s.SetState(ctx, it.ID, todo.Done, todo.StateOptions{Cascade: cascade})
		if err != nil {
			return res, err
		}
		if now, ok := res.Doc.ByID(it.ID); ok && !cascade {
			if done, total := now.TaskCounts(); total > done {
				_, _ = fmt.Fprintln(hint, openSubtasksHint(total-done))
			}
		}
		return res, nil
	}
}

func progressCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "progress",
		Short: "mark a task in progress",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return setStatus(cmd, "progress", set(todo.Working))
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
			return setStatus(cmd, "defer", set(todo.Deferred))
		},
	}
	addSelectorFlags(c, "")
	return c
}

func reopenCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "reopen",
		Short: "reopen a done task: back to working (a subtask: open)",
		Long: "reopen takes a done task back to working (a subtask to open). A task that is\n" +
			"not done keeps its state and nothing is written; with --cascade its subtasks\n" +
			"are reopened.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cascade, _ := cmd.Flags().GetBool("cascade")
			// The store picks the state under the lock, from the task as it
			// is then; the confirmation shows the one it left.
			return setStatus(cmd, "reopen", func(ctx context.Context, s *todo.Store, it *todo.Item) (todo.WriteResult, error) {
				return s.Reopen(ctx, it.ID, cascade)
			})
		},
	}
	addSelectorFlags(c, "")
	c.Flags().Bool("cascade", false, "also open the subtasks (also on a task that is not done)")
	return c
}
