package todo

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"unicode/utf8"
)

// uncategorizedTitle is the category that collects tasks found outside any
// heading, which the store (like the app) does not allow at the root.
const uncategorizedTitle = "Uncategorized"

// ErrNotEmpty is returned by Import when the list already holds items and the
// caller did not ask to replace them.
var ErrNotEmpty = errors.New("list is not empty")

// ImportReport summarises a markdown import: how many categories and tasks were
// read and what the parser left out. Each warning reads `line N: <reason>`.
type ImportReport struct {
	Categories, Tasks int
	Warnings          []string
	// Backup is the path of the copy ImportFile took of a list that held
	// items before replacing them; it is empty when nothing was replaced.
	Backup string
}

// ImportMarkdown parses a TODO.md leniently for import. It fails only when src
// is not valid UTF-8; anything it cannot keep becomes a warning. Tasks found
// outside every category are moved into a new last root category named
// Uncategorized, since the store keeps tasks under categories only.
func ImportMarkdown(src []byte) (*Document, ImportReport, error) {
	if !utf8.Valid(src) {
		return nil, ImportReport{}, errors.New("markdown is not valid UTF-8")
	}
	doc, warnings := parseMarkdown(string(src))

	var loose, roots []*Item
	for _, it := range doc.Roots {
		if it.Kind == Task {
			loose = append(loose, it)
		} else {
			roots = append(roots, it)
		}
	}
	doc.Roots = roots
	if len(loose) > 0 {
		cat := &Item{Kind: Category, Level: 1, Title: uncategorizedTitle}
		doc.AppendRoot(cat)
		for _, t := range loose {
			cat.AppendChild(t)
		}
	}

	rep := ImportReport{Warnings: warnings}
	for _, r := range doc.Roots {
		count(r, &rep)
	}
	return doc, rep, nil
}

// count adds it and its subtree to the totals of rep.
func count(it *Item, rep *ImportReport) {
	if it.Kind == Category {
		rep.Categories++
	} else {
		rep.Tasks++
	}
	for _, c := range it.Children {
		count(c, rep)
	}
}

// Import replaces the list with doc in one write. It returns ErrNotEmpty when
// the list holds items and replace is false, writing nothing. doc is prepared
// in place first: flattened (see Document.FlattenSubtasks), since the list
// keeps subtasks one level deep, its titles cleaned, and every item without an
// ID given one. A title CleanTitle refuses fails the import with
// ErrInvalidTitle; ImportMarkdown never yields one.
func (s *Store) Import(ctx context.Context, doc *Document, replace bool) error {
	_, err := s.importDoc(ctx, doc, replace, false)
	return err
}

// importDoc is Import that, when backUp is set and it replaces a list that
// holds items, first copies the file to the first free BackupPath, inside the
// write and so under the lock, and returns that path. When the write then
// fails, the list is unchanged and the backup is removed again.
func (s *Store) importDoc(ctx context.Context, doc *Document, replace, backUp bool) (string, error) {
	backup := ""
	err := s.write(ctx, func(d *Document) (bool, error) {
		holdsItems := len(d.Roots) > 0
		if holdsItems && !replace {
			return false, ErrNotEmpty
		}
		if err := prepareImport(doc); err != nil {
			return false, err
		}
		if holdsItems && backUp {
			p, err := backupFile(s.Path())
			if err != nil {
				return false, fmt.Errorf("back up %s: %w", s.Path(), err)
			}
			backup = p
		}
		d.Roots = doc.Roots
		return true, nil
	})
	if err == nil {
		return backup, nil
	}
	if backup != "" {
		if rmErr := os.Remove(backup); rmErr != nil && !errors.Is(rmErr, fs.ErrNotExist) {
			return "", fmt.Errorf("%w (backup %s left in place: %v)", err, backup, rmErr)
		}
	}
	return "", err
}

// prepareImport readies doc to become the list: it flattens the subtasks,
// cleans every title (failing on one CleanTitle refuses) and gives every item
// without an ID a fresh one.
func prepareImport(doc *Document) error {
	doc.FlattenSubtasks()
	for _, it := range doc.Enumerate() {
		title, err := CleanTitle(it.Title)
		if err != nil {
			return err
		}
		it.Title = title
	}
	doc.AssignIDs()
	return nil
}

// ImportFile imports the markdown file mdPath into the list at path, creating
// the file when needed, in one write (see Import), and returns a store on path
// opened with opts. It never writes mdPath. A list that holds items is
// ErrNotEmpty without replace, with nothing written. With replace, the list
// file is first copied to the first free BackupPath, named in the report's
// Backup; when the import then fails, that copy is removed again.
func ImportFile(path, mdPath string, replace bool, opts ...Option) (*Store, ImportReport, error) {
	src, err := os.ReadFile(mdPath) //nolint:gosec // mdPath is a user-provided CLI argument; reading it is the point
	if err != nil {
		return nil, ImportReport{}, err
	}
	doc, rep, err := ImportMarkdown(src)
	if err != nil {
		return nil, ImportReport{}, fmt.Errorf("%s: %w", mdPath, err)
	}
	s, err := OpenStore(path, opts...)
	if err != nil {
		return nil, ImportReport{}, err
	}
	if rep.Backup, err = s.importDoc(context.Background(), doc, replace, true); err != nil {
		return nil, ImportReport{}, err
	}
	return s, rep, nil
}

// BackupPath returns where an import that replaces the list at path keeps a
// copy of the old file: <path>.bak for n = 0, <path>.bak.<n> otherwise.
func BackupPath(path string, n int) string {
	if n == 0 {
		return path + ".bak"
	}
	return fmt.Sprintf("%s.bak.%d", path, n)
}

// backupFile copies the file at path, byte for byte and with its mode, to the
// first free BackupPath: .bak, or .bak.1, .bak.2, … when that is taken. An
// existing backup is never replaced. The copy is written to a temporary file
// next to path and only the finished file is put in place (see placeBackup).
// It returns the backup path it used.
func backupFile(path string) (string, error) {
	data, err := os.ReadFile(path) //nolint:gosec // path is the list file the store was opened on
	if err != nil {
		return "", err
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	mode := info.Mode().Perm()
	f, err := os.CreateTemp(filepath.Dir(path), ".todo-backup-*")
	if err != nil {
		return "", err
	}
	tmp := f.Name()
	defer func() { _ = os.Remove(tmp) }()
	_, err = f.Write(data)
	if err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Chmod(tmp, mode)
	}
	if err != nil {
		return "", err
	}
	for n := 0; ; n++ {
		dst := BackupPath(path, n)
		placed, err := placeBackup(tmp, dst, data, mode)
		if err != nil {
			return "", err
		}
		if placed {
			return dst, nil
		}
	}
}

// placeBackup puts the finished copy tmp at dst unless dst exists, reporting
// placed = false then. It hard-links tmp to dst, which fails atomically on an
// existing target. Where the link fails for another reason (a filesystem
// without hard links), it creates dst exclusively (O_EXCL) and writes data
// into it, removing it again if that fails.
func placeBackup(tmp, dst string, data []byte, mode fs.FileMode) (placed bool, err error) {
	linkErr := os.Link(tmp, dst)
	if linkErr == nil {
		return true, nil
	}
	if errors.Is(linkErr, fs.ErrExist) {
		return false, nil
	}
	f, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode) //nolint:gosec // dst is a backup path next to the list file
	if errors.Is(err, fs.ErrExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("create %s (link failed: %v): %w", dst, linkErr, err)
	}
	_, err = f.Write(data)
	if err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		_ = os.Remove(dst)
		return false, err
	}
	return true, nil
}
