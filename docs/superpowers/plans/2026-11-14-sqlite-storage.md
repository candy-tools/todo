# SQLite Storage Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace todo's markdown persistence with a per-directory SQLite database (`.todo.db`) built on go-bumbu `closure-tree` + `db/gormdb`. Keep every TUI and CLI behaviour, and add a one-time markdown import.

**Architecture:**
- The in-memory `todo.Document` tree stays the read model.
- A new `todo.Store` in `internal/todo` is the only code that touches the database. Each store write runs in a single transaction that uses a tree bound to that transaction (`closuretree.New(tx, dbItem{})`, the codex `treeIn` pattern), and bumps `meta.revision`.
- Callers (CLI, TUI) follow one flow: they mutate the in-memory `Document` with the existing domain functions to work out where an item goes, persist that placement or those fields through the store, then reload the `Document` from the store.
- Live reload polls the revision instead of the file.

**Tech Stack:** Go 1.26, `github.com/go-bumbu/closure-tree` v0.10.0, `github.com/go-bumbu/db` v0.1.0 (`gormdb` + `gormdb/sqlite`, pure-Go glebarez), GORM, bubbletea, cobra.

**Spec:** `docs/superpowers/specs/2026-11-14-sqlite-storage-design.md`

## Global Constraints

- `CGO_ENABLED=0` builds must keep working. Use only `gormdb/sqlite` (glebarez), never `gormdb/sqlitecgo` or `gorm.io/driver/sqlite`.
- Open the database with a GORM logger of `logger.Discard`, so GORM never prints into the TUI or CLI output.
- The default database path is `.todo.db`. The legacy import source is `TODO.md`.
- The task node table is named `items` (`TableName()`). The revision table is `meta`.
- There is one `closure-tree` scope (tenant): the constant `storeScope = "todo"`, defined once in `internal/todo/store.go`.
- Inside a store transaction, use only `tx` and the tree bound to `tx`. Never use the root `*gorm.DB` there.
- Field updates go through explicit column maps (`Updates(map[string]any{…})`). `Tree.Update` is used only for moves, with a nil item, because it overwrites every payload column.
- Dependency direction: `app/` → `internal/todo` → libs. `internal/todo` never imports `app/`.
- The CLI exit codes `0/1/2/3/4/5` and their meanings stay unchanged.
- Commits: one-line Conventional Commit messages, no `Co-Authored-By`, `git add` as its own step. The branch is `feat/sqlite-storage`.
- `make verify` must pass at the end: tests, vet, golangci-lint, license-check, and coverage ≥ 70% per package.

## Review Focus

1. **A TUI and a CLI process writing at the same time.** The second writer waits on the busy timeout instead of failing, and the TUI reloads and keeps the selection. This is covered in Task 6: `TestExternalWriteReloadsKeepingSelection`.
2. **Prune while an agent adds a subtask under a done task.** The new open subtask and its done parent survive. Covered in Task 3: `TestPruneUsesFreshData`.
3. **Setting a status back to Open (0), or editing a category.** Other columns are never wiped, and `Kind` stays category. Covered in Task 3: `TestSetStatusesBackToOpen` and `TestEditKeepsKind`.
4. **Running `todo list` in a fresh directory with neither file.** It prints the header and creates no `.todo.db`. Covered in Task 5: `TestListMissingDBCreatesNothing`.
5. **A failed auto-import (invalid UTF-8 in `TODO.md`).** The command fails with a clear error, leaves no `.todo.db`/`-wal`/`-shm` behind, and leaves `TODO.md` untouched. Covered in Task 4: `TestImportFileFailureLeavesNoDB`.

---

## File map

| File | Responsibility |
|---|---|
| `internal/todo/model.go` | Domain tree: add `Item.ID`, `ByID`, `Placement`, `DoneSubtrees`; later, ID-keyed `DoneStates` |
| `internal/todo/store.go` | `Store`, open/close/exists, `Load`, `Revision`, schema structs |
| `internal/todo/store_write.go` | All store writes plus the `write` transaction helper |
| `internal/todo/markdown.go` | The renamed `parse.go`: the lenient markdown parser with warnings, plus `ImportMarkdown` |
| `internal/todo/import.go` | `Store.Import`, `ImportFile` (with cleanup on failure) |
| `internal/todo/guide.go` | Reduced to `guideRange` / `guideStart` detection only (Task 7) |
| `internal/todo/todotest/todotest.go` | Test support: `Seed`, `Dump`, `Render`, used by the cmd and tui tests |
| `app/cmd/store.go` | `openStore` / `openForRead`: path resolution, the `.md` guard, auto-import |
| `app/cmd/import.go` | `todo import <file.md> [--force]` |
| `app/cmd/*.go` | Commands switched to the store, with `--id` replacing `--line` |
| `app/tui/tui.go`, `reload.go`, `tree.go` | Model holds a `*todo.Store`. Write flow, revision polling, ID-keyed folds, selection and snapshots |
| `README.md`, `DOCUMENTATION.md` | Docs |

