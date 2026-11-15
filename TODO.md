<!-- todo:guide — managed by todo; this block is rewritten on save. Docs: https://github.com/candy-tools/todo
This file is a todo list managed by "todo", a terminal TODO app:
https://github.com/candy-tools/todo

todo watches this file and reloads it automatically when it changes on disk, so
you — human or agent — can edit it directly in any editor. Keep to this format
so todo can parse what you write:

  # Heading           Headings ("#" to "######") are categories; they nest by
                      heading level.
  - [ ] Open task     A "- [ ]" line is an open task; "- [x]" marks it done.
  - [/] In progress   "- [/]" flags a task in progress, "- [>]" defers it.
  - [x] Done task     Tasks must live under a category heading.
    - [ ] Subtask     Indent by two spaces to nest a subtask under a task.
    Description text  An indented, non-checkbox line is the task's description.

Notes for editors:
- Text above the first heading (this block included) is preserved on save.
- todo rewrites the file into the canonical form above on every change, so any
  other free-form markdown placed between items is not kept.

Prefer the CLI over editing by hand: it writes this file in the exact format
above. `todo add`, `todo done|progress|defer|reopen`, `todo edit`, `todo rm`
and `todo list --json` cover the common actions — run `todo --help`.
-->

# Main

# auto-improve round 1

- [x] R1-00: make lint fails with 4 issues at base 19172f7
  Baseline: golangci-lint reported 4 issues (tree.go unused searching, rowString gocyclo 21, unified_test.go ineffassign, view.go QF1001). Fixed in 6fd4106, 1fdad69, c6f209a: lint 4 -> 0 issues; tests 424 -> 425 passing; race on app/tui green.
- [x] R1-01: TUI edit overwrites concurrent agent changes made while the form was open
  Concurrent edits (e.g. agent setting a plan while the TUI form is open) were reverted because Store.Edit rewrote every field. Fixed in a3fbd7a: new todo.Changes with optional fields; TUI form and CLI edit send only changed fields. Tests added: TestEditKeepsPlanSetWhileFormOpen, TestEditKeepsFieldsItDoesNotName. Tests 425 -> 427 passing (gate count incl. subtests), lint 0. Note: a no-op edit still bumps the store revision.
- [x] R1-02: Deleted or replaced DB file makes the open TUI write to the unlinked copy
  SQLite kept writing to an unlinked inode after rm/rename-swap of the DB file. Fixed in 64420ad: Store records os.FileInfo at open; Store.CheckFile (os.SameFile) returns ErrFileReplaced; Store.write refuses writes; TUI poll shows the error. Tests: TestWriteRefusesReplacedFile, TestReplacedFileShowsErrorAndRefusesWrite. Tests 427 -> 431 passing, lint 0, race ok. Limits: TUI does not reopen automatically (restart needed); tiny check-then-write window remains.
- [x] R1-03: import --force wipes the store with no backup
  Forced import deleted every item with no backup. Fixed in 7d5c115: backup() returns its path; importFile backs up a non-empty store (VACUUM INTO copy) before replacing; CLI prints the backup path. Tests: TestImportFileReplaceBacksUpNonEmptyStore, TestImportFileReplaceEmptyStoreTakesNoBackup, TestImportCmdForceNamesBackup. Tests 431 -> 434 passing, lint 0, coverage ok. Residual: tiny window between emptiness check and import tx.
- [x] R1-04: Hash comment inside a fenced code block is imported as a category
  Parser treated # lines and checkboxes inside fenced code blocks as headers/tasks. Fixed in 5db8347: CommonMark-style fenceTracker in markdown.go; fenced lines go through the prose path; fences in descriptions end with the description. Tests: TestParseFencedBlockOutsideTaskIsDroppedProse, TestParseFencedBlockInDescription, TestParseUnclosedDescriptionFenceEndsWithTheTask, TestParseFencedDescriptionRoundTrips. Tests 434 -> 438 passing, lint 0, coverage ok.
