# Kanban workflow — design

Date: 2026-11-14

## Intent

Now that the list lives in SQLite (see `2026-11-14-sqlite-storage-design.md`),
Andrés wants to formalize how a task moves from idea to finished as a kanban
board. His loop:

1. Go to the **Backlog** and pull items into **Next**.
2. In Next, refine each item until it's **ready**, meaning it has an
   implementation plan.
3. Implement it. The task is **In Progress** and goes through rounds of
   review and follow-up fixes.
4. Complete it. It's **Done**.

The TUI gets one tab per column. The CLI, which scripts and AI agents use,
speaks the same vocabulary. The categories (`frontend/imageviewer`, …) stay as
the classification of tasks. They never move. Only tasks change column.

## Goals

- Four columns with fixed states. Each state belongs to exactly one column:

  | Column      | States                          | Meaning |
  |-------------|---------------------------------|---------|
  | Backlog     | `open`, `deferred`              | ready to be picked / parked |
  | Next        | `new`, `ready`                  | idea captured / has an implementation plan |
  | In Progress | `working`, `review`, `followup` | being built / in review / working through follow-up items |
  | Done        | `done`                          | complete |

- A TUI tab per column, each showing the category tree filtered to that
  column's tasks.
- An optional **plan link** on every task.
- **One level of subtasks.** Subtasks are a checklist for implementation
  steps and follow-up items ("launch a full agent review", "validate with
  sample data"). They travel with their parent task.
- A one-time migration of existing databases. It flattens deeper nesting and
  maps statuses.
- The CLI covers the whole workflow for agents.

## Non-goals

- Custom or user-defined states and columns.
- Gating transitions. Movement between columns is free, so a `new` task can
  jump straight into In Progress.
- WIP limits, per-column ordering separate from the tree order, swimlanes.
- Tags. The tag design in the SQLite spec stays future work.
- Nesting deeper than one level of subtasks.

## Anti-goals

- Losing or mangling data during the v1→v2 migration, or leaving a half-migrated
  database.
- An older `todo` process (a TUI left running) writing v1-rules data into a v2
  database.
- A board where one piece of work is scattered across tabs, as would happen if
  a parent and its subtasks could sit in different columns.
- Breaking agent scripts silently. Old status names keep working on input, and
  the JSON change ships as an announced breaking change.

## Decisions made with Andrés

- The column/state set above, and its meaning.
- Tabs are filtered views of the unchanged category tree. Moving
  `frontend/imageviewer → task` to Next shows `frontend › imageviewer › task`
  in the Next tab.
- Empty categories show only in Backlog.
- One level of subtasks. Existing deeper nesting gets flattened.
- "Ready" is backed by a **plan link field** on the task (approach A for the
  plan question).
- Approach **A** below, with the refinements from the Pike review.
- **SQLite storage and the kanban board ship in one release.** The SQLite work
  (#22) is on `main` but untagged (the latest tag `v0.7.1` is markdown-only),
  so schema v1 exists only on dev builds. No released binary can write v1-rule
  data into a v2 database, so no earlier "version guard" release and no
  database triggers are needed. The v1→v2 migration still runs for dev
  databases. The release notes cover both breaking changes together.

## Approaches considered

- **A. One stored state, column derived from it (chosen).** Extend the
  existing `Status` int with the new states and derive the column through one
  mapping function. Existing values keep their numbers, so the status side of
  the migration is almost free. A state/column mismatch cannot be represented.
  Tabs are filters over the loaded `Document`.
- **B. Separate stored column and state.** This stores the same fact twice and
  makes rows like (Backlog, `working`) possible, so every write and import has
  to validate. It only pays off with per-column ordering or WIP limits, which
  are non-goals. Rejected.
- **C. The board as a tag group** in the future tags forest. Tags are not
  built, and tags are many-to-many, so "one column per task" would have to be
  enforced outside the schema. The cascade, the undo and the "ready without
  plan" flag would hang off tag names instead of real states. Rejected. The
  SQLite spec already anticipated "kanban groups tasks by status".

## Design

### Domain model (`internal/todo/model.go`)

**Status values are pinned with explicit numbers**, not `iota`, because they
are stored data:

| Value | Name       | Column      | Notes |
|-------|------------|-------------|-------|
| 0     | `open`     | Backlog     | unchanged |
| 1     | `working`  | In Progress | was `InProgress` / `progress` |
| 2     | `deferred` | Backlog     | unchanged |
| 3     | `done`     | Done        | unchanged |
| 4     | `new`      | Next        | new |
| 5     | `ready`    | Next        | new |
| 6     | `review`   | In Progress | new |
| 7     | `followup` | In Progress | new |

- A `Column` type with `Backlog`, `Next`, `InProgress` and `Done` values. It
  needs prefixed constant names (e.g. `ColBacklog`) because `Done` is
  already a `Status`. `Status.Column()` is the **single** state→column
  mapping. Column order and state order within a column come from explicit
  ordered lists (`Columns()`, `Column.States()`). Never sort by the numeric
  value.
- `String()`/`StatusFromName` use the new names. `StatusFromName` also accepts
  the alias `progress` → `working`. An unknown stored value renders visibly
  (e.g. `status(9)`) instead of silently reading as `open`.
- `Marker()` and `statusFromMarker` (markdown import) keep the four checkbox
  markers, mapped to `open`/`working`/`deferred`/`done`. The new states have
  no markdown marker. The CLI's list table shows the state name, not the
  marker (see CLI).
- `Item` gains `Plan string`, a path or URL. It's free text, and the app
  doesn't validate or open it.
- `Item.NeedsPlan()` is true for a `ready` task with an empty plan. The TUI
  flags these.
- `Status.SubtaskAllowed()` is true only for `open` and `done`.
- `IsDone()` is unchanged, as are counts, pruning and the done-cascade.

**Subtask invariants:**

- A task is either a *task* (its parent is a category) or a *subtask* (its
  parent is a task). A subtask has no children.
- A subtask's status is `open` or `done` only.
- A task's column is the column of the whole task. Subtasks appear wherever
  their parent appears.

**Flattening** is a pure domain function on `Document`. Both the migration and
the markdown import use it. It walks each task's subtree in pre-order and
re-parents every deeper descendant to that top-level task, placing each one
after the last item already placed there. The original reading order is
preserved, so a former sub-subtask lands right after its old parent. Subtask
statuses are normalized: `done` stays `done`, and everything else becomes
`open`.

**Moves (`move.go`).** `Indent` can create depth 3 today. It must refuse:

- indenting under a subtask, and
- indenting a task that has subtasks.

When a task is indented into a subtask, its status normalizes (`done` stays,
anything else becomes `open`). When a subtask is outdented into a task, it
**takes its former parent's state** (or stays `done`), so it remains visible in
the tab where the user did the outdent *(Claude's call)*.

**Cascade (behaviour unchanged from today).** In the TUI, completing a task
that has subtasks completes them too and records the in-session undo
snapshot (`DoneStates`). The CLI cascades only with `--cascade`, as today.
A done task with open subtasks stays a legal state, so the store adds no
invariant for it. The cascade only ever sets `done` or `open` on subtasks. It
never pushes a board state such as `review` down to them.

**Reopen and undo.** Un-doing a done task works like this:
- **With an undo snapshot** (it was just completed in this session): restore
  the snapshot exactly, the task's own previous state included. A slip of `x`
  on a `new` task returns it to `new` in Next.
- **Without a snapshot:** the task goes to `working`, not to Backlog, because
  reopening an old done task usually means it isn't finished. Its subtasks
  keep their states.
- CLI `reopen` sets `working`. `reopen --cascade` sets the task to `working`
  and its subtasks to `open`.
*(Claude's calls after the builder check, keeping today's undo semantics.)*

### Store (`internal/todo/store.go`, `store_write.go`)

- `dbItem` gains `Plan string` with `gorm:"not null;default:''"`. SQLite needs
  the default to add a NOT NULL column to existing rows.
- `schemaVersion` becomes **2**.
- **The store is authoritative for the invariants.** The CLI and scripts write
  next to the TUI, so these checks run inside the write transaction against the
  tree as it is at write time, the same way `Prune` already does:
  - `AddTask` under a subtask → sentinel error (e.g. `ErrTooDeep`).
  - `Move` that would put a subtask under a subtask, or a task that has
    children under a task → `ErrTooDeep`.
  - Setting a subtask to a state other than `open`/`done` → sentinel error
    (e.g. `ErrSubtaskStatus`).
  - `CascadeStatus` sets the parent to any state but the descendants only to
    `done` or `open` (e.g. reopen cascade: parent `working`, subtasks `open`).
- **Field edits** gain the plan. `Edit(title, desc)` becomes an edit taking a
  small fields struct (title, description, plan) instead of a growing list of
  positional strings *(builder's call on the exact shape)*.

### Migration v1 → v2

`migrate()` today only creates what's missing. It gains a real upgrade step
that runs when the meta row says `schema_version = 1`:

1. Back up the database to `<name>.bak` (e.g. `.todo.db.bak`) before
   touching anything. Use `VACUUM INTO`, not a plain file copy, because the
   database runs in WAL mode. Overwrite an older `.bak` *(Claude's call)*.
2. AutoMigrate the `plan` column.
3. In **one** transaction (the `write()` pattern, tree bound to `tx`):
   - re-read `schema_version` inside the transaction and stop if it's already 2.
     Every CLI call runs `OpenStore`, so two agents can race to migrate;
   - flatten nesting through `closure-tree` moves (`tree.Update` with a nil
     item), never raw SQL, so the closure table stays consistent;
   - normalize subtask statuses (`done` stays, everything else → `open`);
   - write `schema_version = 2` and bump the revision.
4. Top-level task statuses need no change. Their numbers already mean the
   right thing (`1` is now `working`).

A failed migration rolls back the transaction, leaves the v1 database as it
was, and reports the error. The `.bak` stays.

**Older processes.** `Load` and `write` already read the meta row for the
revision (`revisionIn`). They also check `SchemaVersion` there and refuse to
work with a newer schema. A v1 TUI left running stops writing once the
database is on v2, and shows the existing "upgrade todo" error on its footer.

### Markdown import

`todo import` and the auto-import keep the lenient parser. Afterwards, the
tree is flattened with the shared domain function and subtask statuses are
normalized before insert. The markers map as in the table above.

### TUI (`app/tui`)

**Tabs.** A tab bar sits under the header:

```
 todo · .todo.db
 Backlog 12   [Next 3]   In Progress 2   Done 40
───────────────────────────────────────────────
 ▾ frontend
   ▾ imageviewer
       ◌ Fix zoom                 new
       ● Lazy-load thumbnails     ready ⚠ no plan
         ☐ validate with sample data
         ☑ wire up the cache
```

- Each tab shows the category tree filtered to tasks whose column matches,
  with the category path to each one. `VisibleItems` generalizes to take a
  predicate, which the search reuses. Column membership is judged on tasks
  only. Subtasks follow their parent.
- **Backlog always shows every category**, including empty ones and ones whose
  tasks are all in other tabs. Other tabs show only categories that contain a
  matching task.
- Each task shows its state name and its subtask progress. A `ready` task
  without a plan shows a warning mark. Glyphs extend the existing circle
  family *(builder's call on exact glyphs)*.
- The tab bar shows a task count per column. Category headers count the tasks
  visible in the current tab *(Claude's call)*.
- The app opens on **Backlog**. Each tab keeps its own selection and folds
  for the session *(Claude's call)*.

**Keys.** The existing keys stay unless listed here:

| Key | Action |
|-----|--------|
| `tab` / `shift+tab`, `1`–`4` | Switch tab |
| `]` / `[` | Move the selected task to the next / previous column. It lands in that column's first state (`open`, `new`, `working`, `done`). Moving into Done cascades to subtasks with undo; moving out of Done follows the reopen rule but lands in the target column |
| `t` | Cycle the task's state within its column (`new` ↔ `ready`; `working` → `review` → `followup` → `working`; `open` ↔ `deferred`) |
| `x` / `space` | On a subtask: toggle done. On a task: move to Done (with cascade) or, on a done task, undo/reopen per the rule above |
| `p`, `>` | Removed (replaced by `[`/`]`/`t`) |

`s` is already "move selection down", which is why the state key is `t`
*(Claude's call)*. `]`/`[` on a subtask do nothing (with a footer hint).

**After a task leaves the current tab** (it moved columns), the selection goes
to the nearest remaining row, and the footer says `moved to Next — new`
*(Claude's call)*.

**Adding.**
- `n`/`N` add a task or subtask. A new task takes **the current tab's first
  state**. A new subtask is `open`. `N` on a subtask is refused (too deep).
- Categories (add, rename, delete, move) are managed in **Backlog only**. In
  other tabs those keys show a footer hint *(Claude's call: a category created
  in Next would vanish at once because it's empty there)*.

**Moving tasks in a filtered tab** (`m` grab mode). Reordering steps over
hidden tasks to the next visible sibling. Moving a task across categories
works in every tab. Indent and outdent follow the subtask rules above *(Claude's
call)*.

**Dialog.** It shows column and state, and adds an editable **Plan** field
next to title and description.

**Search** filters within the current tab.

**`D`** (remove completed) is unchanged and works from any tab.

### CLI (`app/cmd`)

- `--status` accepts the eight names. `progress` stays as an input alias for
  `working`.
- New: **`todo status --to <state>`**, using the usual selectors (`--id`,
  `--number`, `--title`). `done`, `progress`, `defer` and `reopen` stay as
  shortcuts: `progress` → `working`, `defer` → `deferred`, `reopen` →
  `working`. `done` and `reopen` keep their opt-in `--cascade` flag.
- `todo list` gains `--column backlog|next|progress|done`. The table shows the
  state name instead of the checkbox marker: `# · id · state · item`.
- `--json` output adds `column` and `plan`. `status` carries the new names
  (`"working"` instead of `"progress"`). **This is a breaking change.** Ship it
  as `feat!` and call it out in the release notes for agent consumers, in the
  same release as the SQLite storage.
- `add` and `modify` gain `--plan <path-or-url>`. `modify --plan ""` clears it.
- `add` without `--status` defaults to `open` (Backlog), matching "capture into
  the backlog" *(Claude's call)*.
- Store sentinel errors (`ErrTooDeep`, `ErrSubtaskStatus`) map to the existing
  usage-error exit code with a clear message.

### Docs

Update `README.md` and `DOCUMENTATION.md`: the board and its states, the tabs
and new keys, the one-level subtask rule, the plan field, the CLI changes, and
the v2 migration with its `.bak` file.

### Testing

- Domain: `Status.Column`, name round-trips including the `progress` alias,
  unknown values rendering visibly, `NeedsPlan`, flattening (order kept,
  statuses normalized), `Indent`/`Outdent` guards and state inheritance.
- Store: invariant rejections inside the transaction (`AddTask` under a
  subtask, deep `Move`, subtask board state). A cascade that only sets
  done/open. Plan round-trip. Migration on a seeded v1 database with deep
  nesting: structure, order, statuses, `schema_version = 2`, `.bak` present.
  The migration runs only once when two store handles open the same v1 file.
  A v1-version check refuses a v2 database.
- TUI: tab filtering, Backlog shows empty categories, other tabs don't, `]`/`[`
  /`t` transitions, the cascade and undo into and out of Done, new-task state
  per tab, category keys blocked outside Backlog, selection after a task leaves
  the tab.
- CLI: `status --to`, `list --column`, JSON `column`/`plan`, `--plan` on
  add/modify, alias input, rejected deep add.

## Left to the builder

- Exact glyphs and colors for the new states and the "no plan" mark.
- The shape of the field-edit API (struct vs. options).
- Sentinel error names and where the flatten helper lives in `internal/todo`.
- Whether the TUI's tab bar and per-tab state live in a new file or in `tui.go`.
- How migration steps are organized for future versions (a simple
  version-switch in `migrate()` is fine).
