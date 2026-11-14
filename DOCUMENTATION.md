# todo — documentation

The full usage guide for [todo](README.md) — how the tree works, every key, the
search filter, and how the list is stored. See the [README](README.md) for
install and development instructions.

## Usage

```sh
todo                 # opens .todo.db in the current directory
todo path/to/my.db   # opens a specific database
todo import file.md  # imports a markdown file into the database
```

With no argument it defaults to `.todo.db`. If the database doesn't exist yet it
is created empty.

**First run with an existing `TODO.md`.** When no path is given (so todo uses
the default `.todo.db`), `.todo.db` doesn't exist and a `TODO.md` is present in
the current directory, todo imports it automatically, once, and prints a summary
(plus a warning for anything it couldn't keep). Naming a database with `-f` or a
positional path disables this automatic import. The markdown file is left
untouched and can be deleted afterwards; from then on the database is the source
of truth. Passing a `.md` path where a database is expected is a usage error
that points you at `todo import`.

**Importing a markdown file.** `todo import file.md` reads a markdown file into
the database (`-f/--file` picks a database other than `.todo.db`). It refuses to
run when the database already has items; pass `--force` to replace them. The
import understands headers as categories, `- [ ]` / `- [/]` / `- [>]` / `- [x]`
checkbox lines as tasks (indentation nests subtasks) and indented text under a
task as its description. Tasks outside any header are gathered under an
`Uncategorized` category, and text it can't map (such as prose between items)
is reported as a warning and dropped.

The screen is a **kanban board**: one tab per column, each showing a collapsible
**category/task tree** of the tasks in that column. Tasks that carry a
description are marked with a small `≡` glyph; press `enter` (or `e`) on any item
to open its dialog, which shows the item's column, state and subtask progress and
lets you edit its title, description and plan in place (`esc` cancels,
`enter`/Save applies).

An example tab, as the tree shows it:

```text
▾ Work (2/5)
  ▾ ○ Ship v1.0 release
      ● Write changelog
      ○ Cut the git tag
    ● Fix login bug
  ▾ Backend (0/1)
      ○ Migrate the database
▾ Personal (0/2)
    ○ Renew passport
    ⏸ Book dentist
+ new category
```

A task's glyph shows its state: `○` open, `⏸` deferred, `◌` new, `◉` ready, `◐`
working, `◑` review, `◒` followup, `●` done. `▾` / `▸` mark an expanded /
collapsed item, and a category shows how many of its tasks are done.

## Board

The board has four columns, shown as tabs (with a task count each): **Backlog**,
**Next**, **In Progress** and **Done**. A task's column comes from its state:

| Column | States (in `t` cycle order) |
|--------|-----------------------------|
| Backlog | `open`, `deferred` |
| Next | `new`, `ready` |
| In Progress | `working`, `review`, `followup` |
| Done | `done` |

- **Categories** group tasks in every tab, but they can only be added, renamed,
  moved or deleted in **Backlog**.
- **Subtasks** go one level deep: a subtask cannot have subtasks of its own. A
  subtask is only ever `open` or `done`, has no board state, and is shown in its
  parent's column. Only a task that is `open` or `done` can hold subtasks.
- **New tasks** take the first state of the tab they are added in (Backlog
  `open`, Next `new`, In Progress `working`, Done `done`). A new subtask is open.
- **Plan.** Each task has an optional plan link — a path or reference to where
  its plan lives — edited in the task's dialog. A `ready` task with no plan is
  flagged **`⚠ no plan`** in its dialog.

Moving a task between columns:

- `]` / `[` move it to the next / previous column; it lands in that column's
  first state. `t` cycles the state inside its column.
- Moving a task out of Done with `[` sets it `working` and restores its subtasks'
  previous states.
- `m` grabs a task to reorder it (see Keys); grab mode steps over tasks hidden in
  the tab.

## Scripting / for agents

Every interactive action is also a non-interactive subcommand, so an agent (or a
script) can edit the list without opening the TUI. All commands take
`-f/--file` (default `.todo.db`).

Read the list — with numbers, ids, and JSON for the read-then-act loop:

```sh
todo list                 # aligned table: # · id · state · item
todo list --json          # nested tree with number/id/title/status/column/plan/path
todo list --filter build  # only matching items and their path
todo list --column next   # only tasks in one column: backlog|next|progress|done
```

The `state` column of `todo list` shows the task's state name (`open`, `ready`,
`working`, …), and `--` for a category. In JSON, `status` is the state name,
`column` is the task's column (`backlog`, `next`, `progress` or `done`; a
subtask reports its parent's column) and `plan` is the plan link.

**Breaking change:** JSON `status` now says `"working"` where it used to say
`"progress"`. On input, `progress` is still accepted as an alias of `working`.

Address an existing item three ways — exactly one, mutually exclusive:

```sh
--number N   # the # column from `todo list`
--id     N   # the item's database id (the id column from `todo list`)
--title  T   # the item's exact title (errors if more than one matches)
```

Common actions:

```sh
todo add --title "Cut the tag" --parent-title "Ship v1.0" [--desc "…"] [--plan docs/plan.md] [--status open|working|deferred|done|new|ready|review|followup]
todo add-category --title "Backend" [--parent-title "Work"]
todo status   --number 4 --to ready
todo done     --title "Cut the tag" [--cascade]
todo progress --number 4   # shortcut: status --to working
todo defer    --id 33      # shortcut: status --to deferred
todo reopen   --title "Cut the tag" [--cascade]
todo edit     --number 4 --set-title "…" --set-desc "…" --set-plan "docs/plan.md"
todo rm       --number 4
todo prune                # drop every fully-completed task
```

