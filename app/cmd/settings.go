package cmd

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/candy-tools/todo/internal/todo"
	"github.com/spf13/cobra"
)

// jsonSettings is the shape `settings --json` emits.
type jsonSettings struct {
	RefPrefix string `json:"refPrefix"`
}

func settingsCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "settings [--ref-prefix <prefix>]",
		Short: "show the list's settings, or set its reference prefix",
		Long: "settings prints the list's settings; today that is its reference prefix.\n\n" +
			"A task created without a reference gets one, <PREFIX>-<n> (WORK-001, WORK-002, ...),\n" +
			"from the prefix of its sprint, else of its nearest category that has one (set with\n" +
			"`todo add-category --prefix` or `todo edit --set-prefix`), else this list prefix. A\n" +
			"prefix is a letter and up to 9 letters or digits, stored in upper case. With\n" +
			"--ref-prefix the prefix is set, and an empty value clears it. Existing references\n" +
			"are never changed.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return withStore(cmd, func(ctx context.Context, s *todo.Store, doc *todo.Document) error {
				if cmd.Flags().Changed("ref-prefix") {
					p, _ := cmd.Flags().GetString("ref-prefix")
					res, err := s.SetRefPrefix(ctx, p)
					if err != nil {
						return err
					}
					doc = res.Doc
				}
				if asJSON, _ := cmd.Flags().GetBool("json"); asJSON {
					return json.NewEncoder(cmd.OutOrStdout()).Encode(jsonSettings{doc.RefPrefix})
				}
				shown := doc.RefPrefix
				if shown == "" {
					shown = "(none)"
				}
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "ref-prefix: %s\n", shown)
				return nil
			})
		},
	}
	c.Flags().String("ref-prefix", "", "set the list's reference prefix; empty clears it")
	c.Flags().Bool("json", false, "output the settings as JSON")
	return c
}
