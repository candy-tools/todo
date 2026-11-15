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
  ▾ ○ Ship v1.0 release open (1/2)
      ● Write changelog
      ○ Cut the git tag
    ● Fix login bug done
  ▾ Backend (0/1)
      ○ Migrate the database open
▾ Personal (0/2)
    ○ Renew passport open
    ⏸ Book dentist deferred
+ new category
```

A task's glyph shows its state: `○` open, `⏸` deferred, `⊘` won't do, `◌` new,
`◉` ready, `◐` working, `◑` review, `◒` followup, `●` done. `▾` / `▸` mark an expanded /
collapsed item, and a category shows how many of its tasks are done. A top-level
task's row also names its state and, when it has subtasks, how many are done.

## Board

The board has four columns, shown as tabs (with a task count each): **Backlog**,
**Next**, **In Progress** and **Done**. A task's column comes from its state:

| Column | States |
|--------|--------|
| Backlog | `open`, `deferred`, `wontdo` (shown as "won't do") |
| Next | `new`, `ready` |
| In Progress | `working`, `review`, `followup` |
| Done | `done` |

- **Categories** group tasks in every tab, but they can only be added, renamed,
  moved or deleted in **Backlog**.
- **Subtasks** go one level deep: a subtask cannot have subtasks of its own. A
  subtask is only ever `open` or `done`, has no board state, and is shown in its
  parent's column. A task in any state can hold subtasks.
- **New tasks** take the first state of the tab they are added in (Backlog
  `open`, Next `new`, In Progress `working`, Done `done`). A new subtask is open.
- **Plan.** Each task has an optional plan link — a path or reference to where
  its plan lives — edited in the task's dialog. A `ready` task with no plan is
  flagged **`⚠ no plan`** on its row in the tree and in its dialog.

Moving a task between columns:

- `p` / `P` move it to the next / previous column; it lands in that column's
  first state.
- `t` opens a dialog listing every state, grouped by column; picking one sets it
  (and moves the task when the state belongs to another column). It is the only
  key that sets a state — completing and reopening included. For a subtask it
  offers `open` and `done`.
- Moving a task out of Done with `P` sets it `working`. If you completed it in
  this session, its subtasks get their previous states back; otherwise they keep
  their current ones.
- `m` grabs a task to reorder it (see Keys); grab mode steps over tasks hidden in
  the tab, and moves a task at most one category per key press.

## Views

Two view toggles change how the board is laid out. They are independent, can
be combined, and last for the session (todo always opens on the Backlog tab).

**All states (`k`).** Hides the tabs and shows every task, whatever its state,
in one tree; the tab bar becomes an `All states · N tasks` line. Press `k`
again to get the board back, on the tab you were on. In this view:

- categories can be added, renamed, moved and deleted, as in Backlog, since
  every category is shown;
- a new task starts `open`, as in Backlog;
- `f` offers every state, and its selection is the view's own — the tabs' state
  filters don't apply here and are untouched by it;
- `tab`, `shift+tab` and `1`–`4` only remind you to press `k`.

**Split by category (`v`).** Shows the categories as a tree in a left sidebar
(**Categories**) and the tasks of the selected one in a list on the right
(**Tasks**). Press `x` to move the keys between the two panes; `v` again goes
back to the single tree, on the item that had focus.

```text
╭─ Categories ─────────╮╭─ Tasks ─────────────────────────╮
│  All                 ││❯ ○ Ship v1.0 release open       │
│❯ ▾ Work (2/5)        ││  ── Backend                     │
│      Backend (0/1)   ││  ○ Migrate the database open    │
│    Personal (0/2)    ││                                 │
│  + new category      ││                                 │
╰──────────────────────╯╰─────────────────────────────────╯
```

- The sidebar starts on **All**, which lists every task. Walking down the tree
  narrows the list: a category shows its own tasks first, then each
  subcategory's under a `── Backend` heading.
- The list follows the current tab (or every state with `k`), and a search or
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
todo add --title "Cut the tag" --parent-title "Ship v1.0" [--desc "…"] [--plan docs/plan.md] [--status open|working|deferred|done|new|ready|review|followup|wontdo]
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
`new`, `ready`, `review`, `followup` or `wontdo` (`progress` is accepted for `working`).
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
kind, `6` conflict (the list changed while the command ran, or another todo
kept it locked for writing for more than 5 seconds; re-run it), `1` other.
Errors go to stderr.

## Keys

| Key | Action |
|-----|--------|
| `tab` / `shift+tab` | Next / previous board tab |
| `1`–`4` | Jump to Backlog / Next / In Progress / Done |
| `↑` / `↓` (or `w`/`s`) | Move the selection |
| `PgUp` / `PgDn` | Move the selection by 10 rows |
| `←` / `→` | Collapse / expand (Left on a leaf jumps to the parent) |
| `enter` | Open the selected item's dialog — its column/state/progress plus its editable title, description & plan |
| `p` / `P` | Move the task to the next / previous column (it takes that column's first state) |
| `t` | Open the state dialog to set the task's state, including done (`↑`/`↓`, `enter` sets, `esc` cancels) |
| `space` | Fold / unfold the selected category |
| `m` | Grab the task to reorder it (arrows move it, `enter`/`esc`/`m` drops it) |
| `n` | New task as a sibling — at the end of the current level (on a category, a task inside it) |
| `N` | New task as a child — a subtask of the selected task (on a category, a task inside it) |
| `c` | Add a category (a subcategory when a category is selected) — Backlog only |
| `e` | Edit the selected item's title (and a task's description and plan) |
| `y` | Copy the selected item to the clipboard — a task's title and full (multi-line) description as plain text |
| `d` | Delete the selected item (and its subtree) — asks to confirm |
| `D` | Remove every completed task (keeping any with unfinished sub-tasks) — asks to confirm |
| `/` | Search / filter the list (see below) |
| `f` | Filter the tab by state: tick states with `space`, `enter` applies, `esc` clears the filter |
| `k` | Show every state in one list (tabs hidden) / back to the board — see Views |
| `v` | Split view: categories sidebar + task list / back to the single tree — see Views |
| `x` | In the split view, move the keys between the sidebar and the task list |
| `?` | Show the shortcuts dialog (`↑`/`↓` scroll it on a short terminal; `?`, `esc`, `q` or `enter` closes it) |
| `q` | Quit |
| `esc` | Clear an active search, then the state filter, or quit when there is neither |

Tasks always live under a category — there are no root-level tasks. On the
Backlog tab the tree always ends with a **`+ new category`** row: select it and
press `c` (or `enter`) to add a top-level category. On an existing category, `c` adds a
subcategory instead.

In the add/edit dialog: `tab` moves between fields (title, description, plan),
`enter` saves (it inserts a newline while you're in the description box), `esc`
cancels.

Grab mode (`m`) is refused while a search or state filter is active.

## Search

Press `/` to filter the current tab as you type — a grep over task/category
titles and task descriptions. The whole path to every match is kept, so a hit
deep in a subtask keeps its parent tasks and category visible (searching `cat`
still shows the tree down to a "catacombs" subtask), while non-matching branches
are hidden; the matched text is highlighted. `enter` keeps the filter and returns
you to navigating the results, `esc` clears it, and pressing `/` again refines
the current query.

## State filter

Press `f` to pick which states the current tab shows: tick them with `space`
(or `x`) and press `enter`. Only top-level tasks in a ticked state stay, with
their subtasks and categories. Each tab keeps its own selection, the footer
shows it, and it combines with a search. `esc` clears it; ticking nothing and
pressing `enter` does too.

## Completing a parent

Setting a parent task to `done` (with `t`, or `p` into Done) completes it and
**all** of its subtasks at once. If you did it by accident, press `t` again: the
dialog opens on the task's previous state, so `enter` restores it and its
subtasks' states exactly — that undo memory is kept in RAM for the session, so
the database only ever records the current states. Without undo history the
dialog opens on `done`, and the subtasks keep their states when you reopen it.

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
  An existing backup is never overwritten: when `<db>.bak` already exists, the
  backup goes to the first free `<db>.bak.1`, `<db>.bak.2`, ….
- Nesting deeper than one subtask level is flattened.
- Subtask states become `open` / `done`.
- Older todo versions refuse an upgraded database.

To go back:

1. Quit todo (the TUI and any running `todo` command).
2. Delete the `<db>-wal` and `<db>-shm` side files next to the database, if
   they exist.
3. Copy the backup over the database file: `<db>.bak`, or the highest-numbered
   `<db>.bak.N` when there are several, as that one is the newest.

Don't open a backup directly with `-f`: that would migrate the backup itself.
