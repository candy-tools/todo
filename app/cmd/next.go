package cmd

import (
	"fmt"
	"io"

	"github.com/candy-tools/todo/internal/todo"
	"github.com/spf13/cobra"
)

// nextSprint is one sprint's work queue: the sprint plus what is in progress
// and what is next.
type nextSprint struct {
	jsonSprint
	InProgress []record `json:"inProgress"`
	Next       []record `json:"next"`
}

// nextReport is the output of `todo next`.
type nextReport struct {
	Sprints []nextSprint `json:"sprints"`
}

// buildNext maps each sprint's work queue (Document.WorkQueue) to records.
func buildNext(doc *todo.Document, sprints []*todo.Sprint) nextReport {
	nums := numberMap(doc)
	toRecords := func(items []*todo.Item) []record {
		rs := []record{}
		for _, it := range items {
			rs = append(rs, toRecord(doc, it, nums))
		}
		return rs
	}
	rep := nextReport{Sprints: []nextSprint{}}
	for _, sp := range sprints {
		inProgress, next := doc.WorkQueue(sp.ID)
		rep.Sprints = append(rep.Sprints, nextSprint{
			jsonSprint: sprintOf(doc, sp),
			InProgress: toRecords(inProgress),
			Next:       toRecords(next),
		})
	}
	return rep
}

// nextCmd is `todo next`: what to work on, per open sprint (or one chosen
// sprint), as text or JSON.
func nextCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "next",
		Short: "what to work on: each open sprint's in-progress and next tasks",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			path, isDefault, err := resolvePath(cmd, nil)
			if err != nil {
				return err
			}
			_, doc, err := loadList(cmd, path, isDefault)
			if err != nil {
				return err
			}
			sprints := doc.OpenSprints()
			if key, _ := cmd.Flags().GetString("sprint"); key != "" {
				sp, err := resolveSprint(cmd, doc)
				if err != nil {
					return err
				}
				sprints = []*todo.Sprint{sp}
			}
			rep := buildNext(doc, sprints)
			if asJSON, _ := cmd.Flags().GetBool("json"); asJSON {
				return writeJSON(cmd.OutOrStdout(), rep)
			}
			writeNextText(cmd.OutOrStdout(), doc, rep)
			return nil
		},
	}
	c.Flags().String("sprint", "", "show only this sprint (reference, id or name), done or not")
	c.Flags().Bool("json", false, "output as JSON")
	return c
}

// writeNextText prints the report as text, one block per sprint separated by a
// blank line, and an empty section as "(none)"; with no sprint it prints "no
// open sprint".
func writeNextText(w io.Writer, doc *todo.Document, rep nextReport) {
	if len(rep.Sprints) == 0 {
		_, _ = fmt.Fprintln(w, "no open sprint")
		return
	}
	p := todo.Printable
	for i, s := range rep.Sprints {
		if i > 0 {
			_, _ = fmt.Fprintln(w)
		}
		head := sprintLabel(&todo.Sprint{Ref: s.Ref, Name: s.Name})
		if s.Goal != "" {
			head += " — " + p(s.Goal)
		}
		if s.Status == "done" {
			head += " (done)"
		}
		_, _ = fmt.Fprintln(w, head)
		for _, sec := range []struct {
			label string
			rs    []record
		}{{"in progress:", s.InProgress}, {"next:", s.Next}} {
			_, _ = fmt.Fprintln(w, "  "+sec.label)
			if len(sec.rs) == 0 {
				_, _ = fmt.Fprintln(w, "    (none)")
			}
			for _, r := range sec.rs {
				it, _ := doc.ByID(r.ID)
				_, _ = fmt.Fprintf(w, "    #%d [%s] %s\n", r.Number, r.Status, p(it.DisplayTitle()))
			}
		}
	}
}
