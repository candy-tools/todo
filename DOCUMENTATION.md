# todo — documentation

The full usage guide for [todo](README.md) — how the tree works, every key, the
search filter, and the markdown file format. See the [README](README.md) for
install and development instructions.

## Usage

```sh
todo                 # opens TODO.md in the current directory
todo path/to/file.md # opens a specific file
```

With no argument it defaults to `TODO.md`. If the file doesn't exist yet it
starts empty and is created on the first change.

The screen is a single collapsible **category/task tree**. Tasks that carry a
description are marked with a small `≡` glyph; press `enter` (or `e`) on any item
to open its dialog, which shows the item's status and subtask progress and lets
you edit its title and description in place (`esc` cancels, `enter`/Save applies).

An example task file:

```markdown
# Work

- [ ] Ship v1.0 release
  The release notes, tag, and announcement blog post.
  - [x] Write changelog
  - [ ] Cut the git tag
- [x] Fix login bug

## Backend

- [ ] Migrate the database

# Personal

- [ ] Renew passport
- [ ] Book dentist
```

## Scripting / for agents

Every interactive action is also a non-interactive subcommand, so an agent (or a
script) can edit the file without opening the TUI. All commands take
`-f/--file` (default `TODO.md`).

Read the list — with numbers, line numbers, and JSON for the read-then-act loop:

```sh
todo list                 # aligned table: # · line · status · item
todo list --json          # nested tree with number/line/title/status/path
todo list --filter build  # only matching items and their path
```

Address an existing item three ways — exactly one, mutually exclusive:

```sh
--number N   # the # column from `todo list`
--line   N   # the item's line in the file
--title  T   # the item's exact title (errors if more than one matches)
```

Common actions:

```sh
todo add --title "Cut the tag" --parent-title "Ship v1.0" [--desc "…"] [--status open|progress|deferred|done]
todo add-category --title "Backend" [--parent-title "Work"]
todo done     --title "Cut the tag" [--cascade]
todo progress --number 4
todo defer    --line 33
todo reopen   --title "Cut the tag" [--cascade]
todo edit     --number 4 --set-title "…" --set-desc "…"
todo rm       --number 4
todo prune                # drop every fully-completed task
```

`--number` and `--line` shift after any add/remove (the file is rewritten to
canonical form on save); `--title` is stable. For a sequence of edits, re-run
`todo list` between steps or address by `--title`.

Exit codes: `0` ok, `2` usage, `3` not found, `4` ambiguous title, `5` wrong
kind, `1` other. Errors go to stderr.

## Keys

| Key | Action |
|-----|--------|
| `↑` / `↓` (or `w`/`s`, `k`/`j`) | Move the selection |
| `PgUp` / `PgDn` | Move the selection by 10 rows |
| `←` / `→` (or `h`/`l`) | Collapse / expand (Left on a leaf jumps to the parent) |
| `enter` | Open the selected item's dialog — its status/progress plus its editable title & description |
| `space` | Toggle a task done, or fold / unfold a header (category) |
| `x` | Toggle a task done |
| `p` | Toggle a task **in progress** (`[/]`) — press again to clear it |
| `>` | Toggle a task **deferred** (`[>]`) — press again to clear it |
| `n` | New task as a sibling — at the end of the current level (on a category, a task inside it) |
| `N` | New task as a child — a subtask of the selected task (on a category, a task inside it) |
| `c` | Add a category (a subcategory when a category is selected) |
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

In the add/edit dialog: `tab` moves between fields, `enter` saves (it inserts a
newline while you're in the description box), `esc` cancels.

## Search

Press `/` to filter the list as you type — a grep over task/category titles and
task descriptions. The whole path to every match is kept, so a hit deep in a
subtask keeps its parent tasks and category visible (searching `cat` still shows
the tree down to a "catacombs" subtask), while non-matching branches are hidden;
the matched text is highlighted. `enter` keeps the filter and returns you to
navigating the results, `esc` clears it, and pressing `/` again refines the
current query.

## Completing a parent

Marking a parent task done completes **all** of its subtasks at once. If you did
it by accident, just unmark the parent again and the previous subtask states are
restored — that undo memory is kept in RAM for the session, so the file only
ever records the current checkbox states.

## Live reload

todo re-reads the open file about once a second and applies any change made
outside the app — so you can edit the raw markdown in your editor, or let an
agent update it, and watch it update live. Your selection and which items are
folded are kept across a reload. The app writes atomically and ignores its own
saves, so editing inside todo and watching from outside never fight each other.

## File format

The app owns a small, standard subset of markdown:

```markdown
# Work

- [ ] Ship v1.0 release
  The description sits indented under the task and shows in the item's dialog.
  - [x] Write changelog
  - [/] Cut the git tag
- [x] Fix login bug

## Backend

- [>] Migrate the database
```

- **Headers** (`#`..`######`) are categories and nest by level.
- **Tasks** are checkbox lines; indentation nests subtasks. Four states:
  `- [ ]` open, `- [/]` in progress, `- [>]` deferred, `- [x]` done — all
  standard markers that render on GitHub and in editors like Obsidian too.
- **Indented text** under a task (that isn't a checkbox) is its description.
- Text before the first header/task is preserved as-is; other free-form prose
  between items is not part of the format.

todo also keeps a short **guide block** — an HTML comment (so it stays invisible
when the markdown is rendered) — at the very top of every saved file. It links
back to this repo and documents the format above for whoever edits the file
directly, human or agent. It's app-managed: todo rewrites it on every save, so
there's no need to edit it by hand.