---

### Task 1: Domain additions (IDs, placement, done-subtrees)

These changes are additive only, so markdown mode keeps working unchanged.

**Files:**
- Modify: `internal/todo/model.go`, `internal/todo/lookup.go`
- Test: `internal/todo/model_test.go`, `internal/todo/lookup_test.go`

**Interfaces:**
- Produces:
  - `Item.ID uint`: the database node ID, 0 until persisted.
  - `func (d *Document) ByID(id uint) (*Item, bool)`: false for 0 or unknown.
  - `func (d *Document) Placement(it *Item) (parentID, afterID uint)`: `parentID` is `it.Parent.ID`, or 0 for a root. `afterID` is the ID of the sibling immediately before `it` among its peers, or 0 when `it` is first.
  - `func (d *Document) DoneSubtrees() []*Item`: the outermost fully-done tasks (`fullyDone`), in pre-order. It doesn't descend into a returned item, but does descend into everything else.

- [ ] **Step 1: Write the failing tests**

```go
func TestPlacement(t *testing.T) {
	doc := Parse("# A\n\n- [ ] t1\n- [ ] t2\n\n## B\n")
	a := doc.Roots[0]; t1, t2, b := a.Children[0], a.Children[1], a.Children[2]
	a.ID, t1.ID, t2.ID, b.ID = 1, 2, 3, 4
	if p, af := doc.Placement(t1); p != 1 || af != 0 { t.Errorf("t1: %d,%d", p, af) }
	if p, af := doc.Placement(t2); p != 1 || af != 2 { t.Errorf("t2: %d,%d", p, af) }
	if p, af := doc.Placement(b); p != 1 || af != 3 { t.Errorf("b: %d,%d", p, af) }
	if p, af := doc.Placement(a); p != 0 || af != 0 { t.Errorf("a: %d,%d", p, af) }
}

func TestDoneSubtrees(t *testing.T) {
	doc := Parse("# A\n\n- [x] done\n  - [x] d1\n- [x] mixed\n  - [ ] open\n  - [x] inner\n")
	got := doc.DoneSubtrees()
	// "done" (whole subtree) and "inner" (inside the kept "mixed"); never "d1" separately.
	if len(got) != 2 || got[0].Title != "done" || got[1].Title != "inner" { t.Fatalf("got %v", titles(got)) }
}

func TestByID(t *testing.T) {
	doc := Parse("# A\n\n- [ ] t\n")
	doc.Roots[0].Children[0].ID = 7
	if it, ok := doc.ByID(7); !ok || it.Title != "t" { t.Fatal("ByID(7)") }
	if _, ok := doc.ByID(0); ok { t.Fatal("ByID(0) must be false") }
}
```
(Add a small `titles([]*Item) []string` test helper if one doesn't exist.)

- [ ] **Step 2: Run** `go test ./internal/todo/ -run 'TestPlacement|TestDoneSubtrees|TestByID'`. Expected: FAIL (undefined: `Placement` / `DoneSubtrees` / `ByID` / field `ID`).

- [ ] **Step 3: Implement.** Add `ID uint` to `Item` with a doc comment. Add `ByID` in `lookup.go` (walk `Enumerate`). Add `Placement` (use `siblings`) and `DoneSubtrees` in `model.go`. Re-implement `RemovableDone` and `RemoveDone` on top of `DoneSubtrees`, keeping their exact results: `RemovableDone` = Σ(1+total) over `DoneSubtrees()`, and `RemoveDone` removes each with `d.Remove`.

- [ ] **Step 4: Run** `go test ./...`. Expected: PASS. The existing `RemoveDone` and `RemovableDone` tests are unchanged.

- [ ] **Step 5: Commit**
```bash
git add internal/todo
git commit -m "feat(todo): add item IDs, placement and done-subtree helpers"
```

---

### Task 2: Store skeleton (open, schema, load, revision)

**Files:**
- Modify: `go.mod`, `go.sum`, plus `overrideLicenses.json` only if Step 6 requires it
- Create: `internal/todo/store.go`
- Test: `internal/todo/store_test.go`

**Interfaces:**
- Consumes: `Item.ID` (Task 1).
- Produces:
  - `func OpenStore(path string) (*Store, error)`: opens or creates the SQLite file through `gormdb.Open(gormdb.Cfg{Driver: gormdb.DriverSqlite, Sqlite: gormdb.SqliteCfg{Path: path}, Logger: logger.Discard}, gormdb.Drivers{Sqlite: sqlite.Driver{}})`. It runs `Tree.Migrate()` and AutoMigrates `dbMeta`, then ensures the meta row exists (`id=1, revision=0, schema_version=1`).
  - `func StoreExists(path string) (bool, error)`: `os.Stat`. A not-exist error returns `false, nil`.
  - `func (s *Store) Close() error`: `gormdb.Close`.
  - `func (s *Store) Path() string`.
  - `func (s *Store) Load(ctx context.Context) (*Document, int64, error)`: reads the whole tree (`TreeDescendants(ctx, 0, 0, storeScope, &[]*dbItem)`) and the revision in one read transaction. It maps rows to `Item`s with `ID`, `Parent`, `Children` and `Kind`/`Title`/`Status`/`Description`. A category's `Level` = 1 + the number of category ancestors.
  - `func (s *Store) Revision(ctx context.Context) (int64, error)`.
  - Unexported: `dbItem{closuretree.Node; Kind int; Title string; Status int; Description string; Children []*dbItem `gorm:"-"`}` with `TableName() "items"`; `dbMeta{ID uint `gorm:"primaryKey"`; Revision int64; SchemaVersion int}` with `TableName() "meta"`; `const storeScope = "todo"`.

- [ ] **Step 1: Add dependencies**
```bash
go get github.com/go-bumbu/closure-tree@v0.10.0 github.com/go-bumbu/db@v0.1.0 gorm.io/gorm
```

- [ ] **Step 2: Write the failing tests** (use the helper `newStore(t) *Store`, which opens `filepath.Join(t.TempDir(), "t.db")` and registers `t.Cleanup(Close)`):
  - `TestOpenStoreCreatesEmpty`: `Load` returns 0 roots and revision 0. `StoreExists` on that path is true. `StoreExists` on a missing path is `false, nil`.
  - `TestOpenStoreRejectsDirectoryPath`: `OpenStore(t.TempDir())` returns an error.

- [ ] **Step 3: Run** `go test ./internal/todo/ -run TestOpenStore`. Expected: FAIL (undefined: `OpenStore`).

- [ ] **Step 4: Implement `store.go`** as specified in the Interfaces block.

- [ ] **Step 5: Run** `go test ./...`. Expected: PASS.

- [ ] **Step 6: Run** `make license-check`. Expected: no violations. If a new module's license is reported as undetected, add an `overrideLicenses.json` line *only* when its real license is on the allowlist (check the module's LICENSE file). If any module is under a license that isn't allowed, stop and report it instead of overriding.

