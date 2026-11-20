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
(plus a warning for anything it couldn't keep). Naming a file with `-f` or a
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
the **Backlog** and the **Sprints** workspace, which has a board per sprint — and
a temporary **Planning** screen (see Workspaces below). The Backlog and each
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
  sprint's board, which has three columns: **Next**, **In Progress** and
  **Done**.

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
only assigning does (`a` / `o` / Planning in the TUI, `todo assign` / `todo unassign` on
the command line).

Sprint rules:

- **Assigning** a backlog task to a sprint makes it `new`. A task assigned from
  one sprint to another keeps its state. Unassigning returns it to the Backlog
  as `open`. Its subtasks keep their open/done state and travel with it.
- **Closing** is refused while any task of the sprint (subtasks included) is
  not done: `can't close: N tasks not done`. Send the unfinished tasks to
  another open sprint or back to the Backlog first. A closed sprint can be
  reopened.
- **Deleting** a sprint returns its unfinished tasks to the Backlog as `open`.
  It is refused while the sprint holds done tasks, so finished work never
  disappears as a side effect: close the sprint instead, or remove its done
  tasks first.
- **A closed sprint is frozen.** Assigning to it, adding a task to it, moving a
  task out of it and setting one of its tasks to a state other than done are
  refused with `reopen the sprint first`. Editing a task's text, deleting a
  task, removing its done tasks and moving tasks between categories still work.
- Sprint names are unique in the list, ignoring case.
- **Nesting.** Indenting a task under another task is allowed only when both
  are in the same sprint, or both in the Backlog.

Other rules, as before:

- **Categories** group tasks on every screen, but they can only be added,
  renamed, moved or deleted in the **Backlog** (and the unified view, `u`).
- **Subtasks** go one level deep: a subtask cannot have subtasks of its own. A
  subtask is only ever `open` or `done`, has no board state, and is shown in its
  parent's column. A task in any state can hold subtasks.
- **New tasks** take the first state of where they are added: `open` in the
  Backlog, and on a sprint board the first state of the tab (Next `new`, In
  Progress `working`, Done `done`), in that sprint. A new subtask is open.
- **Plan.** Each task has an optional plan link — a path or reference to where
  its plan lives — edited in the task's dialog. A `ready` task with no plan is
  flagged **`⚠ no plan`** on its row in the tree and in its dialog.
- **Reference.** A task can carry an optional reference such as `APP-001`,
  shown before its title as `[APP-001] Ship it`. It is free-form text (one
  line, up to 50 characters), but no two tasks in a list may share one —
  compared ignoring case, so `app-001` and `APP-001` clash. Subtasks have no
  reference, so a task holding one cannot be indented under another task;
  clear the reference first. Search matches the reference too.

Moving a task between columns and sprints:

- On a sprint board, `a` / `A` move it to the next / previous column; it lands
  in that column's first state. `A` on a Next task sends it back to the Backlog
  (it becomes `open`) and the footer says `moved to Backlog`.
- On a backlog task, `a` opens a picker of the open sprints and assigns the
  task to the one you choose (it becomes `new`). `A` does nothing there. To
  pull several tasks into one sprint, use Planning (`p`, see Workspaces).
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
  the tab, and moves a task at most one category per key press.
- On a closed sprint's board every key that would change a task's sprint or
  state, `o` included, answers `reopen the sprint first`.

## Workspaces

The header's second line is a switcher, `b Backlog   s Sprints   p Planning`,
with the workspace you are in bracketed (`[s Sprints]`), followed by a
breadcrumb that says where you are:

```text
 b Backlog   [s Sprints]   p Planning   Sprints › Sprint 14 › Next
```

The breadcrumb reads `Backlog › Work › Reviews · N tasks waiting` (the category
under the cursor), `Sprints › Sprint 14 › Next` (a closed sprint is marked `✓`),
`Sprints › Closed`, `All states · N tasks` in the unified view, or, in Planning,
`Sprint 14 · N in Next · N in the Backlog`. The footer shows a few keys of
where you are, most important first, and drops the last ones whole on a narrow
terminal; everything else is in the `?` dialog:

- the Backlog: `?` help, `tab` pane (in the split), `a` assign, `v` split (or
  `v` single tree in the split), `s` sprints, `p` plan, `/` search, `q` quit;
- a sprint board: `?` help, `/` search, `q` quit, `tab` sprints and `L`
  columns (or tabs);
- the Sprints sidebar: `n` new, `e` edit, `c` close/reopen, `d` delete (on the
  Closed row `←→` fold instead of those three), `tab` board, `?` help, `q`
  quit;
- Planning: the focused side's keys (`→/a` pull in on the Backlog side, `←/A`
  back and `r` ready on the sprint side; `tab` the other side, `k` other
  sprint, `esc` done) and `?` help;
