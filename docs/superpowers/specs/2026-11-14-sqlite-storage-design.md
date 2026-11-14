# SQLite storage — design

Date: 2026-11-14 · Branch: `feat/sqlite-storage`

## Intent

todo stores its list in one markdown file (`TODO.md`). The whole file is parsed
into an in-memory tree and rewritten on every save. That made the file easy to
edit by hand, but it's a poor base for what comes next: tasks and subtasks that
can be managed individually, then **tags** and a **kanban** view. This change
moves storage to SQLite on top of the go-bumbu libraries (`closure-tree`, `db`).
The result is that every category and task is a row with a stable ID.

This step is a **migration with feature parity**. Everything the TUI and the
CLI do today keeps working, except the parts that only made sense for a
markdown file (listed under Non-goals).

## Goals

- Persist the category/task tree in a per-directory SQLite database.
- Keep every current behaviour, with the same keys, CLI commands and exit codes:
  the four statuses, descriptions, cascade-complete and its in-session undo,
  move/indent/outdent, search, copy, delete, prune, and live reload.
- Give every item a stable ID that survives reloads and edits.
- One-time import of existing `TODO.md` files.
- A schema that tags (many-to-many) and kanban (grouping by status) can extend
  later without a rewrite.

## Non-goals (this step)

- Tags, kanban, task "actions", or any other new field.
- A global, cross-repo database.
- Keeping markdown as a live format, an export, or something agents edit.
- Postgres or MySQL support. SQLite only, though `gormdb` keeps that door open.

## Anti-goals

- Losing data during import, or silently importing a partial file.
- A slower or flakier TUI. Key presses must stay instant, and a TUI running
  next to CLI writes from another process must never fail with "database is
  locked" or lose a write.
- Requiring CGO. Release builds (goreleaser) must stay pure Go.

## Decisions made with Andrés

- **Markdown is a one-time import only.** After the import nothing reads or
  writes markdown. The guide block, the file-format docs and `--line`
  addressing all go away.
- **One database per directory.** Lists stay per repo, as today.
- **Approach A:** `closure-tree` holds the tree, and `gormdb` opens SQLite.
- Task "actions" are out of scope for now.

## Approaches considered

- **A. `closure-tree` for the hierarchy (chosen).** Categories and tasks are
  nodes of one tree. The library handles parents, sibling order (fractional
  `sort_order`), moves, cycle checks and subtree deletes. The same library
  then serves the tag forest (nested tags, "this tag and its sub-tags"
  queries). The cost is depending on its API and calling renormalize now and
  then.
- **B. Plain GORM table with `parent_id` and a sort column.** Fewer
  dependencies and full control, but it re-implements ordering, moves and
  subtree queries that `closure-tree` already has and tests, and it would have
  to re-implement them again for nested tags.
- **C. Swap only the save format.** Keep the in-memory document and write the
  whole tree to the database on every save. This is the fastest way to parity,
  but tasks stay a blob rewritten on every save, which works against the reason
  for the change. Rejected.

## Design

### Libraries

- `github.com/go-bumbu/db/gormdb` with `gormdb/sqlite`, the pure-Go glebarez
  driver. It opens one writer connection plus a reader pool and sets WAL and a
  busy timeout, so a TUI and a CLI process can share the file safely.
- `github.com/go-bumbu/closure-tree` v0.10.x. It takes `*uint` for parent and
  after-sibling, and every operation takes a tenant.
- `gormdbtest.Sqlite(t, sqlite.Driver{})` for tests, which uses a temp file
  rather than `:memory:`.
- Licenses (MIT, BSD, LGPL-3.0) are already in `allowedLicenses.json`. Running
  `make license-check` must pass.

### Where the data lives

- The default database is `.todo.db` in the current directory. It is created on
  the first write. A read-only run (`todo list`) on a directory with no database
  and no `TODO.md` prints an empty list and creates nothing.