`status --to <state>` sets any state: `open`, `working`, `deferred`, `done`,
`new`, `ready`, `review` or `followup` (`progress` is accepted for `working`).
`add --status` defaults to `open`. `edit --set-plan ""` clears a task's plan.
`reopen` sets a task `working`, or `open` for a subtask. `--cascade` on `done` /
`reopen` applies the change to the task's subtasks too.

Breaking the board's rules — a subtask under a subtask, or a board state on a
subtask — exits with code `2`.

`--id` is stable: an item keeps its id for as long as it exists, so it is the
safest way to address an item across a sequence of edits. `--number` shifts
after any add/remove; `--title` is stable as long as it stays unique. Re-run
`todo list` when you rely on `--number`.

Exit codes: `0` ok, `2` usage, `3` not found, `4` ambiguous title, `5` wrong
kind, `1` other. Errors go to stderr.

## Keys

| Key | Action |
|-----|--------|
| `tab` / `shift+tab` | Next / previous board tab |
| `1`–`4` | Jump to Backlog / Next / In Progress / Done |
| `↑` / `↓` (or `w`/`s`, `k`/`j`) | Move the selection |
| `PgUp` / `PgDn` | Move the selection by 10 rows |
| `←` / `→` (or `h`/`l`) | Collapse / expand (Left on a leaf jumps to the parent) |
| `enter` | Open the selected item's dialog — its column/state/progress plus its editable title, description & plan |
| `]` / `[` | Move the task to the next / previous column (it takes that column's first state) |
| `t` | Cycle the task's state within its column |
| `x` / `space` | On a subtask, toggle done. On a task, complete it with its subtasks, or undo / reopen it. On a category, `space` folds / unfolds it |
| `m` | Grab the task to reorder it (arrows move it, `enter`/`esc`/`m` drops it) |
| `n` | New task as a sibling — at the end of the current level (on a category, a task inside it) |
| `N` | New task as a child — a subtask of the selected task (on a category, a task inside it) |
| `c` | Add a category (a subcategory when a category is selected) — Backlog only |
| `e` | Edit the selected item's title (and a task's description) |
| `y` | Copy the selected item to the clipboard — a task's title and full (multi-line) description as plain text |
| `d` | Delete the selected item (and its subtree) — asks to confirm |
| `D` | Remove every completed task (keeping any with unfinished sub-tasks) — asks to confirm |
| `/` | Search / filter the list (see below) |
| `q` | Quit |
| `esc` | Clear an active search, or quit when there is none |

Tasks always live under a category — there are no root-level tasks. The tree
always ends with a **`+ new category`** row: select it and press `c` (or
`enter`) to add a top-level category. On an existing category, `c` adds a
subcategory instead.

In the add/edit dialog: `tab` moves between fields (title, description, plan),
`enter` saves (it inserts a newline while you're in the description box), `esc`
cancels.

Grab mode (`m`) is refused while a search is active.

## Search

Press `/` to filter the current tab as you type — a grep over task/category
titles and task descriptions. The whole path to every match is kept, so a hit deep in a
subtask keeps its parent tasks and category visible (searching `cat` still shows
the tree down to a "catacombs" subtask), while non-matching branches are hidden;
the matched text is highlighted. `enter` keeps the filter and returns you to
navigating the results, `esc` clears it, and pressing `/` again refines the
current query.

## Completing a parent

Pressing `x` / `space` on a parent task completes it and **all** of its subtasks
at once. If you did it by accident, press `x` again right away and the task's
exact previous state and its subtasks' states are restored — that undo memory is
kept in RAM for the session, so the database only ever records the current
states. A done task with no undo history reopens to `working`.

## Live reload

The open TUI notices changes made to the database by another process — most
commonly `todo` CLI commands run by a script or an agent — and applies them
within about a second, so you can watch the list update live. Your selection and
which items are folded are kept across a reload. Changes made inside todo are
written immediately, so editing in the TUI while a script edits from outside
never fight each other.

## Storage

The list lives in a single SQLite file, `.todo.db` by default, in the directory
you run todo from. While todo has it open, SQLite also keeps two side files next
to it, `.todo.db-wal` and `.todo.db-shm` (the write-ahead log and its shared
memory); they are managed by SQLite and normally disappear when the last
connection closes. The format is plain SQLite, so any SQLite tool can read it.

Whether to commit the list to version control or ignore `.todo.db*` is up to
you. If you commit it, commit `.todo.db` only — never `.todo.db-wal` or
`.todo.db-shm`. While todo is running, recent changes may still sit in the WAL
rather than in `.todo.db` itself, so commit after quitting todo. Keep in mind
that it is a binary file, so git can't merge concurrent changes to it.

## Upgrading

Opening a database made by an older todo migrates it once to schema v2:

- It first writes a backup next to the database, `<db>.bak` (e.g. `.todo.db.bak`).
  An existing `.bak` is kept, not overwritten.
- Nesting deeper than one subtask level is flattened.
- Subtask states become `open` / `done`.
- Older todo versions refuse an upgraded database.

To go back, copy the `.bak` over the database file. Don't open the `.bak`
directly with `-f`: that would migrate the backup itself.