- the unified view: `?` help, `/` search, `q` quit, and `tab` pane when split.

### Backlog (`b`)

The category tree of the tasks in no sprint, in a pane titled **Backlog**. `v`
splits it into a categories sidebar and the task list (see Views); `tab` then
moves the keys between the two panes, and `esc` in the task list goes back to
the categories. `a` on a task assigns it to a sprint (a picker of the open
sprints; on a category it says `select a task to assign it to a sprint`); `p`
opens Planning to pull several in.

### Sprints (`s`)

A sidebar of sprints on the left, the selected sprint's board on the right.
Each sidebar row shows the sprint's name, its task counts (Next · In Progress ·
Done) and its goal; names and goals are cut with `…`, never wrapped. Open
sprints come first in creation order, then a folded **Closed** group holding
the done sprints. `→` / `enter` unfold the group on its row and `←` folds it
(`←` on a done sprint jumps to the group row). The arrows move the selection
and show that sprint's board at once.

On the **Closed** row the right pane is **Closed sprints** instead of a board:
each closed sprint, newest first, as `── ✓ Sprint 12` over its goal and its
task count (`… N more` when they do not all fit; `no open sprints` above them
when every sprint is closed). It is read only: the board keys (`[` `]`,
`1`–`3`, `a`/`A`, `L`, `/`, `f`, …) do nothing there, `tab` does nothing, and
`enter`, `→` and `←` fold or unfold the group. todo still reopens on the last
sprint whose board you saw.

The keys start on the sidebar: `n` creates a sprint (a form with Name and
Goal), `e` edits its name and goal, `c` closes an open sprint or reopens a
closed one, and `d` deletes it after a confirmation. A refused close or delete
says why in the footer (`can't close: 3 tasks not done`; delete is refused
while the sprint holds done tasks). `enter` or `tab` moves the keys onto the
board, and `tab` or `esc` on the board brings them back to the sidebar.

Above the board's tree is a **head** of three lines: the sprint's goal, a
progress bar of the done tasks with whether the sprint can be closed (`close
blocked: 3 not done`, `ready to close`, `no tasks yet`, or `closed` for a done
sprint), and the column bar, `[Next 3]   In Progress 1   Done 2`. On a short
terminal the head yields rows to the tree so the cursor stays visible.

The board shows one column at a time. `[` / `]` step to the previous / next
column and `1`–`3` jump to Next / In Progress / Done. `L` lays the board out as
three side-by-side columns instead (`L` again goes back); on a pane too narrow
for columns it says `too narrow for columns`. The layout is not saved. Each
sprint board remembers its own column, folds and selection.

### Planning (`p`)

A temporary screen for pulling tasks into a sprint: the Backlog on the left,
grouped by category, and the planned sprint on the right, its three columns
(Next, In Progress, Done) each under a header with its task count. From the Backlog or the Sprints
workspace, `p` plans the sprint on screen when it is open, else the last one
you viewed while it is open, else the first open one (with no open sprint it
says `no open sprints — press s, then n to create one`). It starts with the
keys on the Backlog side. Planning itself is never saved: todo reopens on the
view you had before (entering Planning saves the sprint you browsed to in the
sidebar).

| Key | Action |
|-----|--------|
| `↑` / `↓`, `PgUp` / `PgDn` | Move the selection on the focused side |
| `→` / `a` | Backlog side: pull the task into the sprint (it becomes `new`) |
| `←` / `A` | Sprint side: send a Next task back to the Backlog (it becomes `open`) |
| `r` | Sprint side: toggle a task between `new` and `ready` |
| `tab` / `shift+tab` | Switch between the Backlog side and the sprint side |
| `k` / `K` | Plan the next open sprint / pick a sprint: an open one becomes the planned sprint, a closed one leaves Planning for its board |
| `p` / `esc` | Leave Planning for the planned sprint's board, on Next |

