# todo

A minimalistic, keyboard-driven TODO manager for the terminal. It opens a plain
markdown file as a task list — headers are categories, `- [ ]` items are tasks,
and nested items are subtasks. Everything you do is written straight back to the
file, so your todos stay readable, greppable, and git-friendly.

Built with the [Charm](https://charm.sh) stack: [Bubble Tea](https://github.com/charmbracelet/bubbletea),
[Bubbles](https://github.com/charmbracelet/bubbles), and [Lipgloss](https://github.com/charmbracelet/lipgloss),
with [Cobra](https://github.com/spf13/cobra) for the CLI.

![todo — a keyboard-driven TODO manager for the terminal](zarf/screenshot.jpg)

## Install

todo is a single, dependency-free binary (pure Go, no CGO). Prebuilt archives, a
Debian package, and a macOS cask are published on every tagged release.

### macOS (Homebrew)

A macOS cask is published to the
[candy-tools/homebrew-tap](https://github.com/candy-tools/homebrew-tap) tap on
every tagged release. Add the tap, then install:

```bash
brew tap candy-tools/tap
brew install --cask todo
```

`brew upgrade` tracks future releases. The binary isn't notarized, so the cask
strips the Gatekeeper quarantine flag on install — no "todo is damaged" prompt.

### Debian / Ubuntu

Download the `.deb` for your architecture from the
[releases page](https://github.com/candy-tools/todo/releases) and install it:

```bash
sudo apt install ./todo_*_amd64.deb
```

### From source

With a Go 1.26+ toolchain installed:

```bash
go install github.com/candy-tools/todo@latest
```

Or grab a prebuilt `tar.gz` (`.zip` on Windows) for your OS/arch from the
[releases page](https://github.com/candy-tools/todo/releases).

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

## Develop

Requires Go 1.26+. The full toolchain also needs `golangci-lint`, `goreleaser`,
and `go-licence-detector`. todo has no runtime dependencies — it's a single
static binary.

```bash
make help         # list all targets
make test         # run tests with coverage
make lint         # golangci-lint
make vet          # go vet
make coverage     # enforce the per-package coverage threshold
```

### Run

```bash
make run          # runs against example.md
# or plain go:
go run . path/to/file.md
```

### Build

```bash
make build        # goreleaser snapshot build for the current OS/arch → ./dist
# or plain go:
go build ./...
```

### Release

Releases are published by pushing a semver tag from a clean `main` branch. The
tag triggers the [Release workflow](./.github/workflows/release.yml), which runs
GoReleaser to build and publish the archives, `.deb`, and Homebrew cask.

```bash
make tag version="v1.2.3"
```

`make tag` refuses to run unless you're on `main` with a clean working tree, then
creates and pushes the `vX.Y.Z` tag.
