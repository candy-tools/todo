package cmd

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/candy-tools/todo/internal/todo"
	"github.com/spf13/cobra"
)

// jsonSettings is the shape `settings --json` emits: the list's task
// reference prefix and the sprint reference prefix set ("" for none), and the
// sprint reference prefix in effect (the one set, else the folder name's).
type jsonSettings struct {
	RefPrefix       string `json:"refPrefix"`
	SprintPrefix    string `json:"sprintPrefix"`
	SprintRefPrefix string `json:"sprintRefPrefix"`
}

func settingsCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "settings [--ref-prefix <prefix>] [--sprint-prefix <prefix>]",
		Short: "show the list's settings, or set its task and sprint reference prefixes",
		Long: "settings prints the list's settings: its task and sprint reference prefixes.\n\n" +
			"A task created without a reference gets one, <PREFIX>-<n> (WORK-001, WORK-002, ...),\n" +
			"from the prefix of its nearest category that has one (set with\n" +
			"`todo add-category --prefix` or `todo edit --set-prefix`), else the list's\n" +
			"--ref-prefix. Every sprint gets one too (TODO-001, TODO-002, ...), from\n" +
			"--sprint-prefix, else from the name of the folder that holds the list file (a\n" +
			"list in .../todo/ numbers its sprints TODO-001, ...); name a sprint by it with\n" +
			"--sprint. A prefix is a letter and up to 9 letters or digits, stored in upper\n" +
			"case; an empty value clears it. The sprint prefix may not be a task prefix (the\n" +
			"list's or a category's). Existing references are never changed.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return withStore(cmd, func(ctx context.Context, s *todo.Store, doc *todo.Document) error {
				var st todo.Settings
				if cmd.Flags().Changed("ref-prefix") {
					p, _ := cmd.Flags().GetString("ref-prefix")
					st.RefPrefix = &p
				}
				if cmd.Flags().Changed("sprint-prefix") {
					p, _ := cmd.Flags().GetString("sprint-prefix")
					st.SprintPrefix = &p
				}
				if st != (todo.Settings{}) {
					res, err := s.UpdateSettings(ctx, st)
					if err != nil {
						return err
					}
					doc = res.Doc
				}
				if asJSON, _ := cmd.Flags().GetBool("json"); asJSON {
					return json.NewEncoder(cmd.OutOrStdout()).Encode(jsonSettings{doc.RefPrefix, doc.SprintPrefix, doc.SprintRefPrefix()})
				}
				ref := doc.RefPrefix
				if ref == "" {
					ref = "(none)"
				}
				sprint := doc.SprintPrefix
				if sprint == "" {
					sprint = doc.SprintRefPrefix() + " (folder name)"
				}
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "ref-prefix: %s\nsprint-prefix: %s\n", ref, sprint)
				return nil
			})
		},
	}
	c.Flags().String("ref-prefix", "", "set the list's task reference prefix; empty clears it")
	c.Flags().String("sprint-prefix", "", "set the sprint reference prefix; empty goes back to the folder name")
	c.Flags().Bool("json", false, "output the settings as JSON")
	return c
}