- [x] R1-05: Store does not enforce tree invariants (category under task, task without category)
  Store accepted categories under tasks, tasks at root, and statuses on categories. Fixed in 9e182c9: checkParent in the write tx (addRow + Move) returns ErrWrongParent; SetStatuses/CascadeStatus return ErrNotTask for categories; CLI maps both to wrong-kind exit 5. 5 store tests + TestStoreTreeRuleIsWrongKind. Tests 438 -> 444 passing, lint 0, coverage ok. Breaking at store level (tasks at root refused); 5 tests rebuilt with a parent category.
- [x] R1-06: Titles are not validated at the store; newlines and control chars reach the TUI
  Store accepted titles with newlines/ESC and no length cap. Fixed in c473ccb: internal/todo/title.go CleanTitle + ErrInvalidTitle + MaxTitleLen=200 used by AddCategory/AddTask/Edit/Import; parser sanitizes titles with warnings; CLI add/add-category/edit return usage errors; TUI form uses MaxTitleLen. Tests: title_test.go, TestAddInvalidTitleIsUsageError, TestEditInvalidTitleIsUsageError. Tests 444 -> 482 passing, lint 0, race ok, coverage ok. TUI description part not reproduced (bubbles textarea already strips ESC).
- [x] R1-07: [complex] TUI database writes block the UI thread
  TUI commit ran the write inside Update; a held SQLite writer lock froze the TUI ~5s. Plan: /tmp/auto-improve-nCtA/R1-07.md. Landed the simple mitigation in c623a71: OpenStore(path, WithBusyTimeout(d)) (default 5s kept for CLI); TUI uses 750ms; busy errors map to todo.ErrBusy ('list busy ... try again'); CLI maps ErrBusy to exit 6 (documented). Freeze 4.3s -> 0.62s in TestStatusChangeWhileLockedShowsBusy. Tests 505 -> 510 passing, lint 0, race ok, coverage ok. Full async rewrite deferred as R1-07b.
- [x] R1-08: [complex] Complete/reopen policy lives in app layer and CLI and TUI disagree
  Plan: /tmp/auto-improve-nCtA/R1-08.md. The CLI/TUI completion difference is intentional and documented; the reopen-target half of the finding was inaccurate. Landed Option A (pure refactor, no behaviour change): 2aad028 pin tests; 2db20cc internal/todo/complete.go (ReopenTarget, ReopenStates, ReopenOptions, Store.Complete, Store.Reopen, ErrRestoreAndCascade; one revision bump each); 8008d39 CLI uses them; 3d1b19c TUI uses them; 64069f3 DOCUMENTATION.md note. Tests 510 -> 525 passing, lint 0, race ok, coverage ok. Unifying behaviour deferred as R1-08b.
- [x] R1-09: Add placement duplicated and racy; closure-tree errors leak to users
  Front ends computed afterID from a stale tree; raw closure-tree errors reached users. Fixed in 4ae903c: Store.AppendTask places after last task inside the tx (used by CLI add and TUI adds); write maps ErrInvalidAfterNode/ErrInvalidMove to ErrConflict; CLI exit 6 (documented in DOCUMENTATION.md); TUI footer message. 7 tests added. Tests 482 -> 489 passing, lint 0, race ok, coverage ok.
- [x] R1-10: TUI silently swallows reload errors
  Poll reload failures (newer schema, corrupt DB) were dropped silently. Fixed in a59bd25: single reloadFailedMsg (replaces storeFileErrMsg) for every poll error; footer shows 'reload failed: ...' once per distinct error; cleared on recovery without clobbering unrelated errors. TestReloadCheckErrorIsNil (asserted old behaviour) replaced by TestReloadCheckErrorIsReported; added TestReloadFailureShownAndCleared/ShownOnce/DistinctShown. Tests 489 -> 492 passing, lint 0, race ok, coverage ok.
- [x] R1-11: When write and reload both fail, unsaved changes stay on screen as if saved
  View kept unsaved in-memory mutations after write+reload failure. Fixed in 5713910: model keeps a deep copy (saved) of the last loaded list (new Document.Clone); on write+reload failure the view is restored from it via adoptAll. Two tests that asserted the old behaviour rewritten with stricter checks; added TestFailedWriteAndReloadRestoresEdit, TestCloneIsDeep. Tests 492 -> 494 passing, lint 0, race ok, coverage ok.
