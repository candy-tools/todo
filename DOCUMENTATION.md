# todo — documentation

The full usage guide for [todo](README.md) — how the tree works, every key, the
search filter, and how the list is stored. See the [README](README.md) for
install and development instructions.

## Usage

```sh
todo                 # opens .todo.json in the current directory
todo path/to/my.json # opens a specific list file
todo import file.md  # imports a markdown file into the list
```

With no argument it defaults to `.todo.json`. If the file doesn't exist yet it
is created on the first write.

**First run with an existing `TODO.md`.** When no path is given (so todo uses
the default `.todo.json`), `.todo.json` doesn't exist and a `TODO.md` is present in
the current directory, todo imports it automatically, once, and prints a summary
(plus a warning for anything it couldn't keep) on stderr, except under `--json`
or `--flat`, where stderr is kept for JSON errors. Naming a file with `-f` or a
positional path disables this automatic import. The markdown file is left
untouched and can be deleted afterwards; from then on the JSON file is the source
of truth. Passing a `.md` path where a list file is expected is a usage error
that points you at `todo import`.

**Importing a markdown file.** `todo import file.md` reads a markdown file into
the list (`-f/--file` picks a file other than `.todo.json`). It refuses to
run when the list already has items; pass `--force` to replace them. With
`--force` it first copies the old list to `<file>.bak` (or the first free
`<file>.bak.1`, `<file>.bak.2`, …) and names it. The
import understands headers as categories, `- [ ]` / `- [/]` / `- [>]` / `- [x]`
checkbox lines as tasks (indentation nests subtasks) and indented text under a
task as its description. Tasks outside any header are gathered under an
`Uncategorized` category, and text it can't map (such as prose between items)
is reported as a warning and dropped.

Work is organized in **sprints** (see Sprints below). There are two workspaces —
the **Backlog** and the **Sprints** workspace, which has a board per sprint (see
Workspaces below). The Backlog and each
sprint board show a collapsible **category/task tree**. Tasks that carry a
description are marked with a small `≡` glyph; press `enter` (or `e`) on any item
to open its dialog, which shows the item's column, state and subtask progress and
lets you edit its title, reference, description and plan in place (`esc` cancels,
`enter`/Save applies).

Like a file explorer listing folders before files, a category shows its
subcategories first and its own tasks below them — in the tree, in the split
view's task list and in `todo list` alike.

An example screen, as the tree shows it:

```text
▾ Work (2/5)
  ▾ Backend (0/1)
      ○ Migrate the database open
  ▾ ○ Ship v1.0 release open (1/2)
      ● Write changelog
      ○ Cut the git tag
    ● Fix login bug done
▾ Personal (0/2)
    ○ Renew passport open
    ⏸ Book dentist deferred
+ new category
```

A task's glyph shows its state: `○` open, `⏸` deferred, `⊘` won't do, `◌` new,
`◉` ready, `◐` working, `◑` review, `◒` followup, `●` done. `▾` / `▸` mark an expanded /
collapsed item, and a category shows how many of its tasks are done. A top-level
task's row also names its state and, when it has subtasks, how many are done.

## Sprints

A sprint is a named batch of work with an optional goal. Work flows through
it: you **plan** by pulling tasks from the Backlog into a sprint (they become
`new`), **refine** them to `ready`, **execute** them (`working`, `review`,
`followup`) and **close** the sprint once every task is `done`. A sprint is
either `open` or `done` (closed); there are no dates.

The TUI shows the work in two workspaces (see Workspaces below):

- **Backlog** (`b`) — the category tree of the tasks that are in no sprint.
  Every category is shown, even an empty one, and categories are managed here.
  `f` filters by backlog state.
- **Sprints** (`s`) — a sidebar with one row per sprint beside the shown
  sprint's board, which lists the sprint's tasks in three sections: **Next**,
  **In Progress** and **Done**.

A task's place comes from its **sprint** and its **state**. A task is either in
the Backlog or in exactly one sprint, and every write enforces that:

| Where | Column | States |
|-------|--------|--------|
| Backlog (no sprint) | Backlog | `open`, `deferred`, `wontdo` (shown as "won't do") |
| A sprint | Next | `new`, `ready` |
| A sprint | In Progress | `working`, `review`, `followup` |
| A sprint | Done | `done` |

A sprint state on a task with no sprint, or a backlog state on a task in a
sprint, is refused. Setting a state never moves a task in or out of a sprint:
only assigning does (`a` / `o` in the TUI, `todo assign` / `todo unassign` on
the command line).

Sprint rules:

- **Assigning** a backlog task to a sprint makes it `new`. A task assigned from
  one sprint to another keeps its state. Unassigning returns it to the Backlog
  as `open`. Its subtasks keep their open/done state and travel with it.
- **Closing** is refused while any task of the sprint (subtasks included) is
  not done: `can't close: N tasks not done`. Send the unfinished tasks to
  another open sprint or back to the Backlog first. A closed sprint can be
  reopened.
- **Deleting** a sprint removes it together with all its tasks and their
  subtasks; nothing goes back to the Backlog. Only a closed sprint can be
  deleted, so a sprint is only ever deleted once its work is finished: an open
  one is refused with `the sprint is open; close it before deleting it`.
- **A closed sprint is frozen.** Assigning to it, adding a task to it, moving a
  task out of it and setting one of its tasks to a state other than done are
  refused with `reopen the sprint first`. Editing a task's text, deleting a
  task, removing its done tasks and moving tasks between categories still work.
- **References.** Every sprint gets a reference when it is created,
  `<PREFIX>-<n>` (`TODO-001`, `TODO-002`, …), shown before its name on every
  screen. It never changes, and its number is never reused. Names may repeat, so
  the reference is how you name a sprint: `--sprint TODO-002`. The prefix is the
  sprint reference prefix set in the settings (`todo settings --sprint-prefix`,
  `S` in the TUI). Without one it is the name of the folder that holds the list
  file, upper-cased, with only letters and digits, no leading digits, and at most
  10 characters (`todo` gives `TODO`, `candy-tools` gives `CANDYTOOLS`, and
  `SPRINT` is used when nothing is left). A sprint reference never matches a task
  reference. Setting the sprint prefix to the list's or a category's prefix is
  refused, and so is the reverse. When the folder name is also a task prefix,
  sprints add an `S` to it (`TODOS`). A task reference that equals a sprint's is
  refused. The sprints of a list written before sprints had references are
  numbered in creation order when the list is read, and keep those numbers from
  its next write on.
- **Nesting.** Indenting a task under another task is allowed only when both
  are in the same sprint, or both in the Backlog.

Other rules, as before:

- **Categories** group tasks on every screen, but they can only be added,
  renamed, moved or deleted in the **Backlog** (and the unified view, `u`).
- **Subtasks** go one level deep: a subtask cannot have subtasks of its own. A
  subtask is only ever `open` or `done`, has no board state, and is shown in its
  parent's column. A task in any state can hold subtasks.
- **New tasks** take the first state of where they are added: `open` in the
  Backlog, and on a sprint board the first state of the section the cursor is
  in (Next `new`, In Progress `working`, Done `done`), in that sprint. A new subtask is open.
- **Plan.** Each task has an optional plan link — a path or reference to where
  its plan lives — edited in the task's dialog. A `ready` task with no plan is
  flagged **`⚠ no plan`** on its row in the tree and in its dialog.
- **Reference.** A task can carry an optional reference such as `APP-001`,
  shown before its title as `[APP-001] Ship it`. It is free-form text (one
  line, up to 50 characters), but no two tasks in a list may share one —
  compared ignoring case, so `app-001` and `APP-001` clash. Subtasks have no
  reference, so indenting a task under another task drops its reference (its
  number is not reused, and lifting the subtask back out does not restore it);
  `todo move --json` reports it as `droppedRef`. Search matches the
  reference too.
- **Reference prefix.** A task created without a reference gets one generated
  from a prefix, `<PREFIX>-<n>` with `n` counted per prefix and padded to three
  digits (`WORK-001`, `WORK-002`, ..., `WORK-999`, `WORK-1000`). The prefix is the
  one of the nearest category above the task that has one, else the list's;
  with none, the task gets no reference. The sprint a task is created in plays
  no part. A prefix is a letter and up to 9 letters or digits, stored in upper
  case. Set the list's with `todo settings --ref-prefix`, a category's with
  `todo add-category --prefix` or `todo edit --set-prefix` (in the TUI: `S` for
  the list's, the category dialog for a category's); an empty value clears it.
  A task prefix may not be the sprint reference prefix (see Sprint rules). A
  reference typed for a new task where a prefix applies is refused, since it is
  generated: create the task, then change it with `edit --set-ref`. Subtasks never
  get a reference, an import assigns none, and existing references are never
  changed or backfilled when a prefix is set. Numbering continues after the
  highest existing `<PREFIX>-<n>` (hand-typed ones included), and numbers are
  never reused.

Moving a task between columns and sprints:

- On a sprint board, `a` / `A` move it to the next / previous column; it lands
  in that column's first state. `A` on a Next task sends it back to the Backlog
  (it becomes `open`) and the footer says `moved to Backlog`.
- On a backlog task, `a` opens a picker of the open sprints and assigns the
  task to the one you choose (it becomes `new`). `A` does nothing there.
- `o` opens a "send to" picker on a sprint task: the Backlog and the other open
  sprints. A sprint keeps the task's state; the Backlog makes it `open`. It is
  how unfinished work leaves a sprint before you close it. The footer says
  where the task went (`moved to Sprint 14 — working`). It does nothing on a
  subtask.
- `t` opens a dialog listing the states that apply to the task (the backlog
  states for a backlog task, the sprint states for a sprint task), grouped by
  column; picking one sets it (and moves the task when the state belongs to
  another column). It is the only key that sets a state — completing and
  reopening included. For a subtask it offers `open` and `done`.
- Moving a task out of Done with `A` sets it `working`. If you completed it in
  this session, its subtasks get their previous states back; otherwise they keep
  their current ones.
- `m` grabs a task to reorder it (see Keys); grab mode steps over tasks hidden in
  the view (on a sprint board, those of the other sections), and moves a task at
  most one category per key press.
- On a closed sprint's board every key that would change a task's sprint or
  state, `o` included, answers `reopen the sprint first`.

## Workspaces

The header's second line is a switcher, `b Backlog   s Sprints`, with the
workspace you are in bracketed (`[s Sprints]`), followed by a breadcrumb that
says where you are:

```text
 b Backlog   [s Sprints]   Sprints › Sprint 14 › Next
```

The breadcrumb reads `Backlog › Work › Reviews · N tasks waiting` (the category
under the cursor), `Sprints › Sprint 14 › Next` (a closed sprint is marked `✓`),
or `All states · N tasks` in the unified view. The footer shows a few keys of
where you are, most important first, and drops the last ones whole on a narrow
terminal; everything else is in the `?` dialog:

- the Backlog: `?` help, `tab` pane (in the split), `a` assign, `v` split (or
  `v` single tree in the split), `s` sprints, `/` search, `q` quit;
- a sprint board: `?` help, `tab` sprints, `[ ] 1-3` section, `a/A` move task
  (not on a closed sprint), `/` search, `q` quit;
- the Sprints sidebar: `?` help, `n` new, `e` edit, `c` close/reopen, `d`
  delete, `tab` board, `q` quit;
- the unified view: `?` help, `/` search, `q` quit, and `tab` pane when split.

### Backlog (`b`)

The category tree of the tasks in no sprint, in a pane titled **Backlog**. `v`
splits it into a categories sidebar and the task list (see Views); `tab` then
moves the keys between the two panes, and `esc` in the task list goes back to
the categories. `a` on a task assigns it to a sprint (a picker of the open
sprints; on a category it says `select a task to assign it to a sprint`).

### Sprints (`s`)

A sidebar of sprints on the left, the selected sprint's board on the right.
Each sidebar row shows the sprint's reference and name, its task counts (Next · In Progress ·
Done) and its goal (`—` without one); names and goals are cut with `…`, never
wrapped. The sidebar has two sections, each opened by a coloured rule with its
title and how many sprints it holds, like the board's: `━━ Open 2 ━━━━` (cyan,
the board's Next colour) over the open sprints in creation order, then
`━━ Done 2 ━━━━` (green) over the closed ones, newest first. An empty section
shows as its rule alone.

