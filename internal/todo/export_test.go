package todo

// ParseMarkdown exposes parseMarkdown to the external test package, so parser
// tests can inspect the raw tree (tasks outside any heading stay at the root,
// unlike ImportMarkdown, which gathers them under Uncategorized).
var ParseMarkdown = parseMarkdown