- [x] R1-12: Unclosed todo:guide comment lets example lines import as tasks
  Unclosed guide comment (and any multi-line HTML comment) had its lines parsed as tasks/headers. Fixed in 8a37a6d: commentSpan + lineTracker in internal/todo; comment content never becomes headers/checkboxes; an unclosed comment ends before the next column-0 heading (or EOF) with a warning, so real tasks are not lost. Tests: TestParseUnclosedGuideEndsAtNextHeadingAndWarns, TestParseMultiLineCommentContentIsNotParsed, TestParseUnclosedCommentAtEndRunsToEOF. Tests 494 -> 497 passing, lint 0, coverage ok.
- [x] R1-13: A tab counts as one space of indentation in the parser
  Parser measured a tab as one column, mis-nesting tasks and descriptions. Fixed in 280a385: leadingWhitespace measures columns with tab stop 4; dedent uses stripIndent so tab-straddling cuts keep width. Test: TestParseTabIndentIsFourColumns. Tests 497 -> 498 passing, lint 0, coverage ok. Note: a tab in a description's leading indent is stored as spaces.
- [x] R1-14: Dead FindByPath and outdated comment in path.go
  FindByPath was only used by tests; Path comment described title-path restore. Fixed in 4f48eef: FindByPath and its 2 tests removed (Item.Path kept, used by CLI JSON); comment corrected. Tests 498 -> 496 (2 tests of deleted code removed), lint 0, coverage ok.
- [x] R1-15: Duplicated helpers in internal/todo
  Two duplicated helper pairs. Fixed in b896445 (shared insertBeforeCategories for AppendTask and crossDown) and 7c4d65e (shared writeStatuses for SetStatuses and v2 migration; call-site checks kept). Pinning test TestMoveDownSubcategoryLandsAfterTasksBeforeSubcategories. Tests 496 -> 497 passing, lint 0, coverage ok.
- [x] R1-16: CLI write commands open the database twice
  Write commands opened/migrated/closed the DB for the read, then reopened for the write. Fixed in 4a16915: new loadStore returns the open store; withStore writes through it (creates one only when none existed); openForRead wraps it. Test: TestWriteCommandsOpenTheStoreOnce (7 subtests, via openDB hook). Tests 497 -> 505 passing, lint 0, race ok, coverage ok.
- [x] R1-17: [complex] tui.go is very large and mode state is scattered
  Plan: /tmp/auto-improve-nCtA/R1-17.md. Landed: Part A pure file moves 4382faf..0859920 (commit.go, status.go, edit.go, move.go, search.go, confirm.go, copy.go, render.go, help.go/view.go; tui.go 1429 -> 370 lines, verified move-only); B1 4d0762d 7 pinning tests; B2 4ad09d5 searching folded into modeSearch; B3 67490fd grabbed keys routed from updateKey; B4 eb5fff8 model fields grouped with invariants. Not folded by decision: tree.grabbed (render flag), split/sideFocus (layout). Tests 525 -> 532 passing, lint 0, race ok, coverage ok. Open product question (behaviour kept): reloads apply while the search bar is open, unlike other modes.
- [>] R1-07b: [complex] Full async TUI commit (follow-up to R1-07)
  Deferred: needs the user's call. Plan: /tmp/auto-improve-nCtA/R1-07.md (6 tasks: write closures take store+values only; follow-up 'after' steps; async tea.Cmd commit with pending state, poll paused, keys refused/buffered; WaitGroup so quit never loses a write; type-ahead buffer of 32 dropped on failure; delayed 'saving...' indicator). Why deferred: the R1-07 mitigation (750ms busy timeout + ErrBusy) already caps the freeze under 1s; the full rewrite touches every TUI write path and carries critical risks (losing a write on quit, data races on the Document, typed-ahead keys after a failed write). Question for the user: does 'the TUI never freezes, even briefly' matter enough (e.g. heavy agent writers) to justify it?