```text
━━ Open 2 ━━━━━━━━━━
❯ ● TODO-003 Sprint 15   3·1·0
    Ship the importer
  ● TODO-004 Sprint 16   0·0·0
    —
━━ Done 2 ━━━━━━━━━━
  ✓ TODO-002 Sprint 14   0·0·7
    Refs and prefixes
  ✓ TODO-001 Sprint 13   0·0·5
    Board sections
```

The rules are never selected: the arrows move the selection from sprint to
sprint, across both sections, and show that sprint's board at once — a closed
sprint's too, frozen (see Sprint rules). `[` / `]` jump to the first sprint of
the previous / next section that holds sprints, and `1` / `2` to the first
open / closed one (`nothing in Done` when it is empty), scrolling the section
as close to the top as the sidebar allows. With every sprint closed the
workspace opens on the newest. todo reopens on the last sprint whose board you
saw.

The keys start on the sidebar: `n` creates a sprint (a form with Name and
Goal), `e` edits its name and goal, `c` closes an open sprint or reopens a
closed one, and `d` deletes a closed sprint and all its tasks after a
confirmation that says how many tasks go with it. A refused close or delete
says why in the footer (`can't close: 3 tasks not done`; `d` on an open sprint
is refused at once). `enter` or `tab` moves the keys onto the
board, and `tab` or `esc` on the board brings them back to the sidebar.

