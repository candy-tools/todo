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

The screen is a **kanban board**: one tab per column, each showing a collapsible
**category/task tree** of the tasks in that column. Tasks that carry a
description are marked with a small `≡` glyph; press `enter` (or `e`) on any item
to open its dialog, which shows the item's column, state and subtask progress and
lets you edit its title, reference, description and plan in place (`esc` cancels,
`enter`/Save applies).

Like a file explorer listing folders before files, a category shows its
subcategories first and its own tasks below them — in the tree, in the split
view's task list and in `todo list` alike.

An example tab, as the tree shows it:

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
- **Reference.** A task can carry an optional reference such as `APP-001`,
  shown before its title as `[APP-001] Ship it`. It is free-form text (one
  line, up to 50 characters), but no two tasks in a list may share one —
  compared ignoring case, so `app-001` and `APP-001` clash. Subtasks have no
  reference, so a task holding one cannot be indented under another task;
  clear the reference first. Search matches the reference too.

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

Two view toggles change how the board is laid out. They are independent and
can be combined. Both are saved in the list file (see Storage), so todo opens
on the view you left, for everyone who opens that list; the board opens on the
Backlog tab.

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
`-f/--file` (default `.todo.json`).

Read the list — with numbers, ids, and JSON for the read-then-act loop:

```sh
todo list                 # aligned table: # · id · state · item
todo list --json          # nested tree with number/id/ref/title/status/column/plan/path
todo list --filter build  # only matching items and their path
todo list --column next   # only tasks in one column: backlog|next|progress|done
```

The `state` column of `todo list` shows the task's state name (`open`, `ready`,
`working`, …), and `--` for a category. In JSON, `status` is the state name,
`column` is the task's column (`backlog`, `next`, `progress` or `done`; a
subtask reports its parent's column), `plan` is the plan link and `ref` the
task's reference (left out when it has none; `title` never includes it).

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
todo add --title "Cut the tag" --parent-title "Ship v1.0" [--ref APP-001] [--desc "…"] [--plan docs/plan.md] [--status open|working|deferred|done|new|ready|review|followup|wontdo]
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
todo prune                # drop every fully-completed task
```

`status --to <state>` sets any state: `open`, `working`, `deferred`, `done`,
`new`, `ready`, `review`, `followup` or `wontdo` (`progress` is accepted for `working`).
`add --status` defaults to `open`. `edit --set-plan ""` clears a task's plan,
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
| `tab` / `shift+tab` | Next / previous board tab |
| `1`–`4` | Jump to Backlog / Next / In Progress / Done |
| `↑` / `↓` (or `w`/`s`) | Move the selection |
| `PgUp` / `PgDn` | Move the selection by 10 rows |
| `←` / `→` | Collapse / expand (Left on a leaf jumps to the parent) |
| `enter` | Open the selected item's dialog — its column/state/progress plus its editable title, description & plan |
| `p` / `P` | Move the task to the next / previous column (it takes that column's first state) |
| `t` | Open the state dialog to set the task's state, including done (`↑`/`↓`, `enter` sets, `esc` cancels) |
| `space` | Fold / unfold the selected category |
| `z` | Fold every category at the selection's level (the selected category and its siblings; for a task, its category's) — or unfold them all when all are folded. Off while searching |
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

In the add/edit dialog: `tab` moves between fields (title, reference,
description, plan; a subtask has no reference),
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
  "version": 5,
  "view": {
    "split": true
  },
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

- `version` is the format version (now `5`). A file with a higher version is
  refused: `list was written by a newer todo`. A version `3` or `4` file (from
  before references or the view) loads as it is and is written back as
  version `5` on the next change, after which todo versions older than this one
  refuse it.
- `view` is the TUI view the list opens on: `unified` for All states (`k`),
  `split` for the split view (`v`). Each is left out when false, and the whole
  section when both are, which is the board without the split.
- `categories` have an `id`, a `title` and, for a nested one, the `parent`
  category's id. Array order is the order among siblings.
- `tasks` have an `id`, the `category` they sit under, a `title` and a `status`
  (`open`, `working`, `deferred`, `done`, `new`, `ready`, `review`, `followup`
  or `wontdo`). `ref`, `plan`, `description` and `subtasks` are optional.
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