- [>] R1-08b: Unify complete/reopen behaviour between CLI and TUI (follow-up to R1-08)
  Deferred: product decision for the user. Plan: /tmp/auto-improve-nCtA/R1-08.md (Option B). Today: TUI completing a parent always cascades to subtasks (with undo snapshot); CLI todo done completes only the task unless --cascade (no undo); status --to done never cascades; CLI reopen also moves not-done tasks to working. Questions: (1) one complete default, or keep the documented difference (planner leans keep: CLI has no undo)? (2) should status --to done / --to X on a done task go through Complete/Reopen? (3) should reopen on a not-done task error or no-op? Mechanism is now in internal/todo (Store.Complete/Reopen), so whichever is chosen is a small change.

# auto-improve round 2

- [x] R2-01: TUI could offer to reopen the store after ErrFileReplaced
  Done in 97283e8 (Close checkpoints and truncates the WAL when the file was replaced, so writes reach the held file and the new file does not read stale WAL pages) and 87d2243 (TUI poll reopens the store at the same path when a file is back, applies it as an external change, defers while a dialog is open or an item is grabbed, never creates a file). Tests 589 -> 593, lint 0, race ok, make coverage ok. Residual: a file deleted between the existence check and OpenStore can still be created empty (the DB library has no no-create open); another process already sharing the WAL with the new file is not addressed.
- [x] R2-02: Control chars still reach output via legacy titles, CLI list descriptions, CRLF descriptions
  Stored control chars reached the terminal via TUI rows and CLI text output; CRLF imports kept \r in descriptions. Fixed in 575026c: todo.Printable (tab -> space, drops C0/DEL/C1) used in TUI rowString, commit status line, CLI list/categoryPath/rm/status text output, and importTitle; description lines drop trailing \r. Tests: TestStoredControlCharactersAreNotDrawn, TestListTableDropsStoredControlCharacters, TestParseCRLFDescriptionDropsCarriageReturns, TestPrintable. Tests 550 -> 554 passing, lint 0, race ok, coverage ok. CLI list text never printed descriptions. Residual: the edit form receives raw stored strings (sanitising would change what is saved).
- [x] R2-03: Category adds still compute placement from the in-memory tree; AddTask afterID only used by tests
  Category adds placed from a stale tree and failed with ErrConflict after concurrent deletes. Fixed in 93fd8af: Store.AppendCategory places after the parent's last child inside the tx (CleanTitle, checkParent, one revision bump); CLI add-category and TUI category form use it. Tests: TestAppendCategoryPlacesLast, TestAppendCategoryRules, TestEveryWriteBumpsRevision row; TestStalePlacementShowsConflict became TestAddCategoryAfterExternallyDeletedSibling (asserts success, exact doc, selection). Tests 536 -> 538 passing, lint 0, race ok, coverage ok.
- [x] R2-04: SetStatuses silently ignores unknown IDs while CascadeStatus returns ErrNotFound
  SetStatuses (and Complete/Reopen without cascade) returned success for deleted IDs. Fixed in 02c6690: tasksOnly check returns ErrNotFound (lowest missing id) without writing; Reopen with a restore map drops vanished subtasks inside the tx. Tests: TestSetStatusesRejectsMissingID, TestReopenRestoreSkipsVanishedSubtask; TestSetStatusesBackToOpen's 999 (old skip behaviour) moved into the strict new test. Tests 542 -> 544 passing, lint 0, race ok, coverage ok.
- [x] R2-05: Column-0 closing fence or unclosed fence in a description drops the rest of the import
  Regression from 5db8347: a dedented closing fence opened a new unclosed top-level fence and dropped the rest of the file. Fixed in dc1347c: dedented fence closes the description's fence; an unclosed top-level fence ends before the next column-0 heading (or EOF) with a warning; commentSpan skips headings inside fences. Tests: TestParseDedentedClosingFenceClosesDescriptionFence, TestParseUnclosedTopLevelFenceEndsAtNextHeadingAndWarns, TestParseUnclosedCommentSkipsHeadingsInFences. Tests 532 -> 535 passing, lint 0, coverage ok.
- [x] R2-06: Own write reported as external change when the reload after it fails
  After a successful write with a failed reload, the next poll reported the TUI's own change as external and wiped undo snapshots. Fixed in d722199: commit sets lastRev=expected and a new unsynced flag; while unsynced the poll reloads even at the same revision and applies it silently keeping snapshots. Test: TestOwnWriteWithFailedReloadNotExternal. Tests 535 -> 536 passing, lint 0, race ok, coverage ok. Residual: the 'reload failed' footer error from commit is not cleared after the recovering reload (external path never cleared it either).