Above the board's tree is a **head** of three lines: the sprint's goal, a
progress bar of the done tasks with whether the sprint can be closed (`close
blocked: 3 not done`, `ready to close`, `no tasks yet`, or `closed` for a done
sprint), and a rule that sets the head apart from the tasks. On a short
terminal the head yields rows to the tree so the cursor stays visible: the goal
first, then the rule.

The board lists all of the sprint's tasks in one tree, in three sections, each
opened by a coloured rule with its title and task count: `━━ Next 3 ━━━━`,
`━━ In Progress 1 ━━━━` and `━━ Done 2 ━━━━`. An empty section shows as its rule
alone. Inside a section the tasks are grouped under their category paths, as
in the Backlog's split task list (`── Work › Backend`); those headers are not
selectable and do not fold, so `z` on the board only says so. `[` / `]` jump to
the previous / next section that holds tasks, and `1`–`3` jump to Next / In
Progress / Done (`nothing in Next` when it is empty), scrolling the section as
close to the top as the board allows. When `a`, `A` or `t` moves a task to
another section, the cursor stays where it was, on the task that took its
place, and the footer says where the task went (`moved to Done — done`). Each
sprint board remembers its own folds and selection.

### Moving around

| Key | Goes to |
|-----|---------|
| `b` | The Backlog |
| `s` | The Sprints workspace, on the last sprint you viewed, else the first open one |
| `k` | The next open sprint, wrapping around; from the Backlog, the last sprint you viewed while it is open, else the first open one |
| `K` | A dialog listing every sprint, closed ones marked `✓`; `enter` opens its board |
| `q` | Quit, from every screen — a dialog asks first, on Yes: `enter` quits, `esc` stays (`ctrl+c` quits at once) |
| `esc` | Clear the search, then the state filter; then, on a board, back to the sprint sidebar, and in a split's task list, back to the categories; else ask to quit, like `q` |
| `tab` / `shift+tab` | Switch pane: the Backlog's split panes, the Sprints sidebar and board |
| `[` / `]`, `1`–`3` | On a sprint board, switch between Next / In Progress / Done |

`k` and `K` with no sprint to go to show a hint instead (`no open sprints —
press s, then n to create one`). todo saves the workspace and sprint in the
list file and reopens there; a saved view that names no sprint, or one that no
longer exists, opens the Sprints workspace on the first open sprint.

## Views

Two view toggles change how the tasks are laid out. They are independent and
can be combined. Both are saved in the list file (see Storage), so todo opens
on the view you left, for everyone who opens that list.

**Unified view (`u`).** On the Backlog or a sprint board, shows every task,
whatever its state and sprint, in one tree; each task shows its sprint's
reference and name next to its state, and the breadcrumb becomes `All states · N tasks`. Press
`u` again to return to the screen you came from; `b`, `s`, `k` and `K`
leave it and go where they say. In this view:

