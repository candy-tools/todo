package todo

// ParseMarkdown exposes parseMarkdown to the external test package, so parser
// tests can inspect the raw tree (tasks outside any heading stay at the root,
// unlike ImportMarkdown, which gathers them under Uncategorized).
var ParseMarkdown = parseMarkdown

// The JSON list format, exposed to the external test package.
type (
	DiskList     = diskList
	DiskCategory = diskCategory
	DiskView     = diskView
	DiskSprint   = diskSprint
	DiskTask     = diskTask
	DiskSubtask  = diskSubtask
)

// ValidateDisk runs the validation of l.
func ValidateDisk(l *DiskList) error { return l.validate() }
