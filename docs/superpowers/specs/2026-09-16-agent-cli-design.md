# Design: a headless (agent-facing) CLI for `todo`

**Date:** 2026-09-16
**Status:** Draft for review
**Author:** Andrés (with Claude)

## 1. Motivation

`todo` today is a TUI. Its CLI (`app/cmd/root.go`) only launches the interactive
UI (`todo [file.md]`) or prints the version. There is no non-interactive way to
change a list.

Agents (Claude Code and others) currently have one option: hand-edit the
markdown. Live reload picks that up, and the guide block even invites it — but
hand-editing is the error-prone path we want to replace. It is easy to get
wrong: two-space indentation, the four checkbox markers, the canonical spacing,
the managed guide block, the preserved preamble, and the invariant that a
category's direct tasks precede its subcategories (`AppendTask` in
`internal/todo/model.go`).

This design adds a **headless command layer**: real subcommands
(`todo add/done/progress/defer/reopen/rm/edit/list/…`) that an agent invokes to
mutate a file safely. Each command reuses `internal/todo`, so it inherits
canonical round-tripping, the guide block, preamble preservation, the ordering
invariant, and the existing **atomic write** — for free.

## 2. Goals / non-goals

**Goals**

- Cover the most common actions non-interactively: add a task, add a category,
  set status (done / in-progress / deferred / reopen), edit title/description,
  remove, prune completed, and list.
- A stable, explicit, script-friendly interface an agent can depend on.
- Reuse the domain layer; do not fork parsing/rendering/saving logic.
- Machine-readable output (`--json`) and stable exit codes for the read-then-act
  loop.
- Make the interface discoverable to agents (guide block + docs + `--help`).

**Non-goals**

- No change to the TUI's behavior or key bindings.
- No change to the on-disk markdown format. Files stay clean, greppable,
  ID-free markdown.
- No network, no daemon, no config file.

## 3. The interface (the contract)

### 3.1 File selection

A persistent flag, inherited by every subcommand:

```
-f, --file PATH    the todo file to operate on (default "TODO.md")
```

The interactive entry point keeps its positional arg for backward compatibility:
`todo` and `todo work.md` still open the TUI. Subcommands use `--file` only (no
positional file), keeping the scripting surface fully explicit. Resolution for
the TUI: positional arg if given, else `--file`, else `TODO.md`.

### 3.2 Addressing an existing item

Every command that targets an existing item accepts the item **three ways**,
as explicit, mutually exclusive flags — exactly one is required:

```
--number N     the # column printed by `todo list` (1-based, every row)
--line   N     the item's line number in the .md file
--title  TEXT  content match on the item's title
```

Enforced with Cobra's `MarkFlagsMutuallyExclusive` + `MarkFlagsOneRequired`:

- none given → error: "specify one of --number/--line/--title".
- two given → error: "--number, --line, --title are mutually exclusive".
- `--title` matching more than one item → error listing every candidate with its
  `--number`, `--line`, and category path, so the agent re-issues with a unique
  selector. **Never a silent first-match.**
- `--title`/`--number`/`--line` that resolves to nothing → not-found error.
- A selector that resolves to the wrong kind for the command (e.g. `done` on a
  category) → wrong-kind error.

**Stability note (documented for agents):** `--number` and `--line` are
positional — they shift after any add/remove because the file is re-rendered to
canonical form on save. `--title` is the stable selector. Agents doing a
sequence of edits should either re-run `todo list` between steps or prefer
`--title`. All three always work per this design; the guidance is about
choosing well.

### 3.3 Commands

```
todo list      [--json] [--filter TEXT]

todo add       --title TEXT
               (--parent-number N | --parent-line N | --parent-title TEXT)
               [--desc TEXT] [--status open|progress|deferred|done]

todo add-category  --title TEXT
                   [--parent-number N | --parent-line N | --parent-title TEXT]

todo done      (--number N | --line N | --title TEXT) [--cascade]
todo progress  (--number N | --line N | --title TEXT)
todo defer     (--number N | --line N | --title TEXT)
todo reopen    (--number N | --line N | --title TEXT) [--cascade]

todo rm        (--number N | --line N | --title TEXT)
todo prune

todo edit      (--number N | --line N | --title TEXT)
               [--set-title TEXT] [--set-desc TEXT]
```

