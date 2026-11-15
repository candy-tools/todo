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
- [ ] R1-01: TUI edit overwrites concurrent agent changes made while the form was open
  app/tui/tui.go:1024-1026,1095-1105; app/tui/reload.go:66-68; internal/todo/store_write.go:170-186. Scenario: user opens e on a task; agent runs todo edit --set-plan; reload deferred while modal open; on save Store.Edit writes title, desc and plan, reverting the plan silently. CLI edit has the same issue over a shorter window. Fix: Fields carries only changed fields (pointers or mask); TUI sends only what differs from the form's initial values. Severity high, effort small, confirmed (traced). Breaking: internal API only.
- [ ] R1-02: Deleted or replaced DB file makes the open TUI write to the unlinked copy
  internal/todo/store.go:71-82; app/tui/reload.go:43-54. Scenario: rm .todo.db / git checkout / rename-swap by sync tool; SQLite keeps writing the unlinked inode, revision checks pass, edits lost on quit. Fix: record os.Stat in OpenStore; in reloadCheck and commit compare with os.SameFile and stop writing with an error when it differs. Severity medium-high, effort small, plausible. Not breaking.
- [ ] R1-03: import --force wipes the store with no backup
  internal/todo/import.go:84-103,167; app/cmd/import.go:22. Scenario: todo import --force other.md on the real DB deletes every item permanently; migrate.go:124 backup() exists but is unused here. Fix: when the store has items, call s.backup() before s.Import (outside tx) and print the backup path. Severity medium, effort small, confirmed. Not breaking.
- [ ] R1-04: Hash comment inside a fenced code block is imported as a category
  internal/todo/markdown.go:91-104. Reproduced: under ## Work a fenced code block containing '# install deps' became a root category; following tasks and ### Sub moved under it silently. Fix: track triple-backtick and triple-tilde fences and treat inner lines as description or dropped prose, never headers/checkboxes; add test. Severity medium, effort small, confirmed. Not breaking.
- [ ] R1-05: Store does not enforce tree invariants (category under task, task without category)
  internal/todo/store_write.go:133 AddCategory, 141 AddTask, 192/230 SetStatuses/CascadeStatus, 271 Move. Scenario: AddCategory(parent=taskID), AddTask(parentID=0), moving a category under a task, setting status on a category all succeed; rules only live in move.go and cmd/tui guards. Fix: check parent kind inside write (or whole-tree check in the tx), return ErrWrongParent. Severity medium, effort small, confirmed. Not breaking.
- [ ] R1-06: Titles are not validated at the store; newlines and control chars reach the TUI
  app/cmd/add.go:23-25; internal/todo/store_write.go; app/tui/tree.go:648-680. Scenario: todo add with a title containing a newline or ANSI/OSC escapes is accepted; row renders on two lines breaking cursor/scroll math, escape injection into terminal; TUI caps titles at 200 chars, CLI has no cap. Fix: one validTitle in the store (trimmed, non-empty, no control chars, length cap); strip control chars when rendering descriptions. Severity medium, effort small, CLI confirmed / render plausible. Breaking only for odd input.
- [ ] R1-07: [complex] TUI database writes block the UI thread
  app/tui/tui.go:1184-1209. Scenario: commit runs the write and full reload inside Update; while a CLI write holds the lock the TUI freezes up to the 5s busy timeout. Fix: run write+reload in a tea.Cmd returning committedMsg, ignore keys while pending. Severity medium-low, effort medium, plausible. Not breaking. Complex: changes the TUI update model (async commit) across many call sites.
- [ ] R1-08: [complex] Complete/reopen policy lives in app layer and CLI and TUI disagree
  app/tui/tui.go:802-825 vs app/cmd/status.go:68-77,109-120. Scenario: completing in TUI always completes subtasks; todo done does not unless --cascade. Reopen goes to working from CLI but to the column's first state from TUI. Fix: Store.Complete and Store.Reopen in internal/todo with one policy, called by both. Severity low-medium, effort medium, confirmed. Breaking (CLI defaults change). Complex: needs a product decision on which policy wins.
- [ ] R1-09: Add placement duplicated and racy; closure-tree errors leak to users
  app/cmd/add.go:34-46; app/tui/tui.go:1113-1150; internal/todo/store_write.go:36-44. Scenario: both front ends compute afterID from an in-memory tree; if that sibling moved meanwhile, ErrInvalidAfterNode reaches the user, exit 1. Fix: Store.AppendTask(parentID) computing 'after last task' inside the tx; translate ErrInvalidAfterNode/ErrInvalidMove to domain ErrConflict. Severity low-medium, effort small, confirmed (traced). Not breaking.
- [ ] R1-10: TUI silently swallows reload errors
  app/tui/reload.go:46-52. Scenario: newer todo upgrades schema or file becomes unreadable; view silently stops updating, only a later 'save failed' hints. Fix: return reloadFailedMsg and show once in m.err. Severity low, effort small, confirmed. Not breaking.
- [ ] R1-11: When write and reload both fail, unsaved changes stay on screen as if saved
  app/tui/tui.go:1193-1200. Scenario: in-memory tree mutated first (status, Remove); on write+reload failure the view keeps showing it. Fix: mutate a copy, or keep the previous tree and restore it when reload fails. Severity low, effort small, confirmed. Not breaking.
- [ ] R1-12: Unclosed todo:guide comment lets example lines import as tasks
  internal/todo/guide.go:27-34. Scenario: a todo:guide comment opener without a closing --> skips only the marker line, so the guide's checkbox and heading examples import as real items. Fix: skip to EOF and warn, or ignore the guide marker when unclosed. Severity low, effort small, confirmed (traced). Not breaking.
- [ ] R1-13: A tab counts as one space of indentation in the parser
  internal/todo/markdown.go:174-176. Scenario: a tab-indented '- [ ] a' followed by a two-space-indented '- [ ] b' nests b under a. Fix: expand tabs to 4 columns before measuring indent. Severity low, effort small, confirmed (traced). Not breaking.
- [ ] R1-14: Dead FindByPath and outdated comment in path.go
  internal/todo/path.go:7-37. FindByPath only used by tests; comment on Path says reload restores selection by title path, but it restores by ID. Fix: delete FindByPath and fix comment. Severity low, effort small, confirmed. Not breaking.
- [ ] R1-15: Duplicated helpers in internal/todo
  model.go:279-291 vs move.go:284-296 (insert before first category); migrate.go:187-199 vs store_write.go:193-221 (group by status). Fix: single insertBeforeCategories helper and single writeStatuses(tx, map). Severity low, effort small, confirmed. Not breaking.
- [ ] R1-16: CLI write commands open the database twice
  app/cmd/store.go:92-122. openForRead opens, migrates, closes, then withStore reopens for the write: double open cost and wider stale-read window (feeds R1-01, R1-09). Fix: open once, use the same store for Load and the write. Severity low, effort small, confirmed. Not breaking.
- [ ] R1-17: [complex] tui.go is very large and mode state is scattered
  app/tui/tui.go (1389 lines), model 64-122. Mode lives in five places (mode, searching, tree.grabbed, split, sideFocus); valid combinations implicit. Fix: split file by concern (commit, status, form, move) and fold searching into mode. Severity low, effort medium. Not breaking. Complex: wide refactor of the TUI model.