- [ ] **Step 7: Commit**
```bash
git add go.mod go.sum internal/todo/store.go internal/todo/store_test.go overrideLicenses.json
git commit -m "feat(todo): add the SQLite store with schema, load and revision"
```

---

### Task 3: Store writes

**Files:**
- Create: `internal/todo/store_write.go`
- Test: `internal/todo/store_write_test.go`

**Interfaces:**
- Consumes: `Store`, `dbItem`, `storeScope` (Task 2); `DoneSubtrees` (Task 1).
- Produces (each write runs in one `db.Transaction` via `s.write`, bumps `meta.revision` by exactly 1, and renormalizes if `NeedsRenormalizeAny(…, closuretree.DefaultHalvingsBuffer)`):
  - `var ErrNotFound = errors.New("item not found")`, mapped from `closuretree.ErrNodeNotFound` and `ErrParentNotFound`.
  - `func (s *Store) AddCategory(ctx context.Context, title string, parentID, afterID uint) (uint, error)`
  - `func (s *Store) AddTask(ctx context.Context, title, desc string, st Status, parentID, afterID uint) (uint, error)`
  - `func (s *Store) Edit(ctx context.Context, id uint, title, desc string) error`: updates `title` and `description` only. It writes `desc` for tasks only.
  - `func (s *Store) SetStatuses(ctx context.Context, states map[uint]Status) error`: unknown IDs are skipped silently, which the undo of a since-deleted task relies on.
  - `func (s *Store) CascadeStatus(ctx context.Context, id uint, st Status) error`: sets `id` and every descendant that exists *at write time* (`DescendantIds` on the tx tree).
  - `func (s *Store) Move(ctx context.Context, id, parentID, afterID uint) error`: `Tree.Update(ctx, id, nil, &parentID, &afterID, storeScope)`.
  - `func (s *Store) Delete(ctx context.Context, id uint) error`: `DeleteRecurse`.
  - `func (s *Store) Prune(ctx context.Context) (int, error)`: inside the tx, it loads the tree from `tx`, takes `DoneSubtrees()`, `DeleteRecurse`s each one, and returns Σ(1+total).
  - Unexported: `func (s *Store) write(ctx context.Context, fn func(tx *gorm.DB, tree *closuretree.Tree) error) error`.