`b`, `s`, `q` and `?` work as everywhere. If another session deletes or closes
the planned sprint, Planning closes on the Sprints workspace and the footer
says `the planned sprint is gone or closed`.

### Moving around

| Key | Goes to |
|-----|---------|
| `b` | The Backlog |
| `s` | The Sprints workspace, on the last sprint you viewed, else the first open one |
| `p` | Planning |
| `k` | The next open sprint, wrapping around; from the Backlog, the last sprint you viewed while it is open, else the first open one |
| `K` | A dialog listing every sprint, closed ones marked `✓`; `enter` opens its board (in Planning, an open sprint becomes the planned one and a closed one leaves Planning for its board) |
| `q` | Quit, from every screen |
| `esc` | Clear the search, then the state filter; then, on a board, back to the sprint sidebar, and in the Backlog split's task list, back to the categories; in Planning, leave it; else quit |
| `tab` / `shift+tab` | Switch pane: the Backlog's split panes, the Sprints sidebar and board, Planning's two sides |
| `[` / `]`, `1`–`3` | On a sprint board, switch between Next / In Progress / Done |

`k` and `K` with no sprint to go to show a hint instead (`no open sprints —
press s, then n to create one`). todo saves the workspace and sprint in the
list file and reopens there; a saved view that names no sprint, or one that no
longer exists, opens the Sprints workspace on the first open sprint.

## Views

Two view toggles change how the tasks are laid out. They are independent and
can be combined. Both are saved in the list file (see Storage), so todo opens
on the view you left, for everyone who opens that list. (The `L` columns layout
and Planning are not saved.)

**Unified view (`u`).** On the Backlog or a sprint board, shows every task,
whatever its state and sprint, in one tree; each task shows its sprint's name
next to its state, and the breadcrumb becomes `All states · N tasks`. Press
`u` again to return to the screen you came from; `b`, `s`, `p`, `k` and `K`
leave it and go where they say. In this view:

- categories can be added, renamed, moved and deleted, as in Backlog, since
  every category is shown;
- `n` adds a backlog task (`open`) and `N` a subtask of the selected task;
- each task's keys follow its own place: on a backlog task `a` opens the sprint
  picker and `t` offers the backlog states; on a sprint task `a`, `A`, `t` and
  `o` work as on its sprint board;
- `f` offers every state, and its selection is the view's own — the tabs' state
  filters don't apply here and are untouched by it;
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
  matches. When the selected category disappears (switching tab, filtering,
  deleting it) the sidebar falls back to All.
- Categories are edited in the sidebar (`enter`/`e`, `c`, `d`, `m`) under the
  usual rules; `n`/`N` there add a task to the selected category. In the list,
  the task keys work as usual; `n` in an empty list adds to the selected
  category.

## Scripting / for agents

Every interactive action is also a non-interactive subcommand, so an agent (or a
script) can edit the list without opening the TUI. All commands take
`-f/--file` (default `.todo.json`).

Read the list — with numbers, ids, and JSON for the read-then-act loop:

```sh
todo list                 # aligned table: # · id · state · item
todo list --json          # nested tree with number/id/ref/title/status/column/plan/path
todo list --filter build  # only matching items and their path
todo list --column next   # only tasks in one column: backlog|next|progress|done
todo list --sprint "Sprint 13"  # only that sprint's tasks (by id or name)
todo list --backlog       # only tasks in no sprint (same as --column backlog)
todo sprint list [--all] [--json]  # open sprints with their task counts
```

The `state` column of `todo list` shows the task's state name (`open`, `ready`,
`working`, …), and `--` for a category. In JSON, `status` is the state name,
`column` is the task's column (`backlog`, `next`, `progress` or `done`; a
subtask reports its parent's column), `plan` is the plan link and `ref` the
task's reference (left out when it has none; `title` never includes it).
`sprint` is the id of the sprint a top-level task is in (left out for a backlog
task). `--column` without `--sprint` covers that column across every sprint;
`--column backlog` with `--sprint` is a usage error.
`todo sprint list --json` is an array of `{id, name, goal, status, counts}`,
where `status` is `open` or `done` and `counts` has the `next`, `progress` and
`done` task counts. `--all` includes closed sprints.

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
todo add-category --title "Backend" [--parent-title "Work"]
todo status   --number 4 --to ready
todo done     --title "Cut the tag" [--cascade]
todo progress --number 4   # shortcut: status --to working
todo defer    --id k3f9qw2 # shortcut: status --to deferred
todo reopen   --title "Cut the tag" [--cascade]
todo edit     --number 4 --set-title "…" --set-desc "…" --set-plan "docs/plan.md" --set-ref APP-002
todo move     --title "Cut the tag" --parent-title "Backend" [--first | --after-title "…"]
todo move     --title "Backend" --root
todo rm       --number 4
todo prune --sprint "Sprint 13"  # drop that sprint's fully-completed tasks
todo prune --all                 # drop them in every sprint and the backlog