- categories can be added, renamed, moved and deleted, as in Backlog, since
  every category is shown;
- `n` adds a backlog task (`open`) and `N` a subtask of the selected task;
- each task's keys follow its own place: on a backlog task `a` opens the sprint
  picker and `t` offers the backlog states; on a sprint task `a`, `A`, `t` and
  `o` work as on its sprint board;
- `f` offers every state, and its selection is the view's own — the Backlog's
  and the boards' state filters don't apply here and are untouched by it;
- `D` is refused with a hint (`open a sprint to remove its done tasks`);
- `[`, `]` and `1`–`3` only remind you to press `u`.

**Split by category (`v`).** On the Backlog, shows the categories as a tree in
a left sidebar (**Categories**) and the tasks of the selected one in a list on
the right, titled with the category's path (**Backlog** on All). Press `tab` to
move the keys between the two panes, and `esc` in the list to go back to the
categories; `v` again goes back to the single tree, on the item that had
focus. The Sprints
workspace has its own sidebar, so `v` there only says `v splits the Backlog`.

```text
╭─ Categories ─────────╮╭─ Work ──────────────────────────╮
│  All                 ││  ── Backend                     │
│❯ ▾ Work (2/5)        ││❯ ○ Migrate the database open    │
│      Backend (0/1)   ││  ── Work                        │
│    Personal (0/2)    ││  ○ Ship v1.0 release open       │
│  + new category      ││                                 │
╰──────────────────────╯╰─────────────────────────────────╯
```

- The sidebar starts on **All**, which lists every task. Walking down the tree
  narrows the list: as in the tree, each subcategory's tasks come first under
  a `── Backend` heading, then the category's own tasks under its own heading.
  Categories fold in the sidebar only (`z` in the list says so).
- The list follows the current screen (or every task in the unified view), and a search or
  state filter narrows both panes: the sidebar keeps only categories with
  matches. When the selected category disappears (switching screen, filtering,
  deleting it) the sidebar falls back to All.
- Categories are edited in the sidebar (`enter`/`e`, `c`, `d`, `m`) under the
  usual rules; `n`/`N` there add a task to the selected category. In the list,
  the task keys work as usual; `n` in an empty list adds to the selected
  category.

## Scripting / for agents

Every interactive action is also a non-interactive subcommand, so an agent (or a
script) can edit the list without opening the TUI. All commands take
`-f/--file` (default `.todo.json`).

### Quick start for agents

Do not edit the list file by hand and do not walk the nested tree: ask todo.

```sh
todo next --json                       # what to work on, per open sprint
todo show --id APP-001 --json          # one item (--number, --id, --title; --id takes a reference)
todo list --flat [--kind task] [--status working,review] [--sprint "…"] [--column …] [--filter …]
```

`todo next [--sprint <ref|id|name>] [--json]` prints
`{"sprints":[{id, ref, name, goal, status, counts, inProgress, next}]}`: for
each open sprint (or the one named, open or closed) its reference `ref`, its
`status` (`open` or `done`; the text header starts with the reference and name
and adds `(closed)` for a done sprint), its `counts` (`next`,
`progress`, `done`), the tasks `inProgress`, and the tasks `next` (`ready`
before `new`), each a record. `inProgress` and `next` hold top-level tasks only;
subtasks show through `doneCount`/`totalCount`. `todo show` prints one record
with `subtasks`. `todo list --flat` prints a JSON array of records in list order
(it implies `--json`). With `--column`, `--sprint` or `--backlog` it returns
tasks only (subtasks included); `--filter` also returns the categories on the
path to each match. `--kind task|category` and
`--status <states, comma-separated>` need `--flat` (a usage error without it);
`--status` takes state names (`progress` is accepted for `working`).

**The record.** Every item printed as JSON here has this shape, and so does
every node of `todo list --json`, which adds its `children` (camelCase; fields
marked optional are left out when empty):

- `number`: the `#` of `todo list`; `id`: the 7-character id; `kind`: `task` or
  `category`.
- `ref` (optional, tasks): the reference, e.g. `APP-001`; `title` never
  includes it.
- `prefix` (optional, categories): the category's reference prefix.
- `title`: the raw title.
- `status` (tasks): the state name; `column` (tasks): `backlog`, `next`,
  `progress` or `done`.
- `sprint` (optional, tasks): the sprint's id (a subtask shows its parent's);
  `sprintRef` and `sprintName` (optional): its reference and name.
- `path`: the titles of the ancestors, root first, excluding the item itself
  (`[]` at the top).
- `parentId`, `parentTitle` (optional): the parent item.
- `level` (optional, categories): the category depth.
- `description`, `plan` (optional, tasks).
- `doneCount`, `totalCount`: done and total tasks below the item
  (subtasks for a task).
- `subtasks` (optional): only in `show` and in the results of changes; one
  `{number, id, title, status}` per direct child task.

**`--json` on changes.** Add `--json` to any command that changes an item or a
sprint (`import` has none) and it prints the result instead of the text line:
`add`, `add-category`, `status`, `done`, `progress`, `defer`, `reopen`, `edit`,
`move`, `assign` and `unassign` print the record of the item (with
`subtasks`), and `move` adds `droppedRef` with the reference a task indented
under another task lost (left out when none was dropped); `rm` prints
`{"removed": <record>}`; `prune` prints `{"removed": <count>}`; `sprint add`,
`sprint edit`, `sprint close` and `sprint reopen` print the sprint object
(`{id, ref, name, goal, status, counts}`); `sprint rm` prints
`{"removed": <sprint>}`.

