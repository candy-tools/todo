package cmd

import (
	"context"
	"fmt"

	"github.com/candy-tools/todo/internal/todo"
	"github.com/spf13/cobra"
)

func rmCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "rm",
		Short: "remove an item and its subtree",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return withStore(cmd, func(ctx context.Context, s *todo.Store, doc *todo.Document) error {
				it, err := resolveSelector(cmd, doc, "")
				if err != nil {
					return err
				}
				if _, err := s.Delete(ctx, it.ID); err != nil {
					return err
				}
				if wantJSON(cmd) {
					return writeJSON(cmd.OutOrStdout(), map[string]record{"removed": toDetail(doc, it, numberMap(doc))})
				}
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "rm: %s\n", todo.Printable(it.DisplayTitle()))
				return nil
			})
		},
	}
	addSelectorFlags(c, "")
	addJSONFlag(c)
	return c
}

func pruneCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "prune (--sprint <id|name> | --all)",
		Short: "remove fully-completed tasks, from one sprint or from everywhere",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			all, _ := cmd.Flags().GetBool("all")
			inSprint := cmd.Flags().Changed("sprint")
			switch {
			case all && inSprint:
				return usageErr("--sprint and --all are mutually exclusive")
			case !all && !inSprint:
				return usageErr("specify --sprint <id|name> or --all")
			}
			return withStore(cmd, func(ctx context.Context, s *todo.Store, doc *todo.Document) error {
				var n int
				var err error
				if all {
					n, _, err = s.Prune(ctx)
				} else {
					var sp *todo.Sprint
					if sp, err = resolveSprint(cmd, doc); err != nil {
						return err
					}
					n, _, err = s.PruneSprint(ctx, sp.ID)
				}
				if err != nil {
					return err
				}
				if wantJSON(cmd) {
					return writeJSON(cmd.OutOrStdout(), map[string]int{"removed": n})
				}
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "prune: removed %d task(s)\n", n)
				return nil
			})
		},
	}
	c.Flags().String("sprint", "", "prune only this sprint's done tasks, by id or name")
	c.Flags().Bool("all", false, "prune every done task in the list, in all sprints and the backlog")
	addJSONFlag(c)
	return c
}

func editCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "edit",
		Short: "change an item's title, a task's description, plan or reference, or a category's reference prefix",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return withStore(cmd, func(ctx context.Context, s *todo.Store, doc *todo.Document) error {
				it, err := resolveSelector(cmd, doc, "")
				if err != nil {
					return err
				}
				setTitle := cmd.Flags().Changed("set-title")
				setDesc := cmd.Flags().Changed("set-desc")
				setPlan := cmd.Flags().Changed("set-plan")
				setRef := cmd.Flags().Changed("set-ref")
				setPrefix := cmd.Flags().Changed("set-prefix")
				if !setTitle && !setDesc && !setPlan && !setRef && !setPrefix {
					return usageErr("specify --set-title, --set-desc, --set-plan, --set-ref and/or --set-prefix")
				}
				// Only the flags given are written, so a field another process
				// changed since the read is kept.
				var ch todo.Changes
				title := it.Title
				if setTitle {
					title, _ = cmd.Flags().GetString("set-title")
					if title, err = todo.CleanTitle(title); err != nil {
						return usageErr("--set-title: " + err.Error())
					}
					ch.Title = &title
				}
				if setDesc {
					desc, _ := cmd.Flags().GetString("set-desc")
					ch.Description = &desc
				}
				if setPlan {
					plan, _ := cmd.Flags().GetString("set-plan")
					ch.Plan = &plan
				}
				if setRef {
					ref, _ := cmd.Flags().GetString("set-ref")
					if ref, err = todo.CleanRef(ref); err != nil {
						return usageErr("--set-ref: " + err.Error())
					}
					ch.Ref = &ref
				}
				if setPrefix {
					prefix, _ := cmd.Flags().GetString("set-prefix")
					ch.Prefix = &prefix
				}
				// The store refuses a description, plan or reference for a
				// category, and a reference for a subtask or one another task
				// holds.
				res, err := s.Edit(ctx, it.ID, ch)
				if err != nil {
					return wrongKindIn(err, "edit", it)
				}
				return echoItem(cmd, res.Doc, it.ID, func() {
					shown := title
					if now, ok := res.Doc.ByID(it.ID); ok {
						shown = now.DisplayTitle()
					}
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "edit: %s\n", todo.Printable(shown))
				})
			})
		},
	}
	addSelectorFlags(c, "")
	addJSONFlag(c)
	c.Flags().String("set-title", "", "the new title")
	c.Flags().String("set-desc", "", "the new description (tasks only)")
	c.Flags().String("set-plan", "", "the new plan path or reference; empty clears it (tasks only)")
	c.Flags().String("set-ref", "", "the new reference, such as APP-001; unique in the list, empty clears it (tasks, not subtasks)")
	c.Flags().String("set-prefix", "", "the new reference prefix for tasks created in this category; empty clears it (categories only)")
	return c
}