todo sprint add    --name "Sprint 13" [--goal "ship sprint support"]   # prints the new id
todo sprint edit   --sprint "Sprint 13" [--set-name "…"] [--set-goal "…"]
todo sprint close  --sprint "Sprint 13"
todo sprint reopen --sprint "Sprint 13"
todo sprint rm     --sprint "Sprint 13"
todo assign        --sprint "Sprint 13" --number 4
todo unassign      --number 4
```

`status --to <state>` sets any state: `open`, `working`, `deferred`, `done`,
`new`, `ready`, `review`, `followup` or `wontdo` (`progress` is accepted for `working`).
`add --status` defaults to `open`, or `new` with `--sprint`; a sprint state
without `--sprint`, or `--sprint` with a backlog state, is a usage error (exit
code `2`). `edit --set-plan ""` clears a task's plan,
and `edit --set-ref ""` its reference. A reference another task already holds,
or one given to a subtask, is refused as a usage error (exit code `2`).
`move` places an item, with its subtree, under the parent named by
`--parent-number`, `--parent-id` or `--parent-title` (or at the top level with
`--root`, for a category): last among its kind there by default, first with
`--first`, or right after the sibling named by `--after-number`, `--after-id` or
`--after-title`, which must sit directly under that parent (exit code `2`
otherwise). Subcategories always stay ahead of tasks. The move rules match the
TUI's: a task never goes to the top level and a category never under a task
(exit code `5`); a task with subtasks, or with a reference, cannot become a
subtask; a moved subtask becomes a task and takes its state from its old parent.
`reopen` takes a done task back to `working` (a subtask to `open`); a task that
is not done keeps its state and nothing is written (with `--cascade` its
subtasks are reopened). `--cascade` on `done` / `reopen` applies the change to
the task's subtasks too.

**Sprints.** `--sprint` takes a sprint's id or its name (ignoring case); sprint
names are unique, so a name is unambiguous. `todo sprint add` prints the new
sprint's id. `assign` puts a task in a sprint: a backlog task becomes `new`, a
task in another sprint moves there and keeps its state; `unassign` returns it
to the backlog as `open`. `status`, `done`, `progress`, `defer` and `reopen`
never change a task's sprint, so `status --to working` on a backlog task fails
with `task is not in a sprint (use todo assign)`, and a backlog state on a
sprint task is refused too. `sprint close` is refused while tasks are not done
(`cannot close "S1": 1 task not done`); `sprint rm` returns a sprint's
unfinished tasks to the backlog and is refused while it holds done tasks;
changing the state of a task in a closed sprint fails with `reopen the sprint
first`. All of these exit with code `2`; an unknown sprint exits `3`.

The two prunes differ on subtasks. `prune --sprint` (like `D` on a sprint
board) removes only the sprint's top-level tasks that are done with every
subtask done; a done subtask under an unfinished task stays. `prune --all`
also removes those done subtasks under unfinished tasks, in every sprint and
the backlog.

**Breaking change:** `prune` now needs `--sprint <id|name>` or `--all`; a bare
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
| `f` | Filter the tab by state: tick states with `space`, `enter` applies, `esc` clears the filter |
| `b` | Go to the Backlog |
| `s` | Go to the Sprints workspace (sprint sidebar + board) |
| `p` | Planning: the Backlog beside a sprint — see Workspaces |
| `k` | Last viewed sprint while it is open, else the first open one; on a sprint board, the next open sprint |
| `K` | Pick a sprint to go to |
| `q` | Quit, from every screen |
| `tab` / `shift+tab` | Switch pane: Backlog split panes, Sprints sidebar / board, Planning sides |
| `[` / `]` | Previous / next column (sprint board) |
| `1`–`3` | Jump to Next / In Progress / Done (sprint board) |
| `L` | Show the sprint board as tabs or as three columns |
| `u` | Unified view: every task, any sprint / back — see Views |
| `a` / `A` | Move the task to the next / previous column (it takes that column's first state) |
| `a` | On a backlog task: assign it to a sprint (picker of open sprints); on a Backlog category it says to select a task |
| `A` | On a Next task: send it back to the Backlog |
| `o` | Send the task to another open sprint or the Backlog (picker) |
| `t` | Open the state dialog to set the task's state, including done (`↑`/`↓`, `enter` sets, `esc` cancels) |
| `m` | Grab the task to reorder it (arrows move it, `enter`/`esc`/`m` drops it) |
| `y` | Copy the selected item to the clipboard — a task's title and full (multi-line) description as plain text |
| `enter` / `e` | Open / edit the selected item's dialog — its column/state/progress plus its editable title, reference, description & plan |
| `n` | New task as a sibling — at the end of the current level (on a category, a task inside it); on a sprint board, in that sprint |
| `N` | New task as a child — a subtask of the selected task (on a category, a task inside it) |
| `c` | Add a category (a subcategory when a category is selected) — Backlog and unified view only; on the Sprints sidebar, `c` closes / reopens the sprint |
| `d` | Delete the selected item (and its subtree) — asks to confirm |
| `D` | On a sprint board, remove that sprint's completed tasks (keeping any with unfinished sub-tasks) — asks to confirm |
| `v` | Backlog split view: categories sidebar + task list / back to the single tree — see Views |
| `?` | Show the shortcuts dialog (`↑`/`↓` scroll it on a short terminal; `?`, `esc`, `q` or `enter` closes it) |
| `esc` | Clear an active search, then the state filter; when there is neither, on a sprint board back to the sprint sidebar, in the Backlog split's task list back to the categories, else quit |

On the **Sprints sidebar**: `n` creates a sprint, `e` edits its name and goal,
`c` closes or reopens it, `d` deletes it, and `enter` or `tab` focuses its
board.

Tasks always live under a category — there are no root-level tasks. In the
Backlog the tree always ends with a **`+ new category`** row: select it and
press `c` (or `enter`) to add a top-level category. On an existing category, `c` adds a
subcategory instead.

In the add/edit dialog: `tab` moves between fields (title, reference,
description, plan; a subtask has no reference),
`enter` saves (it inserts a newline while you're in the description box), `esc`
cancels.

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
their subtasks and categories. Each tab keeps its own selection, the footer
shows it, and it combines with a search. `esc` clears it; ticking nothing and
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
  "version": 6,
  "view": {
    "screen": "sprint",
    "sprint": "a1b2c3d",
    "split": true
  },
  "sprints": [
    {
      "id": "a1b2c3d",
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

- `version` is the format version (now `6`). A file with a higher version is
  refused: `list was written by a newer todo`. A version `3`, `4` or `5` file
  (from before sprints) loads as it is and is written back as version `6` on
  the next change, after which todo versions older than this one refuse it.
  **Upgrade:** those files have tasks in Next, In Progress or Done with no
  sprint, which version `6` forbids. On load, todo puts every such top-level
  task into one open sprint named `Imported`, keeping its state, so nothing
  changes column and nothing disappears. Backlog tasks stay out of any sprint,
  and a file with no such task gets no `Imported` sprint. A markdown import
  follows the same rule for its `[/]`, `[>]` and `[x]` tasks.
- `view` is the screen and view the list opens on: `screen` is `backlog`
  (default), `sprints` or `sprint`, with `sprint` the id of the board shown for
  `screen: sprint`; `unified` is the unified view (`u`) and `split` the Backlog's
  split view (`v`). Each is left out when it is the default, and the whole
  section when all are. A saved `screen: sprints`, or a saved sprint that no longer
  exists, opens the Sprints workspace on the first open sprint.
- `sprints` is always written, possibly empty, in creation order (which is the
  `k` rotation order). A sprint has an `id`, a `name` (unique, ignoring case),
  a `goal` (optional) and a `status`, `open` or `done`.
- `categories` have an `id`, a `title` and, for a nested one, the `parent`
  category's id. Array order is the order among siblings.
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

**Hand edits.** You can edit `.todo.json` by hand. Two things to know:

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
`.todo.json` exists, todo says so on each run: on stderr for the CLI, in the
footer for the TUI.