**Errors.** Under `--json` or `--flat`, a failing command prints one line on
stderr, `{"error":{"code":"not_found","exit":3,"message":"…"}}`, and exits with
that code. Without those flags the message stays plain text. A successful
command can still write plain-text notes to stderr, such as `done`'s
open-subtasks hint; parse stderr as JSON only when the exit code is non-zero.

| exit | `code`       | meaning |
|------|--------------|---------|
| 1    | `error`      | any other failure |
| 2    | `usage`      | bad flags or arguments |
| 3    | `not_found`  | no such item or sprint |
| 4    | `ambiguous`  | more than one item matches |
| 5    | `wrong_kind` | the action does not fit the item's kind |
| 6    | `conflict`   | the change conflicts with the list's state |

### The nested view and other reading commands

Read the list — with numbers, ids, and JSON for the read-then-act loop:

```sh
todo list                 # aligned table: # · id · state · item
todo list --json          # the nested view: a tree of records, each with children
todo list --filter build  # only matching items and their path
todo list --column next   # only tasks in one column: backlog|next|progress|done
todo list --sprint TODO-013     # only that sprint's tasks (by reference, id or name)
todo list --backlog       # only tasks in no sprint (same as --column backlog)
todo sprint list [--all] [--json]  # open sprints with their task counts
```

