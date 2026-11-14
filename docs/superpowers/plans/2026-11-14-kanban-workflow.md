# Kanban Workflow Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Turn todo's four flat task statuses into a four-column kanban board:
Backlog, Next, In Progress and Done, each with its own states. The TUI shows one
tab per column. Tasks get an optional plan link and at most one level of
subtasks. Existing v1 databases are migrated.

**Architecture:** The `Status` int stays the only stored state. Its column is
derived from it in one mapping (`Status.Column()`). Each tab is a filter over
the loaded `Document`, built with the same path-to-match logic that search
already uses. The store enforces the subtask rules inside its write
transaction. A schema v1→v2 step in `migrate()` backs up the file, flattens
nesting through closure-tree moves, and normalizes subtask statuses.

**Tech Stack:** Go 1.26, `go-bumbu/closure-tree` v0.10, `go-bumbu/db/gormdb` (pure-Go SQLite), Bubble Tea / Lipgloss, Cobra.

**Spec:** `docs/superpowers/specs/2026-11-14-kanban-workflow-design.md`

**Before Task 1:** the working tree is on `main`. Ask Andrés which feature
branch to use. Do not create or switch branches on your own. Commit messages
are a single line with no co-author trailer.

## Global Constraints

- Status numbers are stored data and pinned explicitly: `Open=0, Working=1, Deferred=2, Done=3, New=4, Ready=5, Review=6, Followup=7`. Never use `iota` for them, and never sort by the number.
- State names (CLI, JSON): `open deferred new ready working review followup done`. `progress` is accepted as an **input alias** for `working`.
- Column names (CLI `--column`, JSON `column`): `backlog next progress done`. Tab titles: `Backlog`, `Next`, `In Progress`, `Done`.
- A subtask is a task whose parent is a task. A subtask has no children, and its status is only `open` or `done`.
- `schemaVersion` = 2. The `plan` column is `gorm:"not null;default:''"`.
- Inside a store transaction, use only `tx` and the tree bound to it, never `s.db`.
- Pure Go only, no CGO.
- Exit codes are unchanged (`0/1/2/3/4/5`). Rule violations are usage errors (exit 2).
- `make verify` (test, vet, lint, license-check, coverage) must pass at the end.

## Review Focus

1. **An unknown stored status** (e.g. `9`, written by a future version) must render as `status(9)` and appear in Backlog, never crash or vanish. Tested in Task 1 and Task 7.
2. **Opening a v1 database twice** (two agents, or a retry after a crash) must migrate exactly once and leave the data identical after the second open. Tested in Task 5.
3. **An empty tab** (e.g. Next with no tasks): keys like `]`, `[`, `t`, `x`, `e`, `d`, `m` on an empty selection must do nothing and must not panic. Tested in Task 8.
4. **Another process moves the selected task to another column** while the TUI is on that tab: the reload must keep the cursor on a valid row (the nearest one), not on a hidden item. Tested in Task 7.
5. **A search filter active while switching tabs:** the filter carries over to the new tab and is combined with the column filter, and `esc` still clears it. Tested in Task 7.

---

### Task 1: Status, Column and plan in the domain model

**Files:**
- Modify: `internal/todo/model.go`, `internal/todo/markdown.go` (`statusFromMarker`), `internal/todo/todotest/todotest.go` (`Render`)
- Modify (mechanical rename `InProgress` → `Working` so it compiles): `app/tui/*.go`, `app/cmd/*.go`
- Test: `internal/todo/status_test.go`, `internal/todo/model_test.go`

