package cmd

import (
	"context"
	"fmt"

	"github.com/candy-tools/todo/internal/todo"
	"github.com/spf13/cobra"
)

// statusWrite writes a new status for task it through s and returns the status
// it set, for the confirmation line.
type statusWrite func(ctx context.Context, s *todo.Store, it *todo.Item) (todo.Status, error)

// setStatus resolves the selector to a task, writes its new status through
// write, and prints a confirmation.
func setStatus(cmd *cobra.Command, verb string, write statusWrite) error {
	return withStore(cmd, func(ctx context.Context, open storeOpener, doc *todo.Document) error {
		it, err := resolveSelector(cmd, doc, "")
		if err != nil {
			return err
		}
		if !it.IsTask() {
			return wrongKind("set the status of", it, "task")
		}
		s, err := open()
		if err != nil {
			return err
		}
		st, err := write(ctx, s, it)
		if err != nil {
			return err
		}
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s: [%s] %s  (%s)\n", verb, st.String(), todo.Printable(it.Title), categoryPath(it))
		return nil
	})
}

// statusNames lists the accepted state names for flag help and error messages.
const statusNames = "open|working|deferred|done|new|ready|review|followup|wontdo"

// set writes st for the task alone; its subtasks keep their states.
func set(st todo.Status) statusWrite {
	return func(ctx context.Context, s *todo.Store, it *todo.Item) (todo.Status, error) {
		_, err := s.SetStatuses(ctx, map[string]todo.Status{it.ID: st})
		return st, err
	}
}

// setThroughPolicy writes st for the task alone, through the store's complete
// and reopen policy: into Done it completes the task, out of Done it reopens
// it, and any other change is a plain state change. The subtasks keep their
// states in all three.
func setThroughPolicy(st todo.Status) statusWrite {
	return func(ctx context.Context, s *todo.Store, it *todo.Item) (todo.Status, error) {
		switch {
		case st == todo.Done:
			_, err := s.Complete(ctx, it.ID, false)
			return st, err
		case it.Status == todo.Done:
			_, err := s.SetState(ctx, it.ID, st, todo.StateOptions{})
			return st, err
		}
		return set(st)(ctx, s, it)
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
			return setStatus(cmd, "status", setThroughPolicy(st))
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
			return setStatus(cmd, "done", func(ctx context.Context, s *todo.Store, it *todo.Item) (todo.Status, error) {
				done, total := it.TaskCounts()
				if _, err := s.Complete(ctx, it.ID, cascade); err != nil {
					return todo.Done, err
				}
				if open := total - done; !cascade && open > 0 {
					_, _ = fmt.Fprintln(cmd.ErrOrStderr(), openSubtasksHint(open))
				}
				return todo.Done, nil
			})
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
			return setStatus(cmd, "reopen", func(ctx context.Context, s *todo.Store, it *todo.Item) (todo.Status, error) {
				// The store picks the state under the lock, from the task as
				// it is then; report the one it left.
				res, err := s.Reopen(ctx, it.ID, cascade)
				if err != nil {
					return it.Status, err
				}
				if now, ok := res.Doc.ByID(it.ID); ok {
					return now.Status, nil
				}
				return it.Status, nil
			})
		},
	}
	addSelectorFlags(c, "")
	c.Flags().Bool("cascade", false, "also open the subtasks (also on a task that is not done)")
	return c
}
