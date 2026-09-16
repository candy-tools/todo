package cmd

import (
	"fmt"

	"github.com/candy-tools/todo/internal/todo"
	"github.com/spf13/cobra"
)

// setStatus resolves the selector to a task, sets its status, saves, and prints
// a confirmation. When cascade is true the whole subtree is set (only done and
// reopen expose --cascade).
func setStatus(cmd *cobra.Command, verb string, target todo.Status, cascade bool) error {
	path := fileArg(cmd)
	doc, err := todo.Load(path)
	if err != nil {
		return err
	}
	it, err := resolveSelector(cmd, doc, "")
	if err != nil {
		return err
	}
	if !it.IsTask() {
		return wrongKind("set the status of", it, "task")
	}
	if cascade {
		todo.CascadeSetDone(it, target == todo.Done)
	} else {
		it.Status = target
	}
	if err := doc.Save(path); err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "%s: [%s] %s  (%s)\n", verb, it.Status.Marker(), it.Title, categoryPath(it))
	return nil
}

func doneCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "done",
		Short: "mark a task done",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cascade, _ := cmd.Flags().GetBool("cascade")
			return setStatus(cmd, "done", todo.Done, cascade)
		},
	}
	addSelectorFlags(c, "")
	c.Flags().Bool("cascade", false, "also complete the whole subtree")
	return c
}
