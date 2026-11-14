package todo

import (
	"context"
	"errors"
	"fmt"
	"os"
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
// items are deleted first.
func (s *Store) Import(ctx context.Context, doc *Document, replace bool) error {
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
		row := &dbItem{Kind: int(it.Kind), Title: it.Title, Status: int(it.Status), Description: it.Description}
		if it.Kind == Category {
			row.Status, row.Description = 0, ""
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
// creating the store when needed. It never writes mdPath. On failure it leaves
// no trace: a store file this call created is closed and removed with its WAL
// side files. On success the open store is returned.
func ImportFile(dbPath, mdPath string, replace bool) (*Store, ImportReport, error) {
	return importFile(context.Background(), dbPath, mdPath, replace)
}

// importFile is ImportFile with the context of the store write, so a test can
// make the import fail after the store file was created.
func importFile(ctx context.Context, dbPath, mdPath string, replace bool) (*Store, ImportReport, error) {
	existed, err := StoreExists(dbPath)
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
	s, err := OpenStore(dbPath)
	if err != nil {
		removeNewStore(dbPath, existed)
		return nil, ImportReport{}, err
	}
	if err := s.Import(ctx, doc, replace); err != nil {
		_ = s.Close()
		removeNewStore(dbPath, existed)
		return nil, ImportReport{}, err
	}
	return s, rep, nil
}

// removeNewStore deletes the store file and its WAL side files, unless the
// store existed before the import began.
func removeNewStore(dbPath string, existed bool) {
	if existed {
		return
	}
	for _, p := range []string{dbPath, dbPath + "-wal", dbPath + "-shm"} {
		_ = os.Remove(p)
	}
}
