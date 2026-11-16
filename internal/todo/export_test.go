package todo

import (
	"testing"
	"time"
)

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

// SetOpenWait sets openWait to d for the rest of the test t, so a test can see
// OpenStore give up on another process's lock without waiting out the default.
func SetOpenWait(t testing.TB, d time.Duration) {
	openWait = d
	t.Cleanup(func() { openWait = DefaultBusyTimeout })
}

// SeedV1 exposes seedV1 to the external test package: a schema-1 store, which
// opening has to migrate.
var SeedV1 = seedV1

// The JSON list format, exposed to the external test package.
type (
	DiskList     = diskList
	DiskCategory = diskCategory
	DiskTask     = diskTask
	DiskSubtask  = diskSubtask
)

var (
	ToDisk   = toDisk
	FromDisk = fromDisk
)

// FileVersion is the on-disk format version.
const FileVersion = fileVersion

// ValidateDisk runs the validation of l.
func ValidateDisk(l *DiskList) error { return l.validate() }
