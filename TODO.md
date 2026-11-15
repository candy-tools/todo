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
- [ ] R1-07: [complex] TUI database writes block the UI thread
  app/tui/tui.go:1184-1209. Scenario: commit runs the write and full reload inside Update; while a CLI write holds the lock the TUI freezes up to the 5s busy timeout. Fix: run write+reload in a tea.Cmd returning committedMsg, ignore keys while pending. Severity medium-low, effort medium, plausible. Not breaking. Complex: changes the TUI update model (async commit) across many call sites.
- [ ] R1-08: [complex] Complete/reopen policy lives in app layer and CLI and TUI disagree
  app/tui/tui.go:802-825 vs app/cmd/status.go:68-77,109-120. Scenario: completing in TUI always completes subtasks; todo done does not unless --cascade. Reopen goes to working from CLI but to the column's first state from TUI. Fix: Store.Complete and Store.Reopen in internal/todo with one policy, called by both. Severity low-medium, effort medium, confirmed. Breaking (CLI defaults change). Complex: needs a product decision on which policy wins.
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
- [ ] R1-17: [complex] tui.go is very large and mode state is scattered
  app/tui/tui.go (1389 lines), model 64-122. Mode lives in five places (mode, searching, tree.grabbed, split, sideFocus); valid combinations implicit. Fix: split file by concern (commit, status, form, move) and fold searching into mode. Severity low, effort medium. Not breaking. Complex: wide refactor of the TUI model.

# auto-improve round 2

- [ ] R2-01: TUI could offer to reopen the store after ErrFileReplaced
  Noticed while fixing R1-02 (64420ad). After the DB file is replaced, the TUI shows the error every poll but keeps the old content; the user must restart. Fix: on ErrFileReplaced offer a key (or auto) to reopen the store at the same path and reload. Severity low, effort small-medium, from fixer report.
- [ ] R2-02: Control chars still reach output via legacy titles, CLI list descriptions, CRLF descriptions
  Noticed while fixing R1-06 (c473ccb). (a) Titles already stored with control chars are drawn raw in TUI rows; (b) CLI list prints descriptions unsanitized (escape injection into terminal); (c) imported descriptions from CRLF files keep their trailing \r. Fix: sanitize at render time in TUI rows and CLI list; trim \r from description lines on import. Severity low, effort small, from fixer report.
- [ ] R2-03: Category adds still compute placement from the in-memory tree; AddTask afterID only used by tests
  Noticed while fixing R1-09 (4ae903c). CLI add-category and TUI c compute afterID from the in-memory tree, so a concurrent move/delete now fails with ErrConflict (exit 6) instead of succeeding. Fix: Store.AppendCategory placing inside the tx; consider dropping/privatizing AddTask's explicit afterID if only tests use it. Severity low, effort small, from fixer report.