Semantics:

- **list** — prints the tree (see §3.4). `--filter` reuses the domain filter
  (`internal/todo/filter.go`); the whole path to every match is kept.
  `--number` and `--line` are always assigned over the **full document**;
  `--filter` only changes which rows are shown, never their numbers — so a number
  read from a filtered list still addresses the same item unfiltered.
- **add** — creates a task under a parent addressed by the `--parent-*` triplet
  (exactly one required, same rules as §3.2). If the parent is a **category**,
  the task is appended as its last direct task (`AppendTask`, preserving the
  tasks-before-subcategories invariant). If the parent is a **task**, the task
  becomes its last subtask. `--status` sets the initial marker (default `open`).
  Tasks cannot be top-level (they must live under a category), so a parent is
  always required.
- **add-category** — creates a category. With no `--parent-*` it is top-level;
  with a parent category it is nested one level deeper.
- **done / progress / defer / reopen** — set the target task's status to Done /
  InProgress / Deferred / Open. One shared implementation. Default is
  **target-only** (see §5.4 for the cascade decision). `--cascade` on `done`
  also completes the whole subtree; `--cascade` on `reopen` reopens the whole
  subtree.
- **rm** — remove the target item and its subtree (`Document.Remove`).
- **prune** — remove every fully-done subtree (`Document.RemoveDone`); prints the
  count removed. Mirrors the TUI's `D`.
- **edit** — set the target's title and/or description; at least one of
  `--set-title` / `--set-desc` is required. (`--set-*` avoids colliding with the
  `--title` selector.)

Naming note: for `add`, `--title`/`--desc` are the **new item's content** and
`--parent-*` is the **target**, so there is no collision. For `edit`, the
selector is `--number/--line/--title` and the new values are `--set-title` /
`--set-desc`.

### 3.4 Output

**`todo list` (human)** — aligned columns so what the agent reads is exactly what
it can type back:

```
  #  line  st   item
  1    23  --   # Work
  2    25  [ ]  Ship v1.0 release
  3    28  [x]    Write changelog
  4    29  [ ]    Cut the git tag
  5    31  --   ## Backend
  6    33  [>]    Migrate the database
```

Indentation shows nesting; `#` is the item number; `line` is the file line; `st`
is the checkbox marker (`--` for categories).

**`todo list --json`** — a nested tree mirroring the file; each node carries
everything needed to address and understand it:

```json
[
  {
    "number": 1, "line": 23, "kind": "category", "title": "Work", "level": 1,
    "path": [],
    "children": [
      {
        "number": 2, "line": 25, "kind": "task", "title": "Ship v1.0 release",
        "status": "open", "path": ["Work"],
        "description": "The release notes, tag, and announcement blog post.",
        "doneCount": 1, "totalCount": 3,
        "children": [
          { "number": 3, "line": 28, "kind": "task", "title": "Write changelog",
            "status": "done", "path": ["Work", "Ship v1.0 release"],
            "doneCount": 0, "totalCount": 0, "children": [] }
        ]
      }
    ]
  }
]
```

- `status` ∈ `open | progress | deferred | done` (task only).
- `level` (category only), `path` = ancestor titles.
- `doneCount`/`totalCount` = descendant-task counts (`Item.TaskCounts`).
- `description` present only when non-empty.

**Mutations** print a one-line human confirmation to stdout, e.g.
`done: [x] Cut the git tag  (Work)`. With `--json` they emit the changed item
object instead (same node shape as above), so a scripted caller gets structured
feedback.

### 3.5 Exit codes

- `0` success.
- `2` usage error (bad/missing/conflicting flags) — Cobra convention.
- non-zero for operational failures, ideally distinct so an agent can branch:
  `3` not found, `4` ambiguous title, `5` wrong kind, `1` I/O or parse error.
  Minimum bar: `0` vs a clear non-zero with a specific stderr message; the
  distinct codes are the preferred polish. All error text goes to **stderr**.

## 4. Discovery (so agents find it)

1. **Guide block** (`internal/todo/guide.go`) — the highest-leverage surface,
   since agents read the top of the file. Add one line, e.g.:
   *"Edit from the CLI (safer than hand-editing): `todo add|done|progress|defer|reopen|rm` — run `todo --help`."*
   Keep it to a line or two; the block is rewritten on every save.
