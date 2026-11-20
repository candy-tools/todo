package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/candy-tools/todo/internal/todo"
	"github.com/spf13/cobra"
)

// jsonSprint is the shape `sprint list --json` emits for one sprint.
type jsonSprint struct {
	ID     string         `json:"id"`
	Name   string         `json:"name"`
	Goal   string         `json:"goal"`
	Prefix string         `json:"prefix,omitempty"`
	Status string         `json:"status"`
	Counts map[string]int `json:"counts"`
}

// sprintCounts returns the sprint's task count in each board column, keyed by
// the column names the CLI uses.
func sprintCounts(doc *todo.Document, id string) map[string]int {
	c := doc.ColumnCounts(id)
	return map[string]int{
		todo.ColNext.String():       c[todo.ColNext],
		todo.ColInProgress.String(): c[todo.ColInProgress],
		todo.ColDone.String():       c[todo.ColDone],
	}
}

// writeSprintTable prints sprints as a table: id, status, per-column counts,
// then the name and, when there is one, the goal.
func writeSprintTable(w io.Writer, doc *todo.Document, sprints []*todo.Sprint) {
	_, _ = fmt.Fprintln(w, "     id  status  next  progress  done  name — goal")
	next, progress, done := todo.ColNext.String(), todo.ColInProgress.String(), todo.ColDone.String()
	for _, sp := range sprints {
		c := sprintCounts(doc, sp.ID)
		label := todo.Printable(sp.Name)
		if sp.Goal != "" {
			label += " — " + todo.Printable(sp.Goal)
		}
		_, _ = fmt.Fprintf(w, "%7s  %-6s  %4d  %8d  %4d  %s\n", sp.ID, sp.Status.String(), c[next], c[progress], c[done], label)
	}
}

// writeSprintJSON prints sprints as a JSON array.
func writeSprintJSON(w io.Writer, doc *todo.Document, sprints []*todo.Sprint) error {
	out := []jsonSprint{}
	for _, sp := range sprints {
		out = append(out, sprintOf(doc, sp))
	}
	return writeJSON(w, out)
}

// sprintWrite applies one write to the sprint sp.
type sprintWrite func(ctx context.Context, s *todo.Store, sp *todo.Sprint) (todo.WriteResult, error)

// withSprint resolves --sprint and runs write on it, then prints
// "<verb>: <name>", or under --json the sprint as the written list has it (for
// rm, as it was before the write, wrapped in {"removed": ...}).
func withSprint(cmd *cobra.Command, verb string, write sprintWrite) error {
	return withStore(cmd, func(ctx context.Context, s *todo.Store, doc *todo.Document) error {
		sp, err := resolveSprint(cmd, doc)
		if err != nil {
			return err
		}
		res, err := write(ctx, s, sp)
		if err != nil {
			return err
		}
		if wantJSON(cmd) {
			if verb == "rm" {
				return writeJSON(cmd.OutOrStdout(), map[string]jsonSprint{"removed": sprintOf(doc, sp)})
			}
			now, ok := res.Doc.SprintByID(sp.ID)
			if !ok {
				return fmt.Errorf("sprint %s is missing from the written list", sp.ID)
			}
			return writeJSON(cmd.OutOrStdout(), sprintOf(res.Doc, now))
		}
		name := sp.Name
		if n, _ := cmd.Flags().GetString("set-name"); cmd.Flags().Changed("set-name") {
			name = n
		}
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s: %s\n", verb, todo.Printable(name))
		return nil
	})
}

// addSprintFlag registers --sprint.
func addSprintFlag(cmd *cobra.Command) {
	cmd.Flags().String("sprint", "", "the sprint, by id or name (ignoring case)")
}

func sprintCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "sprint",
		Short: "manage sprints (list|add|edit|close|reopen|rm)",
		Args:  cobra.NoArgs,
		RunE:  func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	c.AddCommand(sprintListCmd(), sprintAddCmd(), sprintEditCmd(), sprintCloseCmd(), sprintReopenCmd(), sprintRmCmd())
	return c
}

func sprintListCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "list",
		Short: "list the open sprints with their task counts (--all includes closed ones)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return withStore(cmd, func(_ context.Context, _ *todo.Store, doc *todo.Document) error {
				sprints := doc.Sprints
				if all, _ := cmd.Flags().GetBool("all"); !all {
					sprints = doc.OpenSprints()
				}
				if asJSON, _ := cmd.Flags().GetBool("json"); asJSON {
					return writeSprintJSON(cmd.OutOrStdout(), doc, sprints)
				}
				writeSprintTable(cmd.OutOrStdout(), doc, sprints)
				return nil
			})
		},
	}
	c.Flags().Bool("all", false, "include closed sprints")
	c.Flags().Bool("json", false, "output the sprints as JSON")
	return c
}

func sprintAddCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "add --name <name> [--goal <goal>] [--prefix <prefix>]",
		Short: "create an open sprint and print its id",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			name, _ := cmd.Flags().GetString("name")
			goal, _ := cmd.Flags().GetString("goal")
			prefix, _ := cmd.Flags().GetString("prefix")
			if name == "" {
				return usageErr("specify --name")
			}
			return withStore(cmd, func(ctx context.Context, s *todo.Store, _ *todo.Document) error {
				id, res, err := s.AddSprintWith(ctx, todo.Fields{Title: name, Description: goal, Prefix: prefix})
				if err != nil {
					return err
				}
				if wantJSON(cmd) {
					sp, ok := res.Doc.SprintByID(id)
					if !ok {
						return fmt.Errorf("sprint %s is missing from the written list", id)
					}
					return writeJSON(cmd.OutOrStdout(), sprintOf(res.Doc, sp))
				}
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), id)
				return nil
			})
		},
	}
	c.Flags().String("name", "", "the sprint's name")
	c.Flags().String("goal", "", "the sprint's goal")
	c.Flags().String("prefix", "", "the reference prefix for tasks created in this sprint, such as S13 (a letter and up to 9 letters or digits)")
	addJSONFlag(c)
	return c
}

func sprintEditCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "edit --sprint <id|name> [--set-name <name>] [--set-goal <goal>] [--set-prefix <prefix>]",
		Short: "change a sprint's name, goal or reference prefix",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			var ch todo.SprintChanges
			if cmd.Flags().Changed("set-name") {
				v, _ := cmd.Flags().GetString("set-name")
				ch.Name = &v
			}
			if cmd.Flags().Changed("set-goal") {
				v, _ := cmd.Flags().GetString("set-goal")
				ch.Goal = &v
			}
			if cmd.Flags().Changed("set-prefix") {
				v, _ := cmd.Flags().GetString("set-prefix")
				ch.Prefix = &v
			}
			if ch.Name == nil && ch.Goal == nil && ch.Prefix == nil {
				return usageErr("specify --set-name, --set-goal or --set-prefix")
			}
			return withSprint(cmd, "edit", func(ctx context.Context, s *todo.Store, sp *todo.Sprint) (todo.WriteResult, error) {
				return s.EditSprint(ctx, sp.ID, ch)
			})
		},
	}
	addSprintFlag(c)
	addJSONFlag(c)
	c.Flags().String("set-name", "", "the new name")
	c.Flags().String("set-goal", "", "the new goal (empty clears it)")
	c.Flags().String("set-prefix", "", "the new reference prefix for tasks created in this sprint (empty clears it)")
	return c
}

func sprintCloseCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "close --sprint <id|name>",
		Short: "close a sprint; refused while it has tasks not done",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return withSprint(cmd, "close", func(ctx context.Context, s *todo.Store, sp *todo.Sprint) (todo.WriteResult, error) {
				res, err := s.CloseSprint(ctx, sp.ID)
				if errors.Is(err, todo.ErrSprintNotDone) {
					return res, fmt.Errorf("cannot close %q: %w", sp.Name, err)
				}
				return res, err
			})
		},
	}
	addSprintFlag(c)
	addJSONFlag(c)
	return c
}

func sprintReopenCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "reopen --sprint <id|name>",
		Short: "reopen a closed sprint",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return withSprint(cmd, "reopen", func(ctx context.Context, s *todo.Store, sp *todo.Sprint) (todo.WriteResult, error) {
				return s.ReopenSprint(ctx, sp.ID)
			})
		},
	}
	addSprintFlag(c)
	addJSONFlag(c)
	return c
}

func sprintRmCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "rm --sprint <id|name>",
		Short: "delete a sprint; its tasks return to the backlog, refused while it holds done tasks",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return withSprint(cmd, "rm", func(ctx context.Context, s *todo.Store, sp *todo.Sprint) (todo.WriteResult, error) {
				return s.DeleteSprint(ctx, sp.ID)
			})
		},
	}
	addSprintFlag(c)
	addJSONFlag(c)
	return c
}
