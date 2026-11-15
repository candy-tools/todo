package todo

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"unicode/utf8"

	closuretree "github.com/go-bumbu/closure-tree"
	"gorm.io/gorm"
)

// uncategorizedTitle is the category that collects tasks found outside any
// heading, which the store (like the app) does not allow at the root.
const uncategorizedTitle = "Uncategorized"

// ErrNotEmpty is returned by Import when the store already holds items and the
// caller did not ask to replace them.
var ErrNotEmpty = errors.New("database is not empty")

// ImportReport summarises a markdown import: how many categories and tasks were
// read and what the parser left out. Each warning reads `line N: <reason>`.
type ImportReport struct {
	Categories, Tasks int
	Warnings          []string
	// Skipped is set by ImportFile when nothing was imported because another
	// process created the store first; the report is otherwise empty.
	Skipped bool
	// Backup is the path of the copy ImportFile took of a store that held
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

// Import writes doc into the store in one transaction. It returns ErrNotEmpty
// when the store holds items and replace is false; with replace, the existing
// items are deleted first. doc is flattened first, in place (see
// Document.FlattenSubtasks), since the store keeps subtasks one level deep. A
// title CleanTitle refuses fails the import with ErrInvalidTitle; ImportMarkdown
// never yields one.
func (s *Store) Import(ctx context.Context, doc *Document, replace bool) error {
	doc.FlattenSubtasks()
	return s.write(ctx, func(tx *gorm.DB, tree *closuretree.Tree) error {
		existing, err := loadDocument(ctx, tx)
		if err != nil {
			return err
		}
		if len(existing.Roots) > 0 {
			if !replace {
				return ErrNotEmpty
			}
			for _, r := range existing.Roots {
				if err := tree.DeleteRecurse(ctx, r.ID, storeScope); err != nil {
					return err
				}
			}
		}
		return insertItems(ctx, tree, doc.Roots, 0)
	})
}

// insertItems adds items, in order, under parentID, each after the previous
// sibling's new ID, then recurses into each item's children.
func insertItems(ctx context.Context, tree *closuretree.Tree, items []*Item, parentID uint) error {
	var afterID uint
	for _, it := range items {
		title, err := CleanTitle(it.Title)
		if err != nil {
			return err
		}
		row := &dbItem{Kind: int(it.Kind), Title: title, Status: int(it.Status), Description: it.Description, Plan: it.Plan}
		if it.Kind == Category {
			row.Status, row.Description, row.Plan = 0, "", ""
		}
		if err := addRow(ctx, tree, row, parentID, afterID); err != nil {
			return err
		}
		afterID = row.NodeId
		if err := insertItems(ctx, tree, it.Children, row.NodeId); err != nil {
			return err
		}
	}
	return nil
}

// ImportFile imports the markdown file mdPath into the store at dbPath,
// creating the store when needed. It never writes mdPath and never removes
// dbPath. A missing store is built in a temporary file next to dbPath and only
// then linked (or, without hard links, renamed) into place, so dbPath never holds a half-done import and a failed
// import leaves no trace. When another process creates dbPath first, that
// store is kept and opened instead, and the report comes back empty with
// Skipped set. Replacing a store that holds items first copies it to the next
// free BackupPath, named in the report's Backup. On success the open store is
// returned.
func ImportFile(dbPath, mdPath string, replace bool) (*Store, ImportReport, error) {
	return importFile(context.Background(), dbPath, mdPath, replace)
}

// storeExists is the existence check importFile runs on the target. It is a
// variable so a test can stand in for a second process that checked before the
// first one created the store.
var storeExists = StoreExists

// linkFile hard-links importNew's temporary store to the target. It is a
// variable so a test can stand in for a filesystem without hard links.
var linkFile = os.Link

// importFile is ImportFile with the context of the store write, so a test can
// make the import fail after a store file was created.
func importFile(ctx context.Context, dbPath, mdPath string, replace bool) (*Store, ImportReport, error) {
	existed, err := storeExists(dbPath)
	if err != nil {
		return nil, ImportReport{}, err
	}
	src, err := os.ReadFile(mdPath) //nolint:gosec // mdPath is a user-provided CLI argument; reading it is the point
	if err != nil {
		return nil, ImportReport{}, err
	}
	doc, rep, err := ImportMarkdown(src)
	if err != nil {
		return nil, ImportReport{}, fmt.Errorf("%s: %w", mdPath, err)
	}
	if !existed {
		return importNew(ctx, dbPath, doc, rep)
	}
	s, err := OpenStore(dbPath)
	if err != nil {
		return nil, ImportReport{}, err
	}
	if replace {
		if rep.Backup, err = s.backupIfNotEmpty(ctx); err != nil {
			_ = s.Close()
			return nil, ImportReport{}, err
		}
	}
	if err := s.Import(ctx, doc, replace); err != nil {
		_ = s.Close()
		return nil, ImportReport{}, err
	}
	return s, rep, nil
}

// backupIfNotEmpty copies a store that holds items to the next free
// BackupPath (see backup) and returns that path, or "" for an empty store. It
// runs outside any transaction, since VACUUM INTO cannot run inside one, and
// only reads the live file, which stays the one the store opened.
func (s *Store) backupIfNotEmpty(ctx context.Context) (string, error) {
	doc, _, err := s.Load(ctx)
	if err != nil {
		return "", err
	}
	if len(doc.Roots) == 0 {
		return "", nil
	}
	path, err := s.backup()
	if err != nil {
		return "", fmt.Errorf("back up %s: %w", s.path, err)
	}
	return path, nil
}

// importNew imports doc into a new store at dbPath. It fills a temporary store
// in the same directory, closes it (which checkpoints its WAL into the file),
// gives it the 0644 mode SQLite gives a store it creates, and hard-links it to
// dbPath, which fails when dbPath exists: then another process imported first,
// and its store is opened instead with an empty, Skipped report. Where the link
// fails for another reason (a filesystem without hard links), the temporary
// store is renamed into place instead (see placeStore). The temporary files are
// removed either way.
func importNew(ctx context.Context, dbPath string, doc *Document, rep ImportReport) (*Store, ImportReport, error) {
	f, err := os.CreateTemp(filepath.Dir(dbPath), ".todo-import-*.db")
	if err != nil {
		return nil, ImportReport{}, err
	}
	tmp := f.Name()
	defer removeStoreFiles(tmp)
	if err := f.Close(); err != nil {
		return nil, ImportReport{}, err
	}
	s, err := OpenStore(tmp)
	if err != nil {
		return nil, ImportReport{}, err
	}
	if err := s.Import(ctx, doc, false); err != nil {
		_ = s.Close()
		return nil, ImportReport{}, err
	}
	if err := s.Close(); err != nil {
		return nil, ImportReport{}, err
	}
	if fi, err := os.Stat(tmp + "-wal"); err == nil && fi.Size() > 0 {
		return nil, ImportReport{}, fmt.Errorf("import into %s: the write-ahead log was not checkpointed", tmp)
	}
	if err := os.Chmod(tmp, 0o644); err != nil { //nolint:gosec // same mode SQLite creates a store with; the list is not secret
		return nil, ImportReport{}, err
	}
	placed, err := placeStore(tmp, dbPath)
	if err != nil {
		return nil, ImportReport{}, err
	}
	if !placed {
		rep = ImportReport{Skipped: true}
	}
	s, err = OpenStore(dbPath)
	if err != nil {
		return nil, ImportReport{}, err
	}
	return s, rep, nil
}

// placeStore puts the finished temporary store tmp at dbPath without ever
// replacing an existing dbPath. placed is false when dbPath already exists
// (another process imported first). It hard-links tmp to dbPath, which fails
// atomically on an existing target; when the link fails for another reason, it
// re-checks that dbPath is still missing and renames tmp onto it. That fallback
// leaves a tiny race: a store created between the check and the rename would
// be replaced, which a filesystem without hard links offers no way to avoid.
func placeStore(tmp, dbPath string) (placed bool, err error) {
	linkErr := linkFile(tmp, dbPath)
	if linkErr == nil {
		return true, nil
	}
	if errors.Is(linkErr, fs.ErrExist) {
		return false, nil
	}
	exists, err := StoreExists(dbPath)
	if err != nil {
		return false, err
	}
	if exists {
		return false, nil
	}
	if err := os.Rename(tmp, dbPath); err != nil {
		return false, fmt.Errorf("place %s (link failed: %v): %w", dbPath, linkErr, err)
	}
	return true, nil
}

// removeStoreFiles deletes the store file at path with its WAL side files. It
// is only ever given a temporary store (importNew's, or the migration's backup
// copy), never an import target or a backup in place.
func removeStoreFiles(path string) {
	for _, p := range []string{path, path + "-wal", path + "-shm"} {
		_ = os.Remove(p)
	}
}