- [x] R2-07: TUI form truncates long descriptions/plans and saving an edit writes the cut text
  bubbles v1.0.0: plan input CharLimit=500 truncated on load (saved cut text); textarea MaxHeight=99 blocked new lines in long descriptions. Fixed in 4c245f7: ta.MaxHeight=0, plan CharLimit removed; changes() unchanged (already compares widget start values so untouched fields are never sent). Tests: TestEditLongDescriptionKeepsAllLines, TestEditLongPlanKeepsWholeValue, TestEditTitleKeepsLongDescriptionAndPlan. Tests 538 -> 541 passing, lint 0, race ok, coverage ok. Residual: bubbles' fixed 10,000-line textarea cap.
- [x] R2-08: import --force backs up and replaces in two steps
  A write between the backup and the replacing import was deleted and absent from the .bak. Fixed in 7ba4439: backupIfNotEmpty returns the revision it read; unexported importDoc checks it inside the write tx and returns ErrConflict writing nothing; CLI import maps ErrConflict to exit 6. Test: TestImportFileReplaceConflictsWithWriteAfterBackup (with/without backup). Tests 547 -> 550 passing, lint 0, race ok, coverage ok. Residual: import does not map ErrBusy to exit 6; the .bak remains after a conflict.
- [x] R2-09: Store.Load reads revision and tree outside one transaction
  Load read revision and tree with separate statements, so an external write between them was mislabelled as the TUI's own. Fixed in 89b8951: Load reads both inside one transaction on gormdb's read pool (no writer lock taken). Test: TestLoadRevisionMatchesTree via unexported loadBetweenReads hook. Tests 546 -> 547 passing, lint 0, race ok, coverage ok.
- [x] R2-10: TUI complete decides cascade from the in-memory tree
  TUI decided cascade from the possibly stale in-memory tree, leaving externally added subtasks open under a done task. Fixed in 8498dd8: TUI complete always calls Store.Complete(ctx, id, true); store cascades over the tree at write time. Test: TestCompleteCascadesToSubtaskAddedExternally. Tests 541 -> 542 passing, lint 0, race ok, coverage ok.
- [x] R2-11: Saving the form with no changes still writes, bumps revision, wipes undo snapshots
  Unchanged form save wrote, bumped the revision and dropped undo snapshots. Fixed in ebbba29: submitForm closes without commit when Changes is empty; Store.Edit with empty Changes returns nil without a write. Tests: TestEditSavedUnchangedWritesNothing, TestEmptyEditKeepsRevision. Tests 544 -> 546 passing, lint 0, race ok, coverage ok. Residual: an edit naming only fields a category ignores still bumps the revision.
- [x] R2-12: ErrConflict also covers invalid moves with a misleading 'reload and try again'
  Moving a category into its own subtree reported 'list changed, reload and try again'. Fixed in f380e16: new ErrInvalidMove ('cannot move an item into its own subtree'); ErrConflict only for stale sibling placement; CLI maps ErrInvalidMove to usage exit 2. Tests: TestMoveUnderOwnDescendantIsInvalidMove (renamed from ...IsConflict, stricter), TestStoreInvalidMoveIsUsage. Tests 576 -> 577 passing, lint 0, race ok, coverage ok. Only the TUI calls Store.Move today.
- [x] R2-13: OpenStore records the file identity after opening
  A file swap between open and identity recording let writes go to the unlinked copy (and its WAL could be replayed onto the new file, wiping it). Fixed in eaccc41: openStoreOnce stats before and after the final open, returns ErrFileReplaced on mismatch; OpenStore retries up to 3 times; a newly created store is reopened once to record identity. Tests: TestOpenStoreFileReplacedWhileOpening (4 subtests), TestOpenStoreFileKeepsBeingReplaced. Tests 581 -> 587 passing, lint 0, race ok, coverage ok. Residual: the read pool opens connections lazily by path, so a later replacement can split readers from the writer.
