package cmd

import (
	"github.com/candy-tools/todo/internal/todo"
	"github.com/spf13/cobra"
)

// fileArg returns the file a subcommand operates on (the --file flag, default
// TODO.md).
func fileArg(cmd *cobra.Command) string {
	f, _ := cmd.Flags().GetString("file")
	return f
}

// numberMap maps each item to its 1-based list number (see Document.Enumerate).
func numberMap(d *todo.Document) map[*todo.Item]int {
	m := map[*todo.Item]int{}
	for i, it := range d.Enumerate() {
		m[it] = i + 1
	}
	return m
}
