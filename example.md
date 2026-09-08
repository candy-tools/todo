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
-->

# Work

- [ ] Ship v1.0 release
  The release notes, tag, and announcement blog post.
  Aim for Friday.
  - [x] Write changelog
  - [ ] Cut the git tag
  - [ ] Announce on the blog
- [x] Fix login bug
- [ ] Review PR #142
  Big refactor — read it in two sittings.
- [/] Update the API docs
- [ ] Triage incoming bug reports
- [ ] Prepare the sprint demo

## Backend

- [ ] Migrate the database
  Move from SQLite to Postgres.
  Run the migration on staging first.
  - [ ] Write the migration script
  - [ ] Back up production data
  - [/] Verify row counts
- [ ] Add rate limiting
- [x] Refactor auth middleware
- [ ] Write integration tests
- [ ] Add request tracing

## Frontend

- [ ] Redesign the settings page
- [ ] Fix dark-mode contrast
  The muted text fails WCAG AA on dark.
- [x] Upgrade the UI framework
- [ ] Add keyboard shortcuts
- [ ] Audit bundle size

## DevOps

- [ ] Set up a staging environment
- [ ] Rotate the secrets
- [x] Enable CI caching
- [ ] Add uptime monitoring

# Learning

- [ ] Finish the Go concurrency course
  - [x] Watch the channels module
  - [ ] Do the worker-pool exercise
- [ ] Read Designing Data-Intensive Applications
- [ ] Practice algorithms
  - [x] Arrays & strings
  - [ ] Graphs
  - [ ] Dynamic programming
- [x] Set up a spaced-repetition deck
- [ ] Build a small side project
- [ ] Learn a bit of Rust

# Home

- [ ] Fix the leaky kitchen faucet
- [ ] Deep-clean the garage
- [x] Replace the air filter
- [ ] Hang the shelves in the office
- [ ] Water the plants
- [ ] Organize the pantry
- [ ] Descale the coffee machine

# Health

- [ ] Book the annual checkup
- [ ] Dentist cleaning
- [x] Refill the prescription
- [ ] Run three times this week
  - [x] Monday
  - [ ] Wednesday
  - [ ] Friday
- [ ] Meal-prep for the week

# Finance

- [ ] File the quarterly taxes
- [ ] Review recurring subscriptions
  Cancel the ones I forgot about.
- [x] Pay the credit card
- [ ] Rebalance the portfolio
- [ ] Automate the emergency-fund transfer

# Travel

- [ ] Plan the summer trip
  - [ ] Book flights
  - [ ] Book the hotel
  - [ ] Draft the itinerary
- [ ] Renew the passport
- [x] Get a travel-insurance quote
- [ ] Make a packing list

# Reading

- [ ] The Pragmatic Programmer
- [x] Project Hail Mary
- [ ] Thinking, Fast and Slow
- [ ] Dune
- [ ] The Go Programming Language

# Shopping

- [ ] Groceries
  - [ ] Milk
  - [ ] Eggs
  - [ ] Coffee beans
  - [ ] Vegetables
- [ ] New running shoes
- [x] Birthday gift for Sam
- [ ] Printer paper