- [ ] **Step 1: Write the failing tests:**
  - `TestAddKeepsOrder`: add category A, tasks t1 (after 0) and t2 (after t1), and category B under A (after t2). `Load` returns A's children `[t1 t2 B]`, B with `Level` 2, and statuses as passed.
  - `TestEditKeepsKind`: add category C, `Edit(C, "C2", "x")`. After `Load`, C is still a `Category` titled "C2" with no description. A task's `Edit` sets its description and keeps its `Status`.
  - `TestSetStatusesBackToOpen`: a Done task with a description, then `SetStatuses({id: Open})`. After `Load`: `Open`, with the description and title unchanged. An unknown ID in the map gives no error.
  - `TestCascadeStatus`: a parent with 2 subtasks, then `CascadeStatus(parent, Done)`. All 3 are Done.
  - `TestMove`: move t2 to first under A (`Move(t2, A, 0)`), giving order `[t2 t1 B]`. Move t1 under B, giving B's children `[t1]`.
  - `TestDeleteSubtree`: deleting a parent removes its subtasks. `Delete(999)` returns `ErrNotFound`.
  - `TestPruneUsesFreshData`:
    1. Store `s1` holds `A > done[x] > d1[x]`.
    2. A second `OpenStore` on the same path adds an open subtask under `done`.
    3. `s1.Prune` must see that: `done` is no longer fully done, so only `d1` goes.
    4. Assert: n == 1, `done` present, the new subtask present, `d1` gone.
  - `TestOpenStoreReopenKeepsData`: `AddCategory`, `Close`, reopen. `Load` has that one category.
  - `TestEveryWriteBumpsRevision`: each of the 8 write methods raises `Revision()` by exactly 1.
  - `TestManyInsertsBetweenSiblingsStayOrdered`: insert 60 tasks, each after the first task (which forces bisection), then check `Load` order is the expected sequence. This exercises renormalize.

- [ ] **Step 2: Run** `go test ./internal/todo/ -run 'TestAdd|TestEdit|TestSetStatuses|TestCascade|TestMove|TestDelete|TestPrune|TestEvery|TestMany'`. Expected: FAIL (undefined methods).

- [ ] **Step 3: Implement `store_write.go`.** In `write`, call `s.db.Transaction(func(tx) { tree, err := closuretree.New(tx, dbItem{}); …; fn(tx, tree); bump; renormalize })`. Status updates use `tx.Model(&dbItem{}).Where("node_id IN ? AND tenant = ?", ids, storeScope).Update("status", int(st))`, grouped by status.

- [ ] **Step 4: Run** `go test ./internal/todo/`. Expected: PASS.

- [ ] **Step 5: Commit**
```bash
git add internal/todo/store_write.go internal/todo/store_write_test.go
git commit -m "feat(todo): add transactional store writes with revision bumps"
```

---

### Task 4: Markdown import and test support

**Files:**
- Rename: `internal/todo/parse.go` → `internal/todo/markdown.go` (`git mv`)
- Create: `internal/todo/import.go`, `internal/todo/todotest/todotest.go`
- Test: `internal/todo/import_test.go`, `internal/todo/todotest/todotest_test.go`

