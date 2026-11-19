package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/candy-tools/todo/internal/todo"
	"github.com/spf13/cobra"
)

func addCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "add",
		Short: "add a task under a category (or as a subtask of a task)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return withStore(cmd, func(ctx context.Context, s *todo.Store, doc *todo.Document) error {
				parent, err := resolveSelector(cmd, doc, "parent-")
				if err != nil {
					return err
				}
				title, _ := cmd.Flags().GetString("title")
				if title, err = todo.CleanTitle(title); err != nil {
					return usageErr("--title: " + err.Error())
				}
				desc, _ := cmd.Flags().GetString("desc")
				sprintID := ""
				if cmd.Flags().Changed("sprint") {
					if parent.IsTask() {
						return usageErr("--sprint applies to top-level tasks; a subtask follows its task")
					}
					sp, err := resolveSprint(cmd, doc)
					if err != nil {
						return err
					}
					sprintID = sp.ID
				}
				statusName, _ := cmd.Flags().GetString("status")
				if !cmd.Flags().Changed("status") && sprintID != "" {
					statusName = todo.New.String()
				}
				st, ok := todo.StatusFromName(statusName)
				if !ok {
					return usageErr(fmt.Sprintf("invalid --status %q (%s)", statusName, statusNames))
				}
				// A top-level task's state and sprint go together: say which flag
				// is missing or wrong rather than what the store would refuse.
				switch {
				case parent.IsTask():
				case st.InSprint() && sprintID == "":
					return usageErr(fmt.Sprintf("--status %s needs --sprint <id|name>", st))
				case !st.InSprint() && sprintID != "":
					return usageErr("--sprint needs a sprint state (" + sprintStateNames() + ")")
				}
				plan, _ := cmd.Flags().GetString("plan")
				ref, _ := cmd.Flags().GetString("ref")
				if ref, err = todo.CleanRef(ref); err != nil {
					return usageErr("--ref: " + err.Error())
				}
				// The store places the task after the parent's last task as the tree
				// is when it writes, so a sibling changed meanwhile does not matter.
				id, res, err := s.AppendTask(ctx, todo.Fields{Title: title, Description: desc, Plan: plan, Ref: ref, Sprint: sprintID}, st, parent.ID)
				if err != nil {
					return err
				}
				if task, ok := res.Doc.ByID(id); ok {
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "add: [%s] %s  (%s)\n", task.Status.String(), todo.Printable(task.DisplayTitle()), categoryPath(task))
				}
				return nil
			})
		},
	}
	c.Flags().String("title", "", "the new task's title (required)")
	c.Flags().String("desc", "", "the new task's description")
	c.Flags().String("plan", "", "a path or reference to the task's plan")
	c.Flags().String("ref", "", "the new task's reference, such as APP-001; unique in the list (not for a subtask)")
	c.Flags().String("status", "open", "initial state: "+statusNames+" (default open; new with --sprint)")
	c.Flags().String("sprint", "", "put the new task in this sprint, by id or name (top-level tasks only)")
	addSelectorFlags(c, "parent-")
	return c
}

// sprintStateNames lists the states a task in a sprint can take, in board
// order, e.g. "new|ready|working|review|followup|done".
func sprintStateNames() string {
	var names []string
	for _, c := range todo.SprintColumns() {
		for _, st := range c.States() {
			names = append(names, st.String())
		}
	}
	return strings.Join(names, "|")
}

func addCategoryCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "add-category",
		Short: "add a category (top-level, or nested under a parent category)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return withStore(cmd, func(ctx context.Context, s *todo.Store, doc *todo.Document) error {
				title, _ := cmd.Flags().GetString("title")
				title, err := todo.CleanTitle(title)
				if err != nil {
					return usageErr("--title: " + err.Error())
				}
				var parent *todo.Item
				if cmd.Flags().Changed("parent-number") || cmd.Flags().Changed("parent-id") || cmd.Flags().Changed("parent-title") {
					if parent, err = resolveSelector(cmd, doc, "parent-"); err != nil {
						return err
					}
				}
				// The store places the category last under its parent as it
				// finds the tree when it writes, so a concurrent change cannot
				// make the add fail, and decides whether the parent can hold a
				// category.
				parentID := ""
				if parent != nil {
					parentID = parent.ID
				}
				id, res, err := s.AppendCategory(ctx, title, parentID)
				if err != nil {
					if parent != nil {
						return wrongKindIn(err, "nest a category under", parent)
					}
					return err
				}
				if cat, ok := res.Doc.ByID(id); ok {
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "add-category: %s %s\n", strings.Repeat("#", depthOf(cat)+1), title)
				}
				return nil
			})
		},
	}
	c.Flags().String("title", "", "the category title (required)")
	addSelectorFlags(c, "parent-")
	return c
}
