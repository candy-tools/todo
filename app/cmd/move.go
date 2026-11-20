package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/candy-tools/todo/internal/todo"
	"github.com/spf13/cobra"
)

func moveCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "move",
		Short: "move an item (and its subtree) under another parent or to another position",
		Long: "move places the item under the parent given by --parent-number, --parent-id or\n" +
			"--parent-title (or at the top level with --root, for a category). It goes last\n" +
			"among the parent's children of its kind, or first with --first, or right after\n" +
			"the sibling given by --after-number, --after-id or --after-title. A category's\n" +
			"subcategories always come before its tasks.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return withStore(cmd, func(ctx context.Context, s *todo.Store, doc *todo.Document) error {
				it, err := resolveSelector(cmd, doc, "")
				if err != nil {
					return err
				}
				parent, err := moveParent(cmd, doc)
				if err != nil {
					return err
				}
				after, first, err := movePosition(cmd, doc, it, parent)
				if err != nil {
					return err
				}
				parentID := ""
				if parent != nil {
					parentID = parent.ID
				}
				oldRef := it.Ref
				var res todo.WriteResult
				switch {
				case after != nil:
					res, err = s.Move(ctx, it.ID, parentID, after.ID)
				case first:
					res, err = s.Move(ctx, it.ID, parentID, "")
				default:
					// Placed last as the list is when the store writes, so a
					// sibling changed meanwhile does not matter.
					res, err = s.MoveLast(ctx, it.ID, parentID)
				}
				if err != nil {
					return wrongKindIn(err, "move", it)
				}
				if now, ok := res.Doc.ByID(it.ID); ok {
					it = now
				}
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "move: %s  (%s)\n", todo.Printable(it.DisplayTitle()), placeName(it.Parent))
				if oldRef != "" && it.Ref == "" {
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "dropped reference %s\n", oldRef)
				}
				return nil
			})
		},
	}
	addSelectorFlags(c, "")
	addSelectorFlags(c, "parent-")
	c.Flags().Bool("root", false, "move a category to the top level")
	c.Flags().Bool("first", false, "place it first among its kind under the parent")
	addSelectorFlags(c, "after-")
	return c
}

// moveParent returns the parent the move flags name: nil for --root, else the
// item the parent- selector names. Exactly one of them must be given.
func moveParent(cmd *cobra.Command, doc *todo.Document) (*todo.Item, error) {
	root, _ := cmd.Flags().GetBool("root")
	given := anyChanged(cmd, "parent-number", "parent-id", "parent-title")
	switch {
	case root && given:
		return nil, usageErr("--root and the --parent-number/--parent-id/--parent-title flags are mutually exclusive")
	case root:
		return nil, nil
	case !given:
		return nil, usageErr("specify the target with --parent-number, --parent-id, --parent-title or --root")
	}
	return resolveSelector(cmd, doc, "parent-")
}

// movePosition returns where under parent the move flags place it: after the
// sibling an after- selector names, first with --first, or neither (nil,
// false) for last. The after item must be a child of parent, other than it,
// and not a task when it is a category (subcategories precede tasks).
func movePosition(cmd *cobra.Command, doc *todo.Document, it, parent *todo.Item) (after *todo.Item, first bool, err error) {
	first, _ = cmd.Flags().GetBool("first")
	if !anyChanged(cmd, "after-number", "after-id", "after-title") {
		return nil, first, nil
	}
	if first {
		return nil, false, usageErr("--first and the --after-number/--after-id/--after-title flags are mutually exclusive")
	}
	if after, err = resolveSelector(cmd, doc, "after-"); err != nil {
		return nil, false, err
	}
	if after.Parent != parent {
		under := "the top level"
		if parent != nil {
			under = fmt.Sprintf("%q", placeName(parent))
		}
		return nil, false, usageErr(fmt.Sprintf("%q is not directly under %s", todo.Printable(after.Title), under))
	}
	switch {
	case after == it:
		return nil, false, usageErr("an item cannot be placed after itself")
	case it.Kind == todo.Category && after.Kind == todo.Task:
		return nil, false, usageErr(fmt.Sprintf("a category cannot be placed after the task %q: subcategories come before tasks", todo.Printable(after.Title)))
	}
	return after, false, nil
}

// anyChanged reports whether any of the named flags was given.
func anyChanged(cmd *cobra.Command, names ...string) bool {
	for _, n := range names {
		if cmd.Flags().Changed(n) {
			return true
		}
	}
	return false
}

// placeName names a parent for a message: its path of titles joined by " / ",
// or "top level" for the root (nil).
func placeName(parent *todo.Item) string {
	if parent == nil {
		return "top level"
	}
	return todo.Printable(strings.Join(parent.Path(), " / "))
}