- `-f/--file` and the positional argument now name a database file. Passing a
  path ending in `.md` exits with a usage error that points to `todo import`.
- The `closure-tree` tenant ("scope") is a single constant string defined in
  one place in the store, because each database holds one repo's list.

### Schema

There is one node table managed by `closure-tree`, explicitly named `items`
(`TableName()`), so that a later `tags` tree can sit beside it (see "Ready for
tags"). The item struct embeds `closuretree.Node`:

- `Kind`: category or task.
- `Title`.
- `Status`: open, progress, deferred or done. It is stored as the existing
  `Status` int, and only tasks use it.
- `Description`: tasks only.

`closure-tree` adds the closure table, its sort-order metadata table and its
lock table. todo adds one small `meta` table holding a `revision` counter (see
Live reload) and a `schema_version` for later migrations.

The category `Level` is no longer stored. It is derived from depth when the
tree is loaded. The 1..6 cap existed only because markdown has six heading
levels, and it is dropped *(builder's call, low risk)*.

### Domain layer (`internal/todo`)

The in-memory `Document`/`Item` tree stays as the **read model**. Search,
counts, lookup, the cascade logic, and the move/indent/outdent rules keep
working on it unchanged. The changes:

- `Item` gains `ID uint`, the node ID. `Line` is removed.
- `parse.go` survives only as the importer (see Import), together with the
  guide-block detection it uses to skip the guide (`guideRange` in
  `guide.go`). The markdown renderer, `Save`, `EnsureFile`, `FileContent` and
  the guide-writing code are deleted.
- The snapshot behind cascade-undo (`DoneStates`), the TUI's fold set and the
  TUI's selection are all keyed by **ID** instead of `*Item` pointer or title
  path. Snapshots survive the TUI's *own* write-then-reload, but are cleared by
  an *external* reload and by any structural or content change, exactly as
  today. That way an undo never writes old statuses over an agent's change
  *(Claude's call)*.
- The rule that "a category's direct tasks come before its subcategories" is
  **kept**. Markdown no longer forces it, but the move rules (`crossUp` and
  `crossDown`) and what the user sees depend on it.

A **store** in `internal/todo` (for example `store.go`) is the only code that
touches the database. It wraps the `closure-tree` `Tree` and the `*gorm.DB` and
exposes operations in todo's terms:

- `Load() → *Document`. One `TreeDescendants` call from the root, mapped into
  `Item`s with parents, children and levels set.
- Adding a category or task under a parent, after a given sibling (or first).
- Updating a title and description.
- Setting the status of a set of IDs in one transaction. The cascade sets a
  parent and all its descendants. The undo restores per-ID statuses.
- Moving an item to a new parent, after a sibling.
- Deleting a subtree, and deleting many subtrees in one go (for prune).
- `Revision()`.

**How writes flow.** The caller first applies the change to the in-memory
`Document` with the existing domain functions. Those functions already
encode where things go: `AppendTask`, `MoveUp`/`MoveDown`, `Indent`,
`Outdent`, `RemoveDone`, and so on. The caller then reads the item's resulting
parent and previous sibling from the tree and persists that placement through
the store (for example `Move(id, parentID, prevSiblingID)`). This keeps the
current move semantics exactly, without re-deriving them in SQL. After any
write, the caller reloads the `Document` from the store, so memory and disk
never drift apart. The list is small, so a full load is cheap.

**Gotcha.** `Tree.Update(id, item, …)` overwrites *every* payload column,
zero values included (`buildUpdateMap` in closure-tree's `move.go`). Passing a
partly filled item would wipe the other columns, for example resetting `Kind`
to category. Field updates should therefore use an explicit column map, or the
store's own `UPDATE … WHERE node_id IN ?`, which is fine because the store owns
this table. Use `Tree.Update` only for moves, with a nil item.

**Fresh data inside the transaction.** The TUI defers external reloads while a
dialog is open, so its in-memory tree can be stale when it writes. Operations
whose effect depends on the current tree are therefore re-derived from the
database inside the store transaction, not taken from the caller's memory:

- prune computes the fully-done subtrees itself, so a subtask an agent just
  added under a done task is never deleted;
- the cascade sets the descendants that exist at write time;
- delete removes the subtree as it is at write time.

Field edits (title, description) are last-write-wins, as today. This is still
better than today, where a TUI save rewrote the whole file over any external
edit. No revision guard or conflict error *(Claude's call)*.

**Transactions (pattern from codex).** Every `closure-tree` write runs in its
own transaction. When the `Tree` is bound to a transaction handle, though, that
write becomes a savepoint *inside* the caller's transaction. A `Tree` bound to
the store's root handle and used inside a transaction would instead reach for
a second connection and deadlock on SQLite's single writer. So each store write
opens one `db.Transaction` and builds a tree bound to `tx`, as codex's
`treeIn(tx)` does (`closuretree.New(tx, item{})`). The tree's writes, the
status updates and the revision bump then all go through `tx` only. This makes
every store operation atomic, including multi-subtree prune and the import.
Inside the transaction, use only `tx`, never the root `db` (a `gormdb` rule).

**Sort-order maintenance.** After each structural write, the store calls
`NeedsRenormalizeAny` / `RenormalizeAll(DefaultHalvingsBuffer)`.

### Ready for tags (not built now)

Tags come later, and they'll need **several independent, selectable trees**,
for example "Area", "Priority" or "Context". This step only has to avoid
blocking that. The target model follows codex's media tags
(`andresbott/codex`, `internal/model/media/tags.go`):

- **A second `closure-tree` node table, `tags`**, separate from `items`.
  `closure-tree` v0.10 supports several node models in one database (its index
  names are scoped per table). The task tree and the tag forest never share
  rows, IDs or sort order.
- **Tag groups are the roots of that forest.** Each node has a `Kind`, either
  `group` or `tag`. A group ("Priority") sits at the root and is a container
  only: it is never assigned to a task, and selecting it filters nothing. Tags
  sit under a group, and can nest under other tags. Each group is one
  independent selectable tree, and a filter then treats each group as its own
  dimension, as codex's facets do.
- **Assignments go in a join table, `item_tags`** (`item_id`, `tag_id`). It
  has a unique key on the pair and a second index led by `tag_id`, for "tasks
  with this tag" and for deleting a tag. Filtering by a tag including its
  sub-tags is a `DescendantIds` call on the tag tree, then a join.
- **Both trees use the same constant scope.** Groups are distinguished by
  `Kind`, not by tenant, so one `TreeDescendants` call returns the whole tag
  forest.

What this step does to stay compatible: it names the task table `items`, keeps
the scope as one constant, and uses the `tx`-bound tree pattern above. Adding
tags later is then additive: a new table, a join table and a migration, with no
change to `items`. Kanban groups tasks by `status`, or later by a chosen tag
group, which needs no schema change now either.

### Live reload

Every store write increments `meta.revision` inside its transaction (see
Transactions). The TUI keeps polling once a second, but now compares
`Revision()` with the last revision it saw or wrote, instead of re-reading a
file. Its own writes record the revision they produced, so they don't trigger
a "changed externally" reload. On an external change it reloads the
`Document` and restores selection and folds by ID. The existing rules stay:
defer while a modal is open or an item is grabbed, and never wipe the view on
a transient read error. The status line reads "reloaded — list changed
externally".

`PRAGMA data_version` was considered and rejected. It is reported
per connection, which doesn't fit `gormdb`'s writer-plus-readers pools, while a
counter column is plain and portable.

### Import

- **Automatic:** when the database doesn't exist and `TODO.md` exists in the
  current directory (default path only, not when `-f` names something else),
  the first command — TUI or CLI — imports it into a new `.todo.db` and prints
  `imported N categories, M tasks from TODO.md`. In the TUI this shows on the
  status line.
- **Explicit:** `todo import <file.md>` imports into the target database. It
  refuses a non-empty database unless given `--force`, which replaces the
  contents *(builder's call on the flag name)*.
- The import reuses today's lenient parser, so the *content* of a file never
  fails an import. An import fails only on I/O errors, invalid UTF-8 or a
  database error. Every item and its order are inserted in a single
  transaction, and a failed import removes the half-created database file, so
  nothing is left behind.
- Warnings, which are printed but don't stop the import *(Claude's call)*:
  - non-guide preamble text;
  - prose lines the parser drops, including unknown checkbox markers such as
    `- [?]` and indented text under a category.
- Tasks before the first header are imported into a category named
  `Uncategorized`, because tasks must live under a category. An empty file
  imports as an empty list *(Claude's call)*.
- The guide block is skipped without a warning.
- `TODO.md` is never modified or deleted. The user removes it.

### TUI (`app/tui`)

The behaviour and key map are unchanged. Internally:

- `newModel` opens the store, which may run the auto-import, and loads the
  `Document`. `save()` and `lastContent` are replaced by store calls and the
  revision.
- Each mutating handler (toggle, set status, add, edit, delete, prune, move)
  follows the "apply in memory → persist placement or fields → reload" flow
  above. A failed write shows `save failed: …` on the footer, as today, and the
  view reloads from the database, so it never shows an unsaved state.
- The header shows the database's base name where it showed the file name.
- Moves while an item is grabbed persist on each step, as saves do today.

### CLI (`app/cmd`)

- `--line` is replaced by **`--id N`** (and `--parent-id`). `--number` and
  `--title` stay. Exactly one selector is still required. Exit codes are
  unchanged (`0/1/2/3/4/5`).
- `todo list` columns become `# · id · status · item`. The `--json` output
  replaces `line` with `id` and keeps `number`, `title`, `status` and `path`.
  The ambiguous-title error lists `--number`/`--id` alternatives.
- Every command opens the store, writes through it and exits. The TUI notices
  the change through the revision.
- New: `todo import <file.md> [--force]`.
- The root help text drops "markdown TODO manager" wording.

### Docs

- Update `README.md` and `DOCUMENTATION.md`: the storage location, import, the
  `--id` selector, and live reload watching the database. Remove the file-format
  and guide-block sections.
- **Sharing the list through git is the repo owner's decision.** todo never
  touches `.gitignore`. The docs state the trade-off neutrally: ignore
  `.todo.db*` for a local-only list, or commit it, accepting that a binary file
  can't be merged. Git worktrees get no special handling: each directory has
  its own database, like any other directory. This repo's own `TODO.md` and
  `.gitignore` are left as they are.

### Testing

- Store tests on `gormdbtest.Sqlite`: add, order, move, delete, prune, status
  sets including back to Open, revision bumps, and Load round-trips.
- Import tests: `example.md` imports with exact structure, order, statuses and
  descriptions. A guide block produces no tasks. A failed import (for example
  invalid UTF-8) leaves no database behind. Preamble and dropped prose produce
  warnings. Root tasks land in `Uncategorized`.
- A prune test where a subtask is added through a second store handle after the
  TUI loaded, and survives the prune.
- The existing domain tests (model, move, filter, lookup) keep passing on the
  in-memory tree.
- TUI and CLI tests switch their fixtures from markdown strings to a seeded
  temp database. A seed helper that imports a markdown string keeps them
  readable.
- One test where a second store handle writes while the TUI model polls,
  asserting a reload with selection preserved by ID.

## Left to the builder

- Exact file and package layout inside `internal/todo` (store, importer).
- Names of store methods and of the `--force` flag.
- Whether prune deletes subtrees with repeated `DeleteRecurse` calls inside the
  one transaction, or with a store-owned query.
- The migration/versioning mechanism behind `schema_version` (GORM
  AutoMigrate is fine for v1).
