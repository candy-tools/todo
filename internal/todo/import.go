package todo

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"unicode/utf8"

	"github.com/candy-tools/todo/libs/jsonfile"
)

// uncategorizedTitle is the category that collects tasks found outside any
// heading, which the store (like the app) does not allow at the root.
const uncategorizedTitle = "Uncategorized"

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

// importDoc replaces the list with doc in one write. It returns ErrNotEmpty
// when the list holds items and replace is false, writing nothing. doc is
// prepared in place first: flattened (see Document.FlattenSubtasks), since the
// list keeps subtasks one level deep, its titles cleaned, and every item
// without an ID given one. A title CleanTitle refuses fails the import with
// ErrInvalidTitle; ImportMarkdown never yields one.
//
// When backUp is set and it replaces a list that holds items, importDoc first
// copies the file to the first free BackupPath beside it, inside the write and
// so under the lock, and returns that path. The copy is
// of the bytes the write read, from the file it read: for a symlinked list,
// the file the link points at, with the backup beside that file. When the
// write then fails, the list is unchanged and the backup is removed again.
func (s *Store) importDoc(ctx context.Context, doc *Document, replace, backUp bool) (WriteResult, string, error) {
	backup := ""
	res, err := s.update(ctx, func(d *Document, cur jsonfile.Snapshot) (bool, error) {
		holdsItems := len(d.Roots) > 0
		if holdsItems && !replace {
			return false, ErrNotEmpty
		}
		if err := prepareImport(doc); err != nil {
			return false, err
		}
		if holdsItems && backUp {
			p, err := backupFile(cur)
			if err != nil {
				return false, fmt.Errorf("back up %s: %w", cur.Path, err)
			}
			backup = p
		}
		d.Roots = doc.Roots
		return true, nil
	})
	if err == nil {
		return res, backup, nil
	}
	if backup != "" {
		if rmErr := os.Remove(backup); rmErr != nil && !errors.Is(rmErr, fs.ErrNotExist) {
			return WriteResult{}, "", fmt.Errorf("%w (backup %s left in place: %v)", err, backup, rmErr)
		}
	}
	return WriteResult{}, "", err
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
// the file when needed, in one write (see importDoc), and returns a store on
// path made with opts. It never writes mdPath. A list that holds items is
// ErrNotEmpty without replace, with nothing written. With replace, the list
// file is first copied to the first free BackupPath beside it (beside the file
// a symlinked path points at), named in the report's Backup; when the import
// then fails, that copy is removed again.
func ImportFile(path, mdPath string, replace bool, opts ...Option) (*Store, ImportReport, error) {
	src, err := os.ReadFile(mdPath) //nolint:gosec // mdPath is a user-provided CLI argument; reading it is the point
	if err != nil {
		return nil, ImportReport{}, err
	}
	doc, rep, err := ImportMarkdown(src)
	if err != nil {
		return nil, ImportReport{}, fmt.Errorf("%s: %w", mdPath, err)
	}
	s := NewStore(path, opts...)
	if _, rep.Backup, err = s.importDoc(context.Background(), doc, replace, true); err != nil {
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

// backupFile writes cur's bytes, the list file as the write read it, to the
// first free BackupPath beside cur.Path: .bak, or .bak.1, .bak.2, … when that
// is taken. An existing backup is never replaced: each candidate is created
// exclusively (O_EXCL), so no temporary file is needed. The backup gets the
// list file's mode. It returns the backup path it used.
func backupFile(cur jsonfile.Snapshot) (string, error) {
	info, err := os.Stat(cur.Path)
	if err != nil {
		return "", err
	}
	for n := 0; ; n++ {
		dst := BackupPath(cur.Path, n)
		f, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600) //nolint:gosec // dst is a backup path beside the list file
		if errors.Is(err, fs.ErrExist) {
			continue
		}
		if err != nil {
			return "", err
		}
		if err := writeBackup(f, cur.Bytes, info.Mode().Perm()); err != nil {
			_ = os.Remove(dst)
			return "", err
		}
		return dst, nil
	}
}

// writeBackup gives f, a new backup file, mode, then writes data to it,
// syncs and closes it.
func writeBackup(f *os.File, data []byte, mode fs.FileMode) error {
	err := f.Chmod(mode) // after creating, so the umask does not narrow it
	if err == nil {
		_, err = f.Write(data)
	}
	if err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	return err
}