**Interfaces:**
- Consumes: the Store writes (Task 3).
- Produces:
  - `func ImportMarkdown(src []byte) (*Document, ImportReport, error)`:
    - Returns an error when `!utf8.Valid(src)`.
    - Otherwise parses leniently and collects warnings.
    - Moves root-level tasks into a new last root category titled `Uncategorized`.
    - Counts categories and tasks.
  - `type ImportReport struct { Categories, Tasks int; Warnings []string }`. Each warning reads `line N: <reason>`, with these reasons:
    - `text before the first heading is not imported`, once, for non-guide preamble;
    - `unrecognised line dropped: "<trimmed text>"`, for prose the parser drops, including unknown checkbox markers and indented text under a category.
  - `var ErrNotEmpty = errors.New("database is not empty")`.
  - `func (s *Store) Import(ctx context.Context, doc *Document, replace bool) error`:
    - Returns `ErrNotEmpty` when the store has items and `!replace`.
    - With `replace`, deletes all roots first.
    - Inserts every item in document order inside **one** `s.write` (pre-order, each `afterID` = the previous sibling's new ID).
  - `func ImportFile(dbPath, mdPath string, replace bool) (*Store, ImportReport, error)`:
    - Reads `mdPath` and creates or opens `dbPath`.
    - On any failure, if `dbPath` didn't exist before the call, it closes the store and removes `dbPath`, `dbPath+"-wal"` and `dbPath+"-shm"`.
    - Never writes `mdPath`.
  - Internal: the parser returns warnings, through `parseMarkdown(src string) (*Document, []string)`. `Parse(src) *Document` stays as a thin wrapper until Task 7.
  - Package `todotest`:
    - `func Seed(t testing.TB, md string) string`: imports `md` into `<t.TempDir()>/.todo.db` and returns that path. It fails the test on error or warnings.
    - `func Dump(t testing.TB, dbPath string) string`: opens, `Load`s and returns `Render(doc)`.
    - `func Render(doc *todo.Document) string`: canonical markdown without the guide block. It wraps `doc.Render()` for now and absorbs the renderer in Task 7.

- [ ] **Step 1: Write the failing tests:**
  - `TestImportExampleRoundTrips`: `Render(Load(Seed(example.md)))` == `Render(Parse(example.md))`. This compares structure, order, statuses and descriptions.
  - `TestImportSkipsGuide`: importing a file whose only content is the guide block plus `# Main` gives exactly 1 category, 0 tasks and 0 warnings.
  - `TestImportWarnings`: given `"intro\n# A\n\n- [?] odd\n  stray under cat\n"`, the report has 3 warnings: preamble, the `- [?]` line and the stray line. The category still imports.
  - `TestImportRootTasksGoToUncategorized`: `"- [ ] loose\n# A\n"` gives roots `[A, Uncategorized]`, with `loose` under `Uncategorized`.
  - `TestImportEmptyFile`: empty input gives 0/0 and no error.
  - `TestImportRefusesNonEmpty` / `TestImportReplace`: `ErrNotEmpty` without `replace`. With `replace`, only the new contents remain.
  - `TestImportFileFailureLeavesNoDB`: `mdPath` holds `[]byte{0xff, 0xfe}`. `ImportFile` errors, none of the three db files exist, and the md file's bytes are unchanged.

- [ ] **Step 2: Run** `go test ./internal/todo/...`. Expected: FAIL (undefined: `ImportMarkdown` / `ImportFile` / `todotest`).

- [ ] **Step 3: Implement** `markdown.go` warnings, `import.go` and `todotest`.

- [ ] **Step 4: Run** `go test ./...`. Expected: PASS.

- [ ] **Step 5: Commit**
```bash
git add internal/todo
git commit -m "feat(todo): import markdown into the store, with warnings and cleanup on failure"
```

---

### Task 5: CLI on the store

**Files:**
- Create: `app/cmd/store.go`, `app/cmd/import.go`, `app/cmd/import_test.go`, `app/cmd/store_test.go`
- Modify: `app/cmd/root.go`, `cli.go`, `list.go`, `add.go`, `status.go`, `modify.go`, `helpers_test.go`, and every `app/cmd/*_test.go`

**Interfaces:**
- Consumes: `OpenStore`, `StoreExists`, `ImportFile`, the Store writes, `Load`, `Placement`, `ByID`, and `todotest.Seed`/`Dump`.
- Produces:
  - `const defaultFile = ".todo.db"`, `const legacyFile = "TODO.md"`.
  - `func resolvePath(cmd *cobra.Command, args []string) (path string, isDefault bool, err error)`:
    - The positional arg wins, then `-f`, then `defaultFile`.
    - A path ending in `.md` returns `usageErr(fmt.Sprintf("%s is a markdown file; import it once with: todo import %s", p, p))`.
  - `func openStore(cmd *cobra.Command, path string, isDefault bool) (*todo.Store, string, error)`:
    - Opens or creates the store.
    - When `isDefault` and the db doesn't exist and `TODO.md` exists, it runs `ImportFile` instead.
    - Returns a notice: `imported N categories, M tasks from TODO.md` followed by one line per warning. Otherwise the notice is `""`.
  - `func openForRead(cmd *cobra.Command, path string, isDefault bool) (*todo.Document, error)`: when there's no db and no import source, it returns `&todo.Document{}` and creates nothing. Otherwise it uses `openStore` + `Load` + `Close`.
  - CLI writes print the notice (if any) to stderr before their normal output.
  - `--line`/`--parent-line` are replaced by `--id`/`--parent-id` in `addSelectorFlags` and `resolveSelector` (`doc.ByID`), including the ambiguous-title hint (`--number N  (--id N, <category>)`).
  - `list` table header: `"  #    id  st   item"`, row format `"%3d %5d  %-4s %s%s\n"` with `it.ID`. JSON `jsonItem`: `Line` is replaced by `ID uint `json:"id"``.
  - `todo import <file.md> [--force]`:
    - Imports into the resolved db.
    - Prints `imported N categories, M tasks from <file>` plus warnings.
    - On `ErrNotEmpty`, exits 1 with `<db> already has items; use --force to replace them`.
  - The `add-category` depth limit (`categories nest at most 6 levels deep`) is removed.
  - Root `Short`: `"A minimalistic terminal TODO manager"`. `Use`: `"todo [db-file]"`. `Long` describes `.todo.db` and the automatic `TODO.md` import.
  - In Task 5, `RunE` still calls `tui.Run(path)` on the resolved path. Task 6 changes that.

**How writes flow in commands** (the same for each): resolve the path, `openStore`, `Load`, resolve the selector on that doc, and validate the kind. Then:
- `add` / `add-category`: insert a fresh `Item` with the existing `AppendTask` / `AppendChild` / `AppendRoot`, take `doc.Placement(newItem)`, and call `AddTask` / `AddCategory`.
- `done`/`progress`/`defer`/`reopen`: `SetStatuses` or `CascadeStatus`.
- `edit`: `Edit`. `rm`: `Delete`. `prune`: `Prune`.

The confirmation lines print exactly as today.

- [ ] **Step 1: Rewrite the test helpers.** `writeTemp(t, md)` becomes `todotest.Seed(t, md)`, and `readFile(t, path)` becomes `todotest.Dump(t, path)`. Existing assertions on markdown text keep working, because `Dump` renders canonical markdown. Change every `--line N` test to `--id N`, getting the ID from a `Load` of the seeded db. Change the list-table and JSON golden strings from line to id.

- [ ] **Step 2: Add the new tests:**
  - `TestListMissingDBCreatesNothing`: in an empty temp cwd (`t.Chdir`), `todo list` exits 0, prints the header, and no `.todo.db` exists afterwards.
  - `TestAutoImportOnFirstRun`: the cwd has `TODO.md`. `todo list` prints the imported items, `.todo.db` exists, stderr has the `imported` notice, and the `TODO.md` bytes are unchanged. A second run prints no notice.
  - `TestAutoImportOnlyForDefaultPath`: with `-f other.db` and a `TODO.md` present, there's no import and `other.db` is empty.
  - `TestMarkdownPathIsUsageError`: `todo list -f TODO.md` gives exit code 2 and the message contains `todo import`.
  - `TestImportCmd`, `TestImportCmdRefusesNonEmpty` (exit 1, mentions `--force`), and `TestImportCmdForce`.
  - `TestSelectById`: `todo done --id <id>` marks that task. `--id 999` gives exit code 3.

- [ ] **Step 3: Run** `go test ./app/cmd/`. Expected: FAIL.

- [ ] **Step 4: Implement** `store.go`, `import.go`, and switch every command as described above.

- [ ] **Step 5: Run** `go test ./...`. Expected: PASS.

- [ ] **Step 6: Commit**
```bash
git add app/cmd
git commit -m "feat(cli): store todo lists in SQLite with --id selectors and markdown import"
```

---

### Task 6: TUI on the store

**Files:**
- Modify: `app/tui/tui.go`, `app/tui/reload.go`, `app/tui/tree.go`, `app/cmd/root.go`, `internal/todo/model.go` (ID-keyed `DoneStates`), and every `app/tui/*_test.go`, `internal/todo/model_test.go`

**Interfaces:**
- Consumes: `Store` and all its writes, `Load`, `Revision`, `Placement`, `ByID`; `openStore` / `resolvePath` (Task 5).
- Produces:
  - `func Run(st *todo.Store, notice string) error` and `func newModel(st *todo.Store, notice string) (model, error)`. `notice` becomes the initial footer status. The header shows `filepath.Base(st.Path())`.
  - Root `RunE`: `resolvePath` → `openStore` (the TUI creates the db at start, as `EnsureFile` did) → `defer st.Close()` → `tui.Run(st, notice)`.
  - `model`:
    - `lastContent string` is replaced by `store *todo.Store` and `lastRev int64`.
    - `snapshots map[*todo.Item]todo.DoneStates` becomes `map[uint]todo.DoneStates`.
  - `tree.collapsed` becomes `map[uint]bool`, keyed by item ID. `newTree`, `rebuild`, fold/unfold and the caret all key by `it.ID`.
  - `todo.DoneStates` becomes `map[uint]Status`. `SnapshotDone(it)` keys by `ID`. `RestoreDone` is replaced by `func (d *Document) ApplyStatuses(s DoneStates)`, which sets in-memory statuses by ID and skips missing ones.
  - `func (m *model) commit(focusID uint, write func(ctx context.Context) error)`:
    - Runs `write`. On error it sets `m.err`, otherwise it clears it.
    - Then **always** reloads: `Load` → `doc`, `lastRev`.
    - Rebuilds the tree, keeping the collapsed IDs that still exist, the viewport height and the offset.
    - Selects `focusID` if present, otherwise keeps the cursor index clamped.
    - Snapshots are kept, since this is the TUI's own write.
  - The poll checks `Revision` off the UI goroutine. When it differs from `lastRev`, the poll `Load`s and sends `storeReloadedMsg{doc *todo.Document, rev int64}`.
  - `handleReload`:
    - Ignores a message whose rev equals `lastRev`.
    - Defers while a modal is open or an item is grabbed, as today.
    - Otherwise swaps in the reloaded doc, restoring selection and folds by ID.
    - **Clears snapshots**, because this was an external change.
    - Sets the status to `reloaded — list changed externally`.
    - A `Revision`/`Load` error yields no message, so the view is never wiped.
  - The handlers keep their in-memory mutation (which works out the placement), then call `commit`:

    | Handler | Store call |
    |---|---|
    | `submitForm` add | `AddTask` / `AddCategory` with `doc.Placement(newItem)`, focusing the returned ID |
    | `submitForm` edit | `Edit` |
    | `toggleDone` cascade | `CascadeStatus(id, Done)`, with the snapshot taken before |
    | `toggleDone` restore | `SetStatuses(snap)` |
    | `toggleDone` single | `SetStatuses({id: st})` |
    | `setStatus` | `SetStatuses({id: st})` |
    | `doDelete` | `Delete`, focusing `prev.ID` |
    | `doClearDone` | `Prune`, with the status text `Removed %d completed task(s).` using the store's count |
    | `applyMove` | `p, a := doc.Placement(it)` then `Move(ctx, it.ID, p, a)`, focusing `it.ID`, with grab mode kept |

  - The category level cap (6) in `submitForm` is removed.

- [ ] **Step 1: Update the test helper.** `newTestModel(t, src)` seeds with `todotest.Seed`, opens the store, registers `Cleanup(Close)` and returns `(model, dbPath)`. `readFile` becomes `todotest.Dump`. Update reload tests to write through a second `OpenStore` handle instead of `os.WriteFile`. Update snapshot assertions to ID keys.

- [ ] **Step 2: Add the new tests:**
  - `TestExternalWriteReloadsKeepingSelection`:
    1. Select task "b" and fold category "A2".
    2. A second store handle adds a task under "A".
    3. Run `reloadCheck` and feed its message to `Update`.
    4. Assert: the new task is visible in `m.doc`, "b" is still selected, "A2" is still folded, and the status reads `reloaded — list changed externally`.
  - `TestOwnWriteDoesNotReportReload`: toggle a task, then run `reloadCheck`, which returns nil because the rev equals `lastRev`.
  - `TestUndoSurvivesOwnReloadClearedByExternal`:
    1. Cascade-complete a parent (subtasks open → done), then un-toggle. The subtask states are restored.
    2. Cascade again, apply an external write, then un-toggle. Everything is set to Open, because the snapshot was cleared.
  - `TestWriteErrorShowsAndReloads`: close the store, then toggle. `m.err` is non-nil, and the footer contains `save failed:`.

- [ ] **Step 3: Run** `go test ./app/tui/`. Expected: FAIL.

- [ ] **Step 4: Implement** the changes described in the Interfaces block. Delete `reloadCheck`'s file read and the `fileReloadedMsg` type.

- [ ] **Step 5: Run** `go test ./...`. Expected: PASS.

- [ ] **Step 6: Smoke test.**
  1. Run `cd $(mktemp -d) && cp <repo>/example.md TODO.md && go run <repo> list | head -5`. Expected: the stderr notice `imported 11 categories, N tasks from TODO.md`, then the table.
  2. Launch the TUI in that directory, toggle a task, and in another shell run `todo add --parent-title Work --title live`.
  3. Expected: within about 1s the TUI shows `live` and `reloaded — list changed externally`, with the selection unchanged.

- [ ] **Step 7: Commit**
```bash
git add app internal/todo
git commit -m "feat(tui): run the TUI on the SQLite store with revision-based live reload"
```

---

### Task 7: Remove markdown persistence, docs, verify

**Files:**
- Modify: `internal/todo/markdown.go`, `guide.go`, `model.go`, `lookup.go`, `move.go`, `internal/todo/todotest/todotest.go`, `README.md`, `DOCUMENTATION.md`, and the relevant `*_test.go`
- Delete: `internal/todo/render.go`, `render_test.go`, `bootstrap_test.go` (EnsureFile), plus the `guide_test.go` cases that cover writing the guide

**Interfaces:**
- Removes:
  - `Load`, `EnsureFile`, `Save`, `FileContent`, the guide text constant, `Document.Render`, `Document.Preamble`, `Item.Line`, `ByLine`, `Parse` (tests use `parseMarkdown` or `ImportMarkdown`), and the 1..6 clamp in `relevel`.
  - `Status.Marker()` stays, because the CLI uses it.
- Keeps: `guideRange` (used by `parseMarkdown`).
- Moves: the renderer goes into `todotest.Render(doc *todo.Document) string`, with its existing output unchanged (`markerFromStatus` is replaced by `Status.Marker()`).

- [ ] **Step 1: Delete and move** as listed. Fix compile errors by migrating each test to `ImportMarkdown`/`parseMarkdown` and `todotest.Render`. Drop the assertions on `Line`.

- [ ] **Step 2: Run** `go test ./... && go vet ./...`. Expected: PASS. Also run `grep -rn "\.Line\b\|ByLine\|EnsureFile\|FileContent\|lastContent" --include=*.go .`. Expected: no matches.

- [ ] **Step 3: Update the docs.**
  - `DOCUMENTATION.md`:
    - The Usage section covers `todo` opening `.todo.db`, the automatic one-time import of `TODO.md` (the file is left untouched and can be deleted), and `todo import file.md [--force]`.
    - The Scripting section uses `--id` instead of `--line`, with list columns `# · id · status · item`. `--id` is stable, and `--number` shifts.
    - Live reload covers changes from `todo` CLI commands in another process.
    - Replace the "File format" and guide-block sections with a short "Storage" section: SQLite file, WAL side files `.todo.db-wal`/`.todo.db-shm`, and whether to commit or ignore `.todo.db*` being the repo owner's choice, since a binary file can't be merged.
  - `README.md`: update any markdown-format wording.

- [ ] **Step 4: Run** `make verify`. Expected: all targets pass, with coverage ≥ 70% for `internal/todo`, `internal/todo/todotest`, `app/cmd` and `app/tui`. If `todotest` falls short, add a `todotest_test.go` covering `Seed`/`Dump`/`Render`.

- [ ] **Step 5: Commit**
```bash
git add -A internal app README.md DOCUMENTATION.md
git commit -m "refactor(todo): drop markdown persistence and document SQLite storage"
```