The `state` column of `todo list` shows the task's state name (`open`, `ready`,
`working`, …), and `--` for a category. `todo list --json` is a tree of records
(see **The record** above), each with its `children`. In JSON, `status` is the
state name, `column` is the task's column (`backlog`, `next`, `progress` or
`done`; a subtask reports its parent's column), `plan` is the plan link and
`ref` the task's reference (left out when it has none; `title` never includes
it). `sprint` is the id of the sprint the task is in (a subtask shows its
parent's; left out for a backlog task). `--column` without `--sprint` covers
that column across every sprint; `--column backlog` with `--sprint` is a usage
error. `todo sprint list --json` is an array of
`{id, name, goal, prefix (optional), status, counts}`, where `status` is `open`
or `done` and `counts` has the `next`, `progress` and `done` task counts.
`--all` includes closed sprints.

**Breaking change:** JSON `status` now says `"working"` where it used to say
`"progress"`. On input, `progress` is still accepted as an alias of `working`.

Address an existing item three ways — exactly one, mutually exclusive:

```sh
--number N   # the # column from `todo list`
--id     K   # the item's 7-character id (the id column from `todo list`), or a task's reference (e.g. APP-001, any case)
--title  T   # the item's exact title (errors if more than one matches)
```

Common actions:

```sh
todo add --title "Cut the tag" --parent-title "Ship v1.0" [--ref APP-001] [--desc "…"] [--plan docs/plan.md] [--sprint "Sprint 13"] [--status open|working|deferred|done|new|ready|review|followup|wontdo]
todo add-category --title "Backend" [--parent-title "Work"] [--prefix BACK]
todo status   --number 4 --to ready
todo done     --title "Cut the tag" [--cascade]
todo progress --number 4   # shortcut: status --to working
todo defer    --id k3f9qw2 # shortcut: status --to deferred
todo reopen   --title "Cut the tag" [--cascade]
todo edit     --number 4 --set-title "…" --set-desc "…" --set-plan "docs/plan.md" --set-ref APP-002
todo edit     --title "Backend" --set-prefix BACK   # a category's reference prefix
todo settings [--ref-prefix TODO] [--sprint-prefix SP] [--json]   # show or set the task and sprint reference prefixes
todo move     --title "Cut the tag" --parent-title "Backend" [--first | --after-title "…"]
todo move     --title "Backend" --root
todo rm       --number 4
todo prune --sprint "Sprint 13"  # drop that sprint's fully-completed tasks
todo prune --all                 # drop them in every sprint and the backlog

todo sprint add    --name "Sprint 13" [--goal "ship sprint support"]   # prints the new reference, e.g. TODO-013
todo sprint edit   --sprint TODO-013 [--set-name "…"] [--set-goal "…"]
todo sprint close  --sprint TODO-013
todo sprint reopen --sprint TODO-013
todo sprint rm     --sprint TODO-013             # a closed sprint, with all its tasks
todo assign        --sprint TODO-013 --number 4
todo unassign      --number 4
```

`status --to <state>` sets any state: `open`, `working`, `deferred`, `done`,
`new`, `ready`, `review`, `followup` or `wontdo` (`progress` is accepted for `working`).
`add --status` defaults to `open`, or `new` with `--sprint`; a sprint state
without `--sprint`, or `--sprint` with a backlog state, is a usage error (exit
code `2`). `edit --set-plan ""` clears a task's plan,
and `edit --set-ref ""` its reference. A reference another task already holds,
or one given to a subtask, is refused as a usage error (exit code `2`), and so
is `add --ref` where a reference prefix applies (see Reference prefix): the
reference is generated and `add` prints it before the title. `edit --set-prefix`
on a task is refused with exit code `5`. `todo settings` prints
`ref-prefix: TODO` (or `ref-prefix: (none)`) and `sprint-prefix: SP` (or, with
none set, the one in use from the folder name: `sprint-prefix: TODO (folder
name)`). `--json` prints
`{"refPrefix":"TODO","sprintPrefix":"SP","sprintRefPrefix":"SP"}`, where
`sprintPrefix` is the one set (`""` for none) and `sprintRefPrefix` the one new
sprints use. `--sprint-prefix ""` goes back to the folder name. An invalid
prefix, or a sprint prefix that is also a task prefix, is a usage error.
`list --json` shows a category's `prefix`, left out when empty, and
`sprint list --json` a sprint's `ref`.
`move` places an item, with its subtree, under the parent named by
`--parent-number`, `--parent-id` or `--parent-title` (or at the top level with
`--root`, for a category): last among its kind there by default, first with
`--first`, or right after the sibling named by `--after-number`, `--after-id` or
`--after-title`, which must sit directly under that parent (exit code `2`
otherwise). Subcategories always stay ahead of tasks. The move rules match the
TUI's: a task never goes to the top level and a category never under a task
(exit code `5`); a task with subtasks cannot become a subtask, and one with a
reference loses it (`move` prints `dropped reference <ref>`, or under `--json`
reports it as `droppedRef`); a moved subtask becomes a task and takes its state
from its old parent.
`reopen` takes a done task back to `working` (a subtask to `open`); a task that
is not done keeps its state and nothing is written (with `--cascade` its
subtasks are reopened). `--cascade` on `done` / `reopen` applies the change to
the task's subtasks too.

**Sprints.** `--sprint` takes a sprint's reference (`TODO-003`), its id or its
name, ignoring case, tried in that order. Names may repeat: a name that several
sprints share exits `4` (ambiguous) and lists their references to pick from.
`todo sprint add` prints the new sprint's reference, `sprint list` shows each
sprint's reference first, and the text output of the sprint commands, `next` and
`show` names a sprint as `TODO-003 Sprint 13`. `assign` puts a task in a sprint: a backlog task becomes `new`, a
task in another sprint moves there and keeps its state; `unassign` returns it
to the backlog as `open`. `status`, `done`, `progress`, `defer` and `reopen`
never change a task's sprint, so `status --to working` on a backlog task fails
with `task is not in a sprint (use todo assign)`, and a backlog state on a
sprint task is refused too. `sprint close` is refused while tasks are not done
(`cannot close "TODO-001 S1": 1 task not done`); `sprint rm` deletes a closed sprint
together with all its tasks and is refused while the sprint is open;
changing the state of a task in a closed sprint fails with `reopen the sprint
first`. All of these exit with code `2`; an unknown sprint exits `3`.

The two prunes differ on subtasks. `prune --sprint` (like `D` on a sprint
board) removes only the sprint's top-level tasks that are done with every
subtask done; a done subtask under an unfinished task stays. `prune --all`
also removes those done subtasks under unfinished tasks, in every sprint and
the backlog.

**Breaking change:** `prune` now needs `--sprint <ref|id|name>` or `--all`; a bare
`todo prune` is a usage error (exit code `2`) rather than emptying every
sprint's history. A new task in a sprint, and `status --to new` / `ready` /
`working` / `review` / `followup` / `done` on a backlog task, behave as above.

Breaking the board's rules — a subtask under a subtask, a board state on a
subtask, or an invalid title — exits with code `2`. Asking an item of the wrong
kind — a state on a category, a category under a task — exits with code `5`,
and the error names the item.

`--id` is stable: an item keeps its id for as long as it exists, so it is the
safest way to address an item across a sequence of edits. `--number` follows
the tree order (a category's subcategories before its tasks) and shifts after
any add, move or remove; `--title` is stable as long as it stays unique. Re-run
`todo list` when you rely on `--number`.

Exit codes: `0` ok, `2` usage, `3` not found, `4` ambiguous title, `5` wrong
kind, `6` conflict (the list changed while the command ran, or another todo
kept it locked for writing for more than 5 seconds; re-run it), `1` other
(including an invalid list file, and an internal error, which writes nothing).
Errors go to stderr.

## Keys

| Key | Action |
|-----|--------|
| `↑` / `↓` | Move the selection (`w`/`s` no longer navigate) |
| `PgUp` / `PgDn` | Move the selection by 10 rows |
| `←` / `→` | Collapse / expand (Left on a leaf jumps to the parent) |
| `space` | Fold / unfold the selected category |
| `z` | Fold every category at the selection's level (the selected category and its siblings; for a task, its category's) — or unfold them all when all are folded. Off while searching |
| `/` | Search / filter the list (see below) |
| `f` | Filter the view by state: tick states with `space`, `enter` applies, `esc` clears the filter |
| `b` | Go to the Backlog |
| `s` | Go to the Sprints workspace (sprint sidebar + board) |
| `k` | Last viewed sprint while it is open, else the first open one; on a sprint board, the next open sprint |
| `K` | Pick a sprint to go to |
| `q` | Quit, from every screen — a dialog asks first, on Yes: `enter` quits, `esc` stays (`ctrl+c` quits at once) |
| `tab` / `shift+tab` | Switch pane: Backlog split panes, Sprints sidebar / board |
| `[` / `]` | Previous / next section (sprint board); on the Sprints sidebar, the first sprint of the previous / next section |
| `1`–`3` | Jump to Next / In Progress / Done (sprint board); on the Sprints sidebar, `1` / `2` jump to the first Open / Done sprint |
| `u` | Unified view: every task, any sprint / back — see Views |
| `a` / `A` | Move the task to the next / previous column (it takes that column's first state) |
| `a` | On a backlog task: assign it to a sprint (picker of open sprints); on a Backlog category it says to select a task |
| `A` | On a Next task: send it back to the Backlog |
| `o` | Send the task to another open sprint or the Backlog (picker) |
| `t` | Open the state dialog to set the task's state, including done (`↑`/`↓`, `enter` sets, `esc` cancels) |
| `m` | Grab the task to reorder it (arrows move it, `enter`/`esc`/`m` drops it) |
| `y` | Copy the selected item to the clipboard — a task's title and full (multi-line) description as plain text |
| `enter` / `e` | Open / edit the selected item's dialog — its column/state/progress plus its editable title, reference, description & plan (a category's reference prefix) |
| `n` | New task as a sibling — at the end of the current level (on a category, a task inside it); on a sprint board, in that sprint |
| `N` | New task as a child — a subtask of the selected task (on a category, a task inside it) |
| `c` | Add a category (a subcategory when a category is selected) — Backlog and unified view only; on the Sprints sidebar, `c` closes / reopens the sprint |
| `d` | Delete the selected item (and its subtree) — asks to confirm |
| `D` | On a sprint board, remove that sprint's completed tasks (keeping any with unfinished sub-tasks) — asks to confirm |
| `v` | Backlog split view: categories sidebar + task list / back to the single tree — see Views |
| `?` | Show the shortcuts dialog (`↑`/`↓` scroll it on a short terminal; `?`, `esc`, `q` or `enter` closes it) |
| `S` | Settings: the task reference prefix (see Reference prefix) and the sprint reference prefix (see Sprint rules), from every screen; an empty value clears one, and an empty sprint prefix shows the folder-name one sprints fall back to |
| `esc` | Clear an active search, then the state filter; when there is neither, on a sprint board back to the sprint sidebar, in a split's task list back to the categories, else ask to quit, like `q` |

On the **Sprints sidebar**: `n` creates a sprint, `e` edits its name and goal,
`c` closes or reopens it, `d` deletes it, `[` / `]` and `1` / `2` jump between
the Open and Done sections, and `enter` or `tab` focuses its board. A sprint's
reference is given when it is created and cannot be edited.

Tasks always live under a category — there are no root-level tasks. In the
Backlog the tree always ends with a **`+ new category`** row: select it and
press `c` (or `enter`) to add a top-level category. On an existing category, `c` adds a
subcategory instead.

In the add/edit dialog: `tab` moves between fields (title, reference,
description, plan; a subtask has no reference),
`enter` saves (it inserts a newline while you're in the description box), `esc`
cancels. Where a reference prefix applies to a new task (see Reference prefix),
the add dialog shows its reference as `WORK-… (automatic)` instead of a field:
it is generated when the task is saved, and the edit dialog can change it
afterwards. The category dialog (`c`, and `e` on a category) has a **Reference
prefix** field after the title; it is stored in upper case, and emptying it
clears the prefix. The sprint dialog has the name and the goal only.

Grab mode (`m`) is refused while a search or state filter is active.

## Search

Press `/` to filter the current screen as you type — a grep over task/category
titles and task descriptions. The whole path to every match is kept, so a hit
deep in a subtask keeps its parent tasks and category visible (searching `cat`
still shows the tree down to a "catacombs" subtask), while non-matching branches
are hidden; the matched text is highlighted. `enter` keeps the filter and returns
you to navigating the results, `esc` clears it, and pressing `/` again refines
the current query.

## State filter

Press `f` to pick which states the current screen shows: tick them with `space`
(or `x`) and press `enter`. Only top-level tasks in a ticked state stay, with
their subtasks and categories. The Backlog, the sprint boards (one selection
for all of them) and the unified view each keep their own, the footer shows it, and it combines with
a search. `esc` clears it; ticking nothing and
pressing `enter` does too.

## Completing a parent

Setting a parent task to `done` (with `t`, or `a` into Done on a sprint board) completes it and
**all** of its subtasks at once. If you did it by accident, press `t` again: the
dialog opens on the task's previous state, so `enter` restores it and its
subtasks' states exactly — that undo memory is kept in RAM for the session, so
the file only ever records the current states. Without undo history the
dialog opens on `done`, and the subtasks keep their states when you reopen it.
The CLI's `todo done` completes only the task unless you pass `--cascade`, and
it has no undo. When subtasks are left open it says so on stderr (`2 subtasks
still open; --cascade completes them`); stdout is unchanged.

## Live reload

The open TUI notices changes made to the list file by another process — most
commonly `todo` CLI commands run by a script or an agent — and applies them
within about a second, so you can watch the list update live. Your selection and
which items are folded are kept across a reload. Changes made inside todo are
written immediately, so editing in the TUI while a script edits from outside
never fight each other.

If the list file is removed while the TUI shows it, the TUI keeps showing the
list and refuses to recreate the file: every save fails with `save failed: list
file was removed`. Started on a list file that does not exist yet, its first
save creates it.

## Storage

The list lives in a single JSON file, `.todo.json` by default, in the directory
you run todo from. It is plain text, so it diffs and merges in git. Every write
changes only the lines that changed.

```json
{
  "_comment": "This file is managed by the todo CLI. Do not edit it directly, …",
  "version": 8,
  "refs": {
    "prefix": "TODO",
    "sprintPrefix": "SP",
    "last": {
      "BACK": 3,
      "SP": 13
    }
  },
  "view": {
    "screen": "sprint",
    "sprint": "a1b2c3d",
    "split": true
  },
  "sprints": [
    {
      "id": "a1b2c3d",
      "ref": "SP-013",
      "name": "Sprint 13",
      "goal": "ship sprint support",
      "status": "open"
    }
  ],
  "categories": [
    {
      "id": "8v1wyaa",
      "title": "Main"
    },
    {
      "id": "s85fd4v",
      "title": "Backend",
      "prefix": "BACK",
      "parent": "8v1wyaa"
    }
  ],
  "tasks": [
    {
      "id": "4r2ft3y",
      "category": "s85fd4v",
      "sprint": "a1b2c3d",
      "ref": "APP-001",
      "title": "Hot reload",
      "status": "ready",
      "plan": "docs/plans/hot-reload.md",
      "description": "Poll the file and reload on change.",
      "subtasks": [
        {
          "id": "t14an2j",
          "title": "flock wrapper",
          "done": true
        }
      ]
    }
  ]
}
```

- `_comment` is a note for whoever opens the file instead of running todo,
  mostly coding agents: it says the file is managed by the todo CLI, should not
  be edited directly, points readers at `todo next --json`,
  `todo show --id <id|ref> --json` and `todo list --flat`, and says to change
  it with todo commands. Every change, from a CLI command or the TUI, writes
  todo's note first in a file that has none, and a new file starts with it. A
  file with a different note keeps it. Opening or reading a list never writes
  it. A file with a different note, or none, loads as usual.
- `version` is the format version (now `8`). A file with a higher version is
  refused: `list was written by a newer todo`. A version `3` to `7` file loads
  as it is and is written back as version `8` on the next change (opening it
  in the TUI alone does not), after which todo versions older than this one
  refuse it.
  **Upgrade:** version `3`, `4` and `5` files (from before sprints) have tasks
  in Next, In Progress or Done with no sprint, which version `6` and later
  forbid. On load, todo puts every such top-level
  task into one open sprint named `Imported`, keeping its state, so nothing
  changes column and nothing disappears. Backlog tasks stay out of any sprint,
  and a file with no such task gets no `Imported` sprint. A markdown import
  follows the same rule for its `[/]`, `[>]` and `[x]` tasks.
- `refs` holds the reference-prefix state and is left out when empty: `prefix`
  is the list's task prefix, `sprintPrefix` the sprint prefix set in the
  settings (left out for the folder name), and `last` is the last number handed
  out for each prefix, so a number is never reused after a task or sprint is
  deleted. `sprintPrefix` may not be `prefix` or a category's prefix.
- `view` is the screen and view the list opens on: `screen` is `backlog`
  (default), `sprints` or `sprint`, with `sprint` the id of the board shown for
  `screen: sprint`; `unified` is the unified view (`u`) and `split` the Backlog's
  split view (`v`). Each is left out when it is the default, and the whole
  section when all are. A saved `screen: sprints`, or a saved sprint that no longer
  exists, opens the Sprints workspace on the first open sprint.
- `sprints` is always written, possibly empty, in creation order (which is the
  `k` rotation order). A sprint has an `id`, a `ref` (its reference, unique
  among tasks and sprints, ignoring case), a `name` (several sprints may share
  one), a `goal` (optional) and a `status`, `open` or `done`. Version 7 sprints
  had an optional `prefix`. It is still read, then ignored and dropped on the
  next write. Their tasks keep the references they already have.
- `categories` have an `id`, a `title` and, for a nested one, the `parent`
  category's id, and an optional `prefix`, the reference prefix for tasks
  created in it. Array order is the order among siblings.
- `tasks` have an `id`, the `category` they sit under, a `title` and a `status`
  (`open`, `working`, `deferred`, `done`, `new`, `ready`, `review`, `followup`
  or `wontdo`). `sprint` (the id of the sprint the task is in), `ref`, `plan`,
  `description` and `subtasks` are optional. A task with a sprint has a sprint
  state and one without has a backlog state; a done sprint holds only done
  tasks. A file that breaks one of these rules is refused.
  `ref` is unique in the file, ignoring case. Array order is the order within
  a category.
- `subtasks` sit inside their task, one level deep. A subtask has an `id`, a
  `title` and `done` (left out when false), plus an optional `description` and
  `plan`.
- IDs are 7 characters from `0123456789abcdefghjkmnpqrstvwxyz`. Every category,
  task and subtask has its own. This is what `--id` takes (besides a
  reference).

**The lock file.** A write first takes a lock on `.todo.json.lock`, next to the
list. Two todo processes never write at the same time. A CLI write that waits
more than 5 seconds for the lock fails as a conflict (exit code `6`); the TUI
waits only 750 ms, so it never freezes, and shows the save as failed. Reading
needs no lock. todo only creates the lock file when it writes.

**Hand edits.** Change the list with todo, not by editing `.todo.json`; the
file's `_comment` says so to agents and scripts. You can still edit it by hand,
for example to repair it. Two things to know:

- An editor does not take the lock, and saving writes back its whole buffer:
  the file as it was when the editor loaded it, plus your edits. Every todo
  change made since then (in the TUI, by a CLI command, by an agent) is
  overwritten, not only one made at the same moment. Reload the file in the
  editor right before you change it, or use the CLI while todo runs.
- A mistake, such as broken JSON or a task whose category does not exist, stops
  every todo write until you fix it. The error names the problem. todo never
  repairs or overwrites such a file.

**Git.** Commit `.todo.json` and ignore `.todo.json.lock`. A merge conflict in
`.todo.json` is resolved by hand, as in any text file. todo checks the result
on the next load: leftover conflict markers or a dangling ID stop every write
until you fix them, with an error naming the problem.

**Old `.todo.db` files.** todo no longer reads SQLite files. A `.todo.db` in the
directory is ignored and left alone. There is no conversion. Until a
`.todo.json` exists, todo says so on each run: on stderr for the CLI (except
under `--json` or `--flat`), in the footer for the TUI.