**Interfaces:**
- Produces:
  - `type Status int` with the pinned constants `Open, Working, Deferred, Done, New, Ready, Review, Followup`.
  - `func (s Status) String() string`: the state name, or `fmt.Sprintf("status(%d)", s)` for an unknown value.
  - `func StatusFromName(name string) (Status, bool)`: the eight names plus `progress`.
  - `func (s Status) Marker() string`: `" "`, `"/"`, `">"`, `"x"` for Open, Working, Deferred, Done, and `""` for any other state.
  - `func (s Status) Column() Column`: an unknown value maps to `ColBacklog`.
  - `func (s Status) SubtaskAllowed() bool`: true only for Open and Done.
  - `func (s Status) NextInColumn() Status`: `open↔deferred`, `new↔ready`, `working→review→followup→working`, `done→done`, unknown → itself.
  - `type Column int` with `ColBacklog, ColNext, ColInProgress, ColDone` (`iota` is fine here because columns aren't stored).
  - `func Columns() []Column`
  - `func (c Column) String() string`: `backlog|next|progress|done`
  - `func (c Column) Title() string`: `Backlog|Next|In Progress|Done`
  - `func ColumnFromName(string) (Column, bool)`
  - `func (c Column) States() []Status`: Backlog `[Open, Deferred]`, Next `[New, Ready]`, InProgress `[Working, Review, Followup]`, Done `[Done]`.
  - `func (c Column) FirstState() Status`
  - `Item.Plan string` (task only).
  - `func (it *Item) IsSubtask() bool`: true for a task whose parent is a task.
  - `func (it *Item) NeedsPlan() bool`: true for a task with `Status == Ready` and `strings.TrimSpace(Plan) == ""`.
  - `func StatusAfterMove(cur Status, toSubtask bool, oldParent Status) Status`:
    - becoming a subtask: Done stays Done, anything else → Open;
    - a subtask becoming a task: Done stays Done, otherwise `oldParent`. If `oldParent` is itself Done, the result is `Working`.
    - Tasks 2 and 4 both use it.

- [ ] **Step 1: Write the failing tests**

```go
func TestStatusNumbersArePinned(t *testing.T) {
	want := map[Status]int{Open: 0, Working: 1, Deferred: 2, Done: 3, New: 4, Ready: 5, Review: 6, Followup: 7}
	for s, n := range want { if int(s) != n { t.Errorf("%v = %d, want %d", s, int(s), n) } }
}
func TestStatusNamesRoundTrip(t *testing.T) // every Column().States() value: StatusFromName(s.String()) == s
func TestStatusFromNameProgressAlias(t *testing.T) // StatusFromName("progress") == (Working, true); ("bogus") ok == false
func TestUnknownStatusIsVisible(t *testing.T) // Status(9).String() == "status(9)"; Status(9).Column() == ColBacklog
func TestStatusColumn(t *testing.T) // table: Open,Deferred→ColBacklog; New,Ready→ColNext; Working,Review,Followup→ColInProgress; Done→ColDone
func TestNextInColumn(t *testing.T) // Open→Deferred→Open; New→Ready→New; Working→Review→Followup→Working; Done→Done
func TestColumnNames(t *testing.T) // String/Title/ColumnFromName for all four; ColumnFromName("x") ok == false
func TestSubtaskAllowed(t *testing.T) // true only Open, Done
func TestNeedsPlan(t *testing.T) // Ready+"" → true; Ready+"  " → true; Ready+"docs/p.md" → false; New+"" → false
func TestIsSubtask(t *testing.T) // category→task: false; task→task: true
func TestStatusAfterMove(t *testing.T) // (Review,true,_)→Open; (Done,true,_)→Done; (Open,false,Ready)→Ready; (Done,false,Ready)→Done; (Open,false,Done)→Working
```

- [ ] **Step 2: Run them and confirm they fail**

Run: `go test ./internal/todo/ -run 'Status|Column|NeedsPlan|IsSubtask|NextInColumn' -v`
Expected: build failure (undefined `Working`, `Column`, …).

- [ ] **Step 3: Implement the model changes**

- In `model.go`, implement the Interfaces above. Remove `InProgress`.
- `statusFromMarker` keeps the mapping `" "→Open`, `"/"→Working`, `">"→Deferred`, `"x"/"X"→Done`.
- `todotest.Render` writes `[<Marker()>]` when the marker isn't empty. Otherwise it writes `[<String()>]` (e.g. `- [ready] t`), a test-only rendering that is never parsed back.
- Rename every `todo.InProgress` use in `app/` to `todo.Working`. No behaviour changes there yet.

- [ ] **Step 4: Run all tests**

Run: `go test ./...`
Expected: PASS. CLI tests that print `progress` names may need their expected strings changed from `progress` to `working`. That's the only accepted change.

- [ ] **Step 5: Commit**

`git add internal/todo app` then `git commit -m "feat(todo): add kanban columns and states to the status model"`

---

### Task 2: Flattening and one-level move rules

**Files:**
- Create: `internal/todo/flatten.go`
- Modify: `internal/todo/move.go` (`Indent`, `Outdent`)
- Test: `internal/todo/flatten_test.go`, `internal/todo/move_test.go`

**Interfaces:**
- Consumes: `StatusAfterMove`, `IsSubtask`, `SubtaskAllowed` (Task 1).
- Produces:
  - `func (d *Document) FlattenSubtasks() []*Item`. It re-parents every task deeper than one subtask level to its top-level task, at its pre-order position. It sets every subtask's status to Done if it was Done, otherwise Open. It returns the moved items **in their final order** under each top-level task, so a caller can persist them one by one with `d.Placement(it)`.
  - `Indent`/`Outdent` enforce one subtask level and adjust status through `StatusAfterMove`.

- [ ] **Step 1: Write the failing tests**

```go
func TestFlattenKeepsReadingOrder(t *testing.T) {
	d := todotest.Parse(t, "# C\n- [ ] T\n  - [ ] a\n    - [ ] a1\n      - [ ] a1x\n  - [ ] b\n")
	moved := d.FlattenSubtasks()
	// T's children are now a, a1, a1x, b, all subtasks with no children
	// titles(moved) == [a1, a1x]
}
func TestFlattenNormalizesSubtaskStatus(t *testing.T) // "  - [/] w" and "  - [>] d" → Open; "  - [x] x" → Done; top-level "[/]" stays Working
func TestFlattenNoopOnFlatTree(t *testing.T)          // returns nil; tree unchanged
func TestIndentUnderSubtaskRefused(t *testing.T)      // "- [ ] T\n  - [ ] a\n  - [ ] b": Indent(b) returns false (prev sibling a is a subtask)
func TestIndentTaskWithChildrenRefused(t *testing.T)  // "- [ ] P\n- [ ] T\n  - [ ] a": Indent(T) returns false
func TestIndentNormalizesStatus(t *testing.T)         // T was Review → after Indent under P, T.Status == Open; a Done task stays Done
func TestOutdentTakesParentState(t *testing.T)        // parent Ready, subtask Open → after Outdent, Status == Ready; parent Done → Working
```

- [ ] **Step 2: Run them and confirm they fail**

Run: `go test ./internal/todo/ -run 'Flatten|Indent|Outdent' -v`
Expected: FAIL (`FlattenSubtasks` undefined, Indent accepts depth 3).

- [ ] **Step 3: Implement `FlattenSubtasks` in `flatten.go` and the guards in `move.go`**

- For each top-level task, collect its descendants in pre-order (`Descendants()`) and make that list the new children of the task (parent set, no grandchildren).
- `Indent` returns false when `prev` is a subtask, or when `it` is a task that has children. Otherwise it sets `it.Status = StatusAfterMove(it.Status, true, prev.Status)`.
- `Outdent` of a subtask sets `StatusAfterMove(it.Status, false, parent.Status)`.

- [ ] **Step 4: Run tests**

Run: `go test ./internal/todo/...`
Expected: PASS. Existing `move_test.go` cases that indent to depth 3 must change to assert refusal.

- [ ] **Step 5: Commit**

`git commit -m "feat(todo): flatten subtasks and limit nesting to one level"`

---

### Task 3: Column filtering in the domain

**Files:**
- Modify: `internal/todo/filter.go`
- Test: `internal/todo/filter_test.go`

**Interfaces:**
- Consumes: `Status.Column()`, `IsSubtask` (Task 1).
- Produces:
  - `func (d *Document) VisibleWhere(match func(*Item) bool) map[*Item]bool`: the current path-to-match + subtree-of-match logic, with the predicate pulled out. `VisibleItems(q)` becomes a thin wrapper (blank query → nil, as today).
  - `func (d *Document) ColumnItems(c Column) map[*Item]bool`: `VisibleWhere(task && !subtask && Status.Column()==c)`. For `ColBacklog` it also adds **every category**. The result is never nil: an empty map means nothing shows.
  - `func (d *Document) ColumnCounts() map[Column]int`: counts top-level tasks per column.
  - `func (it *Item) CountInColumn(c Column) int`: counts top-level tasks under category `it` in column `c`.

- [ ] **Step 1: Write the failing tests**

```go
const board = "# A\n## Empty\n# B\n- [ ] backlogTask\n- [/] working\n  - [ ] sub\n- [x] doneTask\n"
func TestColumnItemsKeepsCategoryPath(t *testing.T)   // ColumnItems(ColInProgress): B, working, sub visible; A, Empty, backlogTask, doneTask not
func TestColumnItemsBacklogShowsAllCategories(t *testing.T) // ColumnItems(ColBacklog): A, Empty, B, backlogTask visible; working not
func TestColumnItemsEmptyColumn(t *testing.T)         // ColumnItems(ColNext) is non-nil and empty
func TestSubtaskFollowsParentColumn(t *testing.T)     // sub never decides visibility: a "[x]" subtask under a working task shows in ColInProgress, not ColDone
func TestColumnCounts(t *testing.T)                   // {Backlog:1, Next:0, InProgress:1, Done:1}
func TestCountInColumn(t *testing.T)                  // B.CountInColumn(ColInProgress) == 1; A.CountInColumn(ColBacklog) == 0
func TestVisibleItemsUnchanged(t *testing.T)          // existing VisibleItems tests still pass (no edits)
```

- [ ] **Step 2: Run them and confirm they fail.** `go test ./internal/todo/ -run 'Column|Visible' -v` → undefined `ColumnItems`.

- [ ] **Step 3: Implement as described in Interfaces.**

- [ ] **Step 4: Run tests.** `go test ./internal/todo/...` → PASS.

- [ ] **Step 5: Commit.** `git commit -m "feat(todo): filter the tree by board column"`

---

### Task 4: Store: plan field, rule checks, cascade, newer-schema guard

**Files:**
- Modify: `internal/todo/store.go` (`dbItem.Plan`, `itemFromRow`, `schemaVersion = 2`, `revisionIn` guard), `internal/todo/store_write.go`, `internal/todo/import.go` (`insertItems` writes `Plan`)
- Modify callers so it compiles: `app/cmd/add.go`, `app/cmd/modify.go`, `app/tui/tui.go`
- Test: `internal/todo/store_write_test.go`, `internal/todo/store_test.go`

**Interfaces:**
- Consumes: Task 1 (`SubtaskAllowed`, `StatusAfterMove`).
- Produces:
  - `var ErrTooDeep = errors.New("subtasks cannot have subtasks")`
  - `var ErrSubtaskStatus = errors.New("a subtask can only be open or done")`
  - `var ErrNewerSchema`: the existing "created by a newer todo" message as a sentinel, wrapped with the version number.
  - `type Fields struct { Title, Description, Plan string }`
  - `func (s *Store) AddTask(ctx context.Context, f Fields, st Status, parentID, afterID uint) (uint, error)`
  - `func (s *Store) Edit(ctx context.Context, id uint, f Fields) error`: a category ignores `Description` and `Plan`.
  - `SetStatuses(ctx, map[uint]Status)` and `Move(ctx, id, parentID, afterID)` keep their signatures but gain the checks below.
  - `CascadeStatus(ctx, id, st)` keeps its signature. It sets `id` to `st` and its descendants to `Done` if `st == Done`, otherwise `Open`.

**Checks** (all inside the `write` transaction, against rows as they are at write time; one helper `kindAndParent(tx, id)` reads a row's kind, status and parent through the closure table):
- `AddTask`: if the parent is a subtask → `ErrTooDeep`. If the parent is a task and `!st.SubtaskAllowed()` → `ErrSubtaskStatus`.
- `Move` of a task:
  - to a task parent: if the moved task has children, or the new parent is a subtask → `ErrTooDeep`.
  - otherwise the status is rewritten with `StatusAfterMove(cur, toSubtask, oldParentStatus)` in the same transaction.
- `SetStatuses` / `CascadeStatus`: setting a subtask to a non-allowed state → `ErrSubtaskStatus`.
- `revisionIn` (used by `Load`) and `write` return `ErrNewerSchema` when `meta.schema_version > schemaVersion`.

- [ ] **Step 1: Write the failing tests**

```go
func TestPlanRoundTrip(t *testing.T)                  // AddTask with Plan "docs/p.md" → Load shows it; Edit with Plan "" clears it
func TestEditCategoryIgnoresPlan(t *testing.T)
func TestAddUnderSubtaskIsTooDeep(t *testing.T)       // errors.Is(err, ErrTooDeep)
func TestAddSubtaskWithBoardStateRejected(t *testing.T) // AddTask(..., Review, parent=task) → ErrSubtaskStatus
func TestMoveTaskWithChildrenUnderTaskIsTooDeep(t *testing.T)
func TestMoveIntoSubtaskNormalizesStatus(t *testing.T) // a Review task moved under a task → Open
func TestMoveSubtaskOutTakesParentState(t *testing.T)  // subtask Open under a Ready task, moved under the category → Ready
func TestSetStatusesRejectsBoardStateOnSubtask(t *testing.T)
func TestCascadeDoneAndReopen(t *testing.T)           // Cascade(id, Done): all Done; Cascade(id, Working): parent Working, subtasks Open
func TestCascadeOnSubtaskWithBoardStateRejected(t *testing.T)
func TestLoadRefusesNewerSchema(t *testing.T)         // open, then set meta.schema_version=3 through a second gorm handle; Load → errors.Is(err, ErrNewerSchema)
func TestWriteRefusesNewerSchema(t *testing.T)        // same setup; AddCategory → ErrNewerSchema
```

- [ ] **Step 2: Run them and confirm they fail.** `go test ./internal/todo/ -run 'Plan|TooDeep|Subtask|Cascade|NewerSchema|Move' -v` → FAIL / build errors.

- [ ] **Step 3: Implement the store changes.** Update callers to pass `todo.Fields{...}` (behaviour unchanged, plan empty). `checkSchemaVersion` reuses `ErrNewerSchema`.

- [ ] **Step 4: Run tests.** `go test ./...` → PASS.

- [ ] **Step 5: Commit.** `git commit -m "feat(todo): store plan links and enforce one-level subtasks"`

---

### Task 5: v1→v2 migration and flattening on import

**Files:**
- Create: `internal/todo/migrate.go` (move `migrate`/`checkSchemaVersion` out of `store.go`)
- Modify: `internal/todo/store.go`, `internal/todo/import.go` (`Store.Import` calls `doc.FlattenSubtasks()` before `insertItems`)
- Test: `internal/todo/migrate_test.go`, `internal/todo/import_test.go`

**Interfaces:**
- Consumes: `FlattenSubtasks`, `Placement` (Task 2), `write` (Task 4).
- Produces:
  - `func BackupPath(dbPath string) string` → `dbPath + ".bak"`.
  - Internally, `migrate()` becomes:
    1. read the stored version (0 = new store);
    2. if it's newer → `ErrNewerSchema`;
    3. if it's 1 and `BackupPath` doesn't exist → `VACUUM INTO '<bak>'`. An existing `.bak` is **kept**, not overwritten: a plan refinement of the spec, so a racing or retried migration never replaces the original v1 backup;
    4. `tree.Migrate()` + AutoMigrate meta (this adds `plan`);
    5. a new store creates meta with version 2; a v1 store runs `upgradeToV2(ctx)`.
  - `upgradeToV2` runs through `s.write`:
    - re-read `schema_version` inside `tx`, and return nil if it's ≥ 2;
    - `loadDocument(ctx, tx)` then `FlattenSubtasks()`;
    - for each moved item in order, `tree.Update(ctx, it.ID, nil, &parentID, &afterID, storeScope)` with `parentID, afterID := doc.Placement(it)`;
    - write `status` for every subtask, grouped by value;
    - set `schema_version = 2`. The revision bump comes from `write`.

- [ ] **Step 1: Write the failing tests**

Test helper `seedV1(t) string` in `migrate_test.go`:
- open a store, then add rows through `closuretree.New(s.db, dbItem{})` + `addRow`, bypassing the Task 4 checks: category C → task T (Working) → sub a (Review) → subsub a1 (Deferred) → a1x (Done), plus sub b (Open);
- set `meta.schema_version = 1` and close;
- return the path.

```go
func TestMigrateV1FlattensAndNormalizes(t *testing.T) // OpenStore(seedV1) → T children titles == [a, a1, a1x, b]; a,a1,b Open, a1x Done; T stays Working; meta version 2
func TestMigrateV1WritesBackup(t *testing.T)           // BackupPath exists and opens as a store with schema_version 1 and 5 tasks
func TestMigrateKeepsExistingBackup(t *testing.T)      // pre-create .bak with known bytes → bytes unchanged after migration
func TestMigrateRunsOnce(t *testing.T)                 // OpenStore twice in a row (and two handles opened before either closes): data equal, revision bumped exactly once
func TestNewStoreIsV2(t *testing.T)                    // fresh OpenStore → schema_version 2, no .bak created
func TestImportFlattensDeepMarkdown(t *testing.T)      // ImportMarkdown + Import of "- [ ] T\n  - [/] a\n    - [ ] a1" → T children [a, a1], a Open
```

- [ ] **Step 2: Run them and confirm they fail.** `go test ./internal/todo/ -run 'Migrate|NewStoreIsV2|ImportFlattens' -v` → FAIL.

- [ ] **Step 3: Implement `migrate.go` and the import call.**

- [ ] **Step 4: Run tests.** `go test ./...` → PASS. `todotest.Seed` fixtures with deep nesting now come out flattened. Update any test that relied on depth 3.

- [ ] **Step 5: Commit.** `git commit -m "feat(todo): migrate databases to schema v2 with flattened subtasks"`

---

### Task 6: CLI: states, columns, plan

**Files:**
- Modify: `app/cmd/status.go`, `app/cmd/add.go`, `app/cmd/modify.go` (`edit`), `app/cmd/list.go`, `app/cmd/store.go` (error mapping), `app/cmd/root.go` (register `status`, help text)
- Test: `app/cmd/status_test.go`, `app/cmd/add_test.go`, `app/cmd/list_test.go`, `app/cmd/modify_test.go`

**Interfaces:**
- Consumes: Tasks 1, 3 and 4 (`StatusFromName`, `ColumnFromName`, `ColumnItems`, `Fields`, the sentinels).
- Produces (CLI surface):
  - `todo status --to <state>` with the standard selectors. An invalid state → usage error listing the eight names.
  - Shortcuts: `progress` → Working, `defer` → Deferred, `reopen` → Working (`--cascade`: subtasks Open), `done` (`--cascade` unchanged).
  - Confirmation line: `"<verb>: [<state>] <title>  (<path>)"`, using `Status.String()` instead of `Marker()`.
  - `todo list --column backlog|next|progress|done`. It combines with the existing filter by intersecting `ColumnItems` with the filter set. The table header is `# · id · state · item`, and the state cell is `Status.String()`.
  - JSON `jsonItem` gains `Column string json:"column,omitempty"` (tasks only, `Status.Column().String()`) and `Plan string json:"plan,omitempty"`.
  - `add --plan <s>`. `--status` defaults to `open`, and its help lists the eight names.
  - `edit --set-plan <s>` (empty clears it; tasks only, otherwise `wrongKind`). This follows the existing `edit --set-*` flags instead of the spec's `modify --plan`, because the command is named `edit`.
  - `ErrTooDeep` and `ErrSubtaskStatus` map to `usageErr(err.Error())` (exit 2) where `ErrNotFound` is mapped in `store.go`.

- [ ] **Step 1: Write the failing tests**

```go
func TestStatusTo(t *testing.T)               // status --to ready --title a → Dump contains "- [ready] a"
func TestStatusToInvalid(t *testing.T)        // --to bogus → exit 2
func TestStatusToProgressAlias(t *testing.T)  // --to progress → working
func TestReopenGoesToWorking(t *testing.T)    // done task, reopen → "- [/] a"
func TestReopenCascadeOpensSubtasks(t *testing.T)
func TestSubtaskBoardStateIsUsageError(t *testing.T) // progress on a subtask → exit 2
func TestAddUnderSubtaskIsUsageError(t *testing.T)   // add --parent-title <subtask> → exit 2
func TestAddWithPlan(t *testing.T)            // add --plan docs/p.md → list --json has "plan":"docs/p.md"
func TestEditSetPlan(t *testing.T)            // --set-plan x then --set-plan "" → plan absent from JSON
func TestListColumnFilter(t *testing.T)       // board fixture; list --column progress shows the working task and its category only
func TestListShowsStateNames(t *testing.T)    // table row contains "working", not "[/]"
func TestListJSONColumnAndStatus(t *testing.T) // "status":"working","column":"progress"
```

- [ ] **Step 2: Run them and confirm they fail.** `go test ./app/cmd/ -v` → FAIL.

- [ ] **Step 3: Implement the CLI changes.** Update expected strings in existing tests where the confirmation or table format changed.

- [ ] **Step 4: Run tests.** `go test ./app/cmd/...` → PASS.

- [ ] **Step 5: Commit.** `git commit -m "feat(cli)!: speak kanban states, columns and plan links"`

---

### Task 7: TUI tabs and filtered views

**Files:**
- Create: `app/tui/tabs.go` (tab bar rendering + per-tab state)
- Modify: `app/tui/tree.go` (`rebuild`, `rowString`), `app/tui/tui.go` (keys, `header`, `adopt` on reload, `n`/`N`/`c` placement), `app/tui/styles.go` (glyph styles)
- Test: `app/tui/tabs_test.go`, existing TUI tests

**Interfaces:**
- Consumes: Task 3 (`ColumnItems`, `ColumnCounts`, `CountInColumn`), Task 1 (states, `NeedsPlan`, `FirstState`).
- Produces:
  - `tree.column todo.Column`.
  - `func (t *tree) setColumn(c todo.Column)`: saves the current cursor's item ID and fold map under the old column, restores the new column's (empty folds and top selection the first time), and rebuilds.
  - `tree.foldsByCol map[todo.Column]map[uint]bool`, `tree.selByCol map[todo.Column]uint`. `tree.collapsed` stays the active tab's map.
  - `rebuild` uses `vis := ColumnItems(t.column)`. When a filter is set it intersects with `VisibleItems(filter)`. Collapse still applies when only the column filter is active, and is ignored while searching (as today). The `+ new category` placeholder row appears only on Backlog with no search. An empty tab renders a single dim line `nothing in <Title>` and has no selectable rows.
  - Keys in main mode: `tab`/`shift+tab` cycle tabs, `1`–`4` jump to a tab. The app starts on `ColBacklog`.
  - Header: a tab bar line under the title, `Backlog 12   [Next 3]   In Progress 2   Done 40`, from `ColumnCounts`, with the active tab in `selectedRowStyle`.
  - Category row note: ` (N)`, where N = `CountInColumn(t.column)`. On Backlog it keeps today's `(done/total)`.
  - Task row: glyph per state: open `○`, deferred `⏸`, new `◌`, ready `◉`, working `◐`, review `◑`, followup `◒`, done `●`. Subtasks show only `○`/`●`. After the title comes a dim state name (top-level tasks only) and, when `NeedsPlan()`, ` ⚠ no plan` in `deferStyle`.
  - New items: `n`/`N` on a task or category creates a task with `t.column.FirstState()`. A subtask is created `Open`. `N` on a subtask is refused with footer `subtasks can't have subtasks`. `c`, category rename/delete (`e`/`d` on a category) and moving a category (`m` on a category) work only on Backlog. Elsewhere the footer says `categories are managed in Backlog`.
  - Reload (`adopt`): if the focused ID is no longer visible in the current tab, select the row at the old cursor index, clamped.

- [ ] **Step 1: Write the failing tests**

Add the helper `newTestModelOn(t, src string, c todo.Column) (model, string)` in `helpers_test.go` (it calls `newTestModel` and then `setColumn`).

```go
func TestStartsOnBacklog(t *testing.T)                 // board fixture: rows include "backlogTask" and "Empty", not "working"
func TestTabKeysSwitchColumns(t *testing.T)            // press "tab" → column Next; "3" → InProgress; "shift+tab" → Next
func TestNonBacklogHidesEmptyCategories(t *testing.T)  // InProgress tab: rows == [B, working, sub]
func TestEmptyTabShowsNothingLine(t *testing.T)        // Next tab View() contains "nothing in Next"; selected() == nil
func TestTabKeepsOwnSelectionAndFolds(t *testing.T)    // fold B on InProgress, switch to Backlog (B expanded), back → B folded
func TestTabBarCounts(t *testing.T)                    // View() contains "Backlog 1", "Next 0", "In Progress 1", "Done 1"
func TestNewTaskTakesTabFirstState(t *testing.T)       // on Next, select category B, "n", type "idea", enter → store has idea=New
func TestCategoryKeysBacklogOnly(t *testing.T)         // on Next: "c" → footer "categories are managed in Backlog", no form
func TestReadyWithoutPlanFlagged(t *testing.T)         // a Ready task with no plan on Next: row contains "⚠ no plan"
func TestUnknownStatusRendersInBacklog(t *testing.T)   // set status 9 via SetStatuses on a second handle → Backlog row shows "status(9)"
func TestExternalMoveKeepsCursorValid(t *testing.T)    // on InProgress with "working" selected, second handle sets it Done → after reload tick, selected() is a visible row (or nil if the tab is empty), no panic
func TestSearchCarriesAcrossTabs(t *testing.T)         // "/" "work" enter on Backlog → no match rows; "3" → shows "working"; "esc" clears filter
```

Update existing TUI tests whose fixtures act on `[x]`/`[/]` items to start on the right tab with `newTestModelOn`.

- [ ] **Step 2: Run them and confirm they fail.** `go test ./app/tui/ -run 'Tab|Backlog|Empty|NewTask|Category|Plan|Unknown|External|Search' -v` → FAIL.

- [ ] **Step 3: Implement tabs as specified.**

- [ ] **Step 4: Run tests.** `go test ./app/tui/...` → PASS.

- [ ] **Step 5: Commit.** `git commit -m "feat(tui): show the board as one tab per column"`

---

### Task 8: TUI transitions, undo and moves inside a tab

**Files:**
- Modify: `app/tui/tui.go` (`updateMainItemKey`, `toggleDone`, remove `setStatus` for `p`/`>`, `footer`, `applyMove`)
- Test: `app/tui/status_test.go`, `app/tui/move_test.go`

**Interfaces:**
- Consumes: Task 1 (`NextInColumn`, `FirstState`), Task 4 (`CascadeStatus` semantics), Task 7 (`tree.column`).
- Produces (key behaviour):
  - **`]` / `[`** on a top-level task: the target column is ±1 (no wrap). The task gets `target.FirstState()`.
    - Into Done: same path as completing (snapshot + `CascadeStatus(id, Done)` when it has children).
    - Out of Done: parent `Working`. If a snapshot exists, restore its subtask entries (the parent's entry excluded) and drop it.
    - On a subtask, or at the edge column: footer hint, no write.
  - **`t`**: `SetStatuses({id: s.NextInColumn()})`. On a subtask: hint.
  - **`x` / `space`** on a task:
    - subtask: toggle Open/Done;
    - top-level not done: complete as today;
    - done with snapshot: restore the snapshot exactly (parent included);
    - done without snapshot: set `Working`.
  - `p` and `>` removed.
  - When the selected task leaves the tab, the cursor stays at the same row index (clamped) and the footer reads `moved to <Title> — <state>`.
  - **Grab mode in a filtered tab** (column ≠ Backlog, or search active): up/down repeat the domain `MoveUp`/`MoveDown` until the item has passed one visible row or the move returns false, then persist once. Indent and outdent go through the Task 2 rules. A refused move shows `subtasks can't have subtasks`.
  - Footer hints: `↑↓ Move  ←→ Fold  tab Tab  ]/[ Column  t State  space Done  / Search  n/N New  c Cat  m Move  enter/e Edit  y Copy  d Del  D Clear  q Quit`.

- [ ] **Step 1: Write the failing tests**

```go
func TestBracketMovesToNextColumn(t *testing.T)       // Backlog task, "]" → store New; selection leaves tab; footer "moved to Next — new"
func TestBracketBackFromDoneIsWorking(t *testing.T)   // Done tab, "[" → Working
func TestBracketIntoDoneCascadesWithUndo(t *testing.T) // Review task with 2 open subtasks, "]" → all Done; on Done tab "x" → parent Review again (snapshot), subtasks Open
func TestBracketOnSubtaskIsHint(t *testing.T)         // no store change
func TestBracketAtEdgeIsNoop(t *testing.T)            // "[" on Backlog, "]" on Done
func TestStateCycleKey(t *testing.T)                  // Next task New, "t" → Ready, "t" → New
func TestUndoAccidentalCompleteRestoresExactState(t *testing.T) // Next task New: "x" then (on Done tab) "x" → New
func TestReopenWithoutSnapshotIsWorking(t *testing.T) // seeded done task: on Done tab "x" → Working
func TestSubtaskToggle(t *testing.T)                  // "x" on subtask → Done, again → Open
func TestOldKeysRemoved(t *testing.T)                 // "p" and ">" change nothing
func TestEmptyTabKeysAreNoops(t *testing.T)           // Next tab empty: "]", "[", "t", "x", "e", "d", "m" → no panic, store revision unchanged
func TestGrabMoveStepsOverHiddenTasks(t *testing.T)   // "# C\n- [/] w1\n- [ ] b\n- [/] w2" on InProgress: "m", "up", "m" → store order under C == [w2, w1, b]
func TestIndentUnderSubtaskRefusedInTUI(t *testing.T)
```

- [ ] **Step 2: Run them and confirm they fail.** `go test ./app/tui/ -run 'Bracket|State|Undo|Reopen|Subtask|OldKeys|EmptyTab|Grab|Indent' -v` → FAIL.

- [ ] **Step 3: Implement the key behaviour.**

- [ ] **Step 4: Run tests.** `go test ./app/tui/...` → PASS.

- [ ] **Step 5: Commit.** `git commit -m "feat(tui): move tasks across columns and states"`

---

### Task 9: TUI dialog: column, state and plan

**Files:**
- Modify: `app/tui/form.go`, `app/tui/tui.go` (`openForm`, `submitForm`, `itemMeta`)
- Test: `app/tui/form_test.go`, `app/tui/tui_test.go`

**Interfaces:**
- Consumes: `Fields`, `Edit`, `AddTask` (Task 4).
- Produces:
  - `newForm(titleVal, descVal, planVal string, isTask, editing bool) form`, with a single-line `plan textinput.Model` (CharLimit 500).
  - `ring()` for a task: title, desc, plan, save, cancel.
  - `values() (title, desc, plan string)`. The plan is `strings.TrimSpace`d.
  - `itemMeta(it)` for a task: `<glyph> <Column.Title> · <state>`, then the subtask progress as today, then `  ·  ⚠ no plan` when `NeedsPlan()`.

- [ ] **Step 1: Write the failing tests**

```go
func TestFormPlanFieldInRing(t *testing.T)     // task form: tab, tab → focus on plan; category form has no plan field
func TestEditSavesPlan(t *testing.T)           // "e", tab tab, type "docs/p.md", save → store Plan == "docs/p.md"; flag disappears on Next
func TestItemMetaShowsColumnAndState(t *testing.T) // Ready task → contains "Next · ready" and "⚠ no plan"
```

- [ ] **Step 2: Run them and confirm they fail.** `go test ./app/tui/ -run 'Form|EditSavesPlan|ItemMeta' -v` → FAIL.

- [ ] **Step 3: Implement.**

- [ ] **Step 4: Run tests.** `go test ./app/tui/...` → PASS.

- [ ] **Step 5: Commit.** `git commit -m "feat(tui): edit a task's plan link in its dialog"`

---

### Task 10: Docs and full verification

**Files:**
- Modify: `README.md`, `DOCUMENTATION.md`, `example.md` (if `make run` should show every column, add tasks with `[/]` and `[x]`)

- [ ] **Step 1: Update `DOCUMENTATION.md`**
  - Add a **Board** section: columns, states, the one-level subtask rule, the plan link and the "no plan" flag.
  - Rewrite the **Keys** table: tabs, `]`/`[`, `t`, `x`/`space`. Remove `p` and `>`.
  - Update the CLI sections: `status --to`, `list --column`, the `state` column, JSON `column`/`plan`, `add --plan`, `edit --set-plan`, the `progress` alias, `reopen` → working.
  - Add a **Upgrading** note: schema v2, the `.todo.db.bak` backup, flattening.
- [ ] **Step 2:** Update the README's one-paragraph description to mention the kanban tabs.
- [ ] **Step 3: Run the full suite.** `make verify` → all targets pass. Fix lint and coverage findings.
- [ ] **Step 4: Manual check.** `make run` and switch through the four tabs. Move a task Backlog → Next → In Progress → Done and back. Add a plan.
- [ ] **Step 5: Commit.** `git commit -m "docs: document the kanban board"`
