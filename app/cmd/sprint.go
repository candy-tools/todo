package cmd

import (
	"context"
	"fmt"
	"io"

	"github.com/candy-tools/todo/internal/todo"
	"github.com/spf13/cobra"
)

// jsonSprint is the shape every `--json` sprint output uses: one per sprint in
// `sprint list`, the sprint itself from `sprint add` and `edit`, the sprint wrapped in {"removed": ...} from `sprint rm`, and the
// sprint fields embedded in each sprint of `todo next`.
type jsonSprint struct {
	ID     string         `json:"id"`
	Ref    string         `json:"ref"`
	Name   string         `json:"name"`
	Goal   string         `json:"goal"`
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

// writeSprintTable prints sprints as a table: reference, id, status (derived
// from the tasks, see todo.Document.SprintStatus),
// per-column counts, then the name and, when there is one, the goal.
func writeSprintTable(w io.Writer, doc *todo.Document, sprints []*todo.Sprint) {
	width := len("ref")
	for _, sp := range sprints {
		width = max(width, len(sp.Ref))
	}
	_, _ = fmt.Fprintf(w, "%-*s       id  status    next  progress  done  name — goal\n", width, "ref")
	next, progress, done := todo.ColNext.String(), todo.ColInProgress.String(), todo.ColDone.String()
	for _, sp := range sprints {
		c := sprintCounts(doc, sp.ID)
		label := todo.Printable(sp.Name)
		if sp.Goal != "" {
			label += " — " + todo.Printable(sp.Goal)
		}
		_, _ = fmt.Fprintf(w, "%-*s  %7s  %-8s  %4d  %8d  %4d  %s\n", width, sp.Ref, sp.ID, doc.SprintStatus(sp.ID).String(), c[next], c[progress], c[done], label)
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
// "<verb>: <ref> <name>", or under --json the sprint as the written list has it (for
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
		shown := *sp
		if n, _ := cmd.Flags().GetString("set-name"); cmd.Flags().Changed("set-name") {
			shown.Name = n
		}
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s: %s\n", verb, sprintLabel(&shown))
		return nil
	})
}

// addSprintFlag registers --sprint.
func addSprintFlag(cmd *cobra.Command) {
	cmd.Flags().String("sprint", "", "the sprint, by reference (TODO-001), id or name (ignoring case)")
}

func sprintCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "sprint",
		Short: "manage sprints (list|add|edit|rm); a sprint's status (open, ready, progress, done) follows its tasks",
		Args:  cobra.NoArgs,
		RunE:  func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	c.AddCommand(sprintListCmd(), sprintAddCmd(), sprintEditCmd(), sprintRmCmd())
	return c
}

func sprintListCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "list",
		Short: "list the sprints not done with their task counts (--all includes done ones)",
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
	c.Flags().Bool("all", false, "include done sprints")
	c.Flags().Bool("json", false, "output the sprints as JSON")
	return c
}

func sprintAddCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "add --name <name> [--goal <goal>]",
		Short: "create an open sprint and print its reference (TODO-001)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			name, _ := cmd.Flags().GetString("name")
			goal, _ := cmd.Flags().GetString("goal")
			if name == "" {
				return usageErr("specify --name")
			}
			return withStore(cmd, func(ctx context.Context, s *todo.Store, _ *todo.Document) error {
				id, res, err := s.AddSprint(ctx, name, goal)
				if err != nil {
					return err
				}
				sp, ok := res.Doc.SprintByID(id)
				if !ok {
					return fmt.Errorf("sprint %s is missing from the written list", id)
				}
				if wantJSON(cmd) {
					return writeJSON(cmd.OutOrStdout(), sprintOf(res.Doc, sp))
				}
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), sp.Ref)
				return nil
			})
		},
	}
	c.Flags().String("name", "", "the sprint's name")
	c.Flags().String("goal", "", "the sprint's goal")
	addJSONFlag(c)
	return c
}

func sprintEditCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "edit --sprint <ref|id|name> [--set-name <name>] [--set-goal <goal>]",
		Short: "change a sprint's name or goal",
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
			if ch.Name == nil && ch.Goal == nil {
				return usageErr("specify --set-name or --set-goal")
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
	return c
}

func sprintRmCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "rm --sprint <ref|id|name>",
		Short: "delete a sprint and all its tasks; refused while it has tasks not done",
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