- [x] R2-14: WithBusyTimeout(0) behaves inconsistently and the TUI's 750ms also limits startup open retries
  WithBusyTimeout(0) skipped open retries; the TUI's 750ms capped startup open/migrate waits. Fixed in 1b493fa: d<=0 means DefaultBusyTimeout; open+migrate always use the default; a non-default timeout reopens the store with the caller's value for later writes. Tests: TestOpenStoreZeroBusyTimeoutRetriesBusyOpen, TestOpenStoreShortBusyTimeoutWaitsAtOpen (WAL and busy-open). Tests 577 -> 581 passing, lint 0, race ok, coverage ok. Note: ImportFile writes from the TUI first-run import also use the short timeout.
- [x] R2-15: CleanTitle accepts invisible format/separator characters
  Titles accepted bidi overrides and line/paragraph separators. Fixed in ac5d8ae: unsafeRune (control, U+202A-202E, U+2066-2069, U+2028/2029, U+FEFF) used by CleanTitle, importTitle and Printable; ZWJ/ZWNJ/soft hyphen still allowed. Tests: extended badTitles, TestImportMarkdownSanitizesTitles, TestPrintable; new TestCleanTitleKeepsJoinersAndSoftHyphen. Tests 554 -> 576 passing (table cases), lint 0, coverage ok.
- [x] R2-16: Dead Store.AddTask and redundant title checks in CLI
  Fixed in 645b0e5 (CLI add/add-category/edit rely on CleanTitle only; exit 2 unchanged; message now '--title: invalid title: it must not be empty'; tests TestAddBlankTitleIsUsageError, TestEditBlankTitleIsUsageError) and 17179ab (AddTask/AddCategory kept exported and documented as the explicit-placement API since cross-package tests depend on afterID; AppendTask added to TestEveryWriteBumpsRevision). Tests 587 -> 589 passing, lint 0, coverage ok.

# auto-improve follow-ups

- [ ] F-01: Read pool reopens connections by path after a file replacement
  From R2-13 residual. internal/todo/store.go: the read pool opens connections lazily by path, so after the file at the path is replaced, new read connections read the new file while the writer still holds the old one (Load/poll see a different DB than writes go to). Fix: pin read connections to the file opened at OpenStore time (open eagerly, keep idle conns, no lifetime/idle expiry), or detect the replacement on read and fail with ErrFileReplaced. Severity medium, effort medium, plausible.
- [ ] F-02: TUI first-run import uses the short 750ms busy timeout
  From R2-14 note. The TUI's first-run ImportFile runs on a store opened with WithBusyTimeout(750ms), so a large import that meets a held lock fails fast. Fix: ImportFile (one large deliberate write) waits with DefaultBusyTimeout regardless of the store's timeout. Severity low, effort small.
- [ ] F-03: import does not map ErrBusy to exit code 6
  From R2-08 residual. app/cmd import maps ErrConflict to exit 6 but not ErrBusy, unlike the other write commands (DOCUMENTATION.md says 6 covers conflict and busy). Fix: map ErrBusy to exitConflict in import. Severity low, effort small.
- [ ] F-04: import --force leaves the .bak file after a conflict
  From R2-08 residual. When import --force backs up and the replacing write then fails with ErrConflict, nothing was overwritten but the .bak stays. Decision (orchestrator default): delete the backup made by this run when the import writes nothing; keep it on success. Severity low, effort small.
- [ ] F-05: Category edit naming only ignored fields bumps the revision
  From R2-11 residual. Store.Edit on a category with Changes that only set fields a category ignores (description, plan) still writes and bumps the revision, dropping TUI undo snapshots. Fix: treat it as a no-op like an empty Changes. Severity low, effort small.
- [ ] F-06: TUI edit form shows raw stored strings
  From R2-02 residual. The edit form is filled with stored title/description/plan as-is, so legacy control characters reach the terminal. Fix: clean the values before filling the form (keep newlines in description and plan); a field the user did not touch must not be written just because cleaning changed it. Severity low, effort small.
- [ ] F-07: reload failed footer error stays after recovery
  From R2-06 residual. After a write whose reload fails, the footer shows the reload error; when a later poll reload succeeds (unsynced recovery or external change) the error is not cleared. Fix: clear that error on the recovering reload, without clearing unrelated errors. Severity low, effort small.
