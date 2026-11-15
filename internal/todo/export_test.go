package todo

import "testing"

// ParseMarkdown exposes parseMarkdown to the external test package, so parser
// tests can inspect the raw tree (tasks outside any heading stay at the root,
// unlike ImportMarkdown, which gathers them under Uncategorized).
var ParseMarkdown = parseMarkdown

// SetImportAfterBackup sets importAfterBackup to f for the rest of the test t,
// so the external test package can act between ImportFile's open of an
// existing store (and its backup) and the import write.
func SetImportAfterBackup(t testing.TB, f func()) {
	importAfterBackup = f
	t.Cleanup(func() { importAfterBackup = nil })
}