2. **DOCUMENTATION.md** — a new "Scripting / for agents" section: the command
   table, the addressing triplet, the JSON shape, and exit codes.
3. **`--help`** — Cobra generates per-command help; write clear `Short`/`Long`
   and flag usage strings.

## 5. Architecture / internals

### 5.1 Reuse

Each mutating command is:

```
doc, err := todo.Load(path)      // existing; missing file => empty doc
ref := resolve(doc, selector)    // new resolver
// existing op: AppendTask / CascadeSetDone or set Status / Remove / RemoveDone
err = doc.Save(path)             // existing; atomic temp-file + rename, 0644
```

`Load`/`Save`/`FileContent`/`Render` and the tree ops already exist in
`internal/todo`. Reuse means the guide block, preamble, canonical form,
ordering invariant, and atomic write are inherited unchanged. **No TUI code is
touched.**

### 5.2 New domain code (`internal/todo`)

- **Source-line tracking.** Add `Line int` to `Item` (the 1-based line of the
  item's header/checkbox in the *original* source). Populate it in `Parse`,
  which today discards positions (it `stripGuide`s then splits). The parser must
  become guide-aware about line numbers so `--line` and the `line` column match
  the actual on-disk file — including a hand-edited, not-yet-canonical file.
  (Anchor: a category's header line; a task's checkbox line.)
- **Numbering.** A pre-order walk assigning a 1-based `number` over every item
  (categories + tasks) in document order. Computed for `list` and reused by the
  resolver so a `--number` means the same row the agent saw.
- **Resolver.** `func (d *Document) Resolve(sel Selector) (*Item, error)` where
  `Selector` is exactly one of number / line / title. Title match returns an
  ambiguity error carrying all candidates. Callers then assert kind
  (task vs category) per command.

### 5.3 New command code (`app/cmd`)

Follow the existing convention (`version.go`): one `*cobra.Command` constructor
per command, wired in `newRootCommand()`, writing to `cmd.OutOrStdout()` /
`cmd.ErrOrStderr()`, returning errors via `RunE`. Suggested files: `file.go`
(persistent `--file`), `ref.go` (selector flags + resolution + the shared
kind/ambiguity error rendering), `list.go`, `add.go`, `status.go`
(done/progress/defer/reopen), `rm.go`, `prune.go`, `edit.go`. A small
`render_cli.go`/`json.go` for the human table and the JSON node shape.

### 5.4 Decision: cascade behavior (please confirm at review)

The TUI cascades: marking a parent done completes all subtasks, with an in-RAM
undo snapshot. A stateless CLI has no snapshot to restore, so this design makes
the CLI **target-only by default** (predictable for scripting: "mark *this*
done") with an explicit `--cascade` on `done` and `reopen` for the whole
subtree. This is a deliberate divergence from the interactive default; the
alternative is to cascade `done` by default to match the TUI. Flagged as an open
decision in §7.

## 6. Testing

`make verify` runs test + vet + lint + license-check + coverage, and the
coverage threshold is enforced for `app` and `internal`. New code needs tests:

- Domain: line tracking (incl. guide-block offset and a hand-edited file),
  numbering, and the resolver (number/line/title, not-found, ambiguous,
  wrong-kind) in `internal/todo`.
- Commands: golden tests per subcommand asserting the resulting file content and
  stdout/exit code, using the `cmd.OutOrStdout()` seam (as `version_test.go` /
  `root_test.go` do). Include the mutually-exclusive/one-required flag errors and
  the `--json` shape.

## 7. Open decisions (for spec review)

1. **Cascade (§5.4):** target-only + `--cascade` opt-in (proposed), or make CLI
   `done` cascade by default to match the TUI?
2. **`add-category` in v1:** included as proposed; cut if you'd rather ship
   task-only first.
3. **Distinct exit codes (§3.5):** implement the 3/4/5 split, or ship
   `0`/non-zero + clear stderr and add codes later?
4. **`list --json` shape:** nested tree (proposed) vs a flat array of
   addressable rows. (Nested carries structure; flat is simpler to scan.)

## 8. Backward compatibility

- `todo` and `todo <file>` continue to open the TUI unchanged.
- The on-disk format is unchanged; `Line`/`number` are in-memory only.
- The guide block gains a line; existing files pick it up on the next save.
```
