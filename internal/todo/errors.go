package todo

import (
	"errors"
	"fmt"
)

// The store's errors come in classes, so a caller can treat a whole class
// alike without listing its members: errors.Is(err, ErrBadRequest) holds for
// every member of that class, wrapped or not, and errors.Is(err, ErrTooDeep)
// still holds for that member alone. ErrNotFound, ErrConflict,
// ErrInvalidList and ErrListMissing are both a class and its first member.
// ErrNotEmpty belongs to no class: only an import that does not replace the
// list returns it.
var (
	// ErrBadRequest is the class of a write the rules refuse whatever the
	// list holds: retrying it unchanged fails again.
	ErrBadRequest = errors.New("bad request")
	// ErrWrongKind is the class of a write that names an item of the wrong
	// kind for it: a task where a category is needed, or the other way round.
	ErrWrongKind = errors.New("wrong kind of item")
	// ErrInternal is the class of a failure that is a bug in todo, not in the
	// request or the file, such as a write that produced a list the format
	// refuses (nothing is written then).
	ErrInternal = errors.New("internal error")
)

// ErrNotFound is returned by a write that names an item (or a parent) the
// store does not hold. It is its own class.
var ErrNotFound = errors.New("item not found")

// ErrConflict is returned by a write whose placement no longer fits the tree
// it finds: the sibling it names to place an item after is no longer under
// that parent. It usually means another process changed the list since the
// caller read it. It is also the class of ErrBusy: re-running the request is
// the fix for both.
var ErrConflict = errors.New("the list changed since it was read; reload and try again")

// ErrInvalidList is returned (wrapped) when the list file is not a valid list:
// it does not decode, or it breaks the format's rules. It is also the class of
// ErrNewerFormat: a file this todo cannot read.
var ErrInvalidList = errors.New("invalid list file")

// ErrListMissing is returned by a write through a store from Existing when
// the list file does not exist: it was removed since the caller read it. It
// is its own class.
var ErrListMissing = errors.New("the list file does not exist")

// ErrTooDeep is returned by a write that would nest a task below a subtask:
// subtasks cannot have subtasks. Its class is ErrBadRequest.
var ErrTooDeep = classed(ErrBadRequest, "subtasks cannot have subtasks")

// ErrSubtaskStatus is returned by a write that would give a subtask a state
// other than Open or Done. Its class is ErrBadRequest.
var ErrSubtaskStatus = classed(ErrBadRequest, "a subtask can only be open or done")

// ErrInvalidMove is returned by a move that would put an item under itself or
// one of its descendants. It is usually a bad request, but can also follow a
// stale read (another process made the target a descendant of the item since
// the caller read the list); either way, retrying the same move fails again.
// Its class is ErrBadRequest.
var ErrInvalidMove = classed(ErrBadRequest, "cannot move an item into its own subtree")

// ErrInvalidTitle is returned by a write that would store an empty title, one
// longer than MaxTitleLen characters, or one holding a control character (a
// newline, tab or escape among them) or another unsafe character (see
// unsafeRune). Its class is ErrBadRequest.
var ErrInvalidTitle = classed(ErrBadRequest, "invalid title")

// ErrInvalidRef is returned by a write that would store a reference longer
// than MaxRefLen characters or one holding a control or other unsafe
// character (see unsafeRune). Its class is ErrBadRequest.
var ErrInvalidRef = classed(ErrBadRequest, "invalid reference")

// ErrDuplicateRef is returned by a write that would give a task a reference
// another task of the list already holds (compared ignoring case). Its class
// is ErrBadRequest.
var ErrDuplicateRef = classed(ErrBadRequest, "duplicate reference")

// ErrSubtaskRef is returned by a write that would give a subtask a reference,
// or move a task that has one under a task: only a top-level task has a
// reference. Its class is ErrBadRequest.
var ErrSubtaskRef = classed(ErrBadRequest, "a subtask cannot have a reference")

// ErrRestoreAndCascade is returned by a state change asked both to restore the
// subtasks from a snapshot and to cascade to them all: the two disagree. Its
// class is ErrBadRequest.
var ErrRestoreAndCascade = classed(ErrBadRequest, "a reopen cannot both restore the subtasks and open them all")

// ErrNowhere is returned by StepPlacement for a step with nowhere to go: the
// item is already first or last that way, has no sibling of its kind before
// it to nest under (unless it is too deep to nest at all: ErrTooDeep), or no
// parent it may leave. Its class is ErrBadRequest.
var ErrNowhere = classed(ErrBadRequest, "nowhere to move the item that way")

// ErrWrongParent is returned by a write that would place a task at the root or
// a category under a task: a task always sits under a category or a task, and
// a category under a category or at the root. Its class is ErrWrongKind.
var ErrWrongParent = classed(ErrWrongKind, "a task must sit under a category and a category cannot sit under a task")

// ErrNotTask is returned by a write that would give a category a state, a
// description, a plan or a reference: only a task has them. Its class is
// ErrWrongKind.
var ErrNotTask = classed(ErrWrongKind, "only a task has a state, a description, a plan or a reference")

// ErrBusy is returned by a write that gave up waiting for another process to
// release the list's write lock (after the store's busy timeout). Its class
// is ErrConflict.
var ErrBusy = classed(ErrConflict, "list busy (another todo is writing), try again")

// ErrNewerFormat is returned when the list file was written by a newer todo,
// whose format this binary does not know and must not read or write. It is
// wrapped with the version found. Its class is ErrInvalidList.
var ErrNewerFormat = classed(ErrInvalidList, "list was written by a newer todo")

// ErrNotEmpty is returned by Import when the list already holds items and the
// caller did not ask to replace them. It belongs to no class.
var ErrNotEmpty = errors.New("list is not empty")

// ErrSprintNotFound is returned by a write that names a sprint the list does
// not hold. Its class is ErrNotFound.
var ErrSprintNotFound = classed(ErrNotFound, "sprint not found")

// ErrNeedsSprint is returned by a write that would give a task outside a
// sprint a state that only a sprint task has. Its class is ErrBadRequest.
var ErrNeedsSprint = classed(ErrBadRequest, "task is not in a sprint (use todo assign)")

// ErrInSprint is returned by a write that would give a task in a sprint a
// backlog state. Its class is ErrBadRequest.
var ErrInSprint = classed(ErrBadRequest, "a task in a sprint cannot take a backlog state; unassign it first")

// ErrSprintClosed is returned by a write that would change a done sprint's
// tasks beyond what it allows: add a task or subtask to it, move a task into
// or out of it, nest or lift its tasks, or leave one of them not done. Its
// class is ErrBadRequest.
var ErrSprintClosed = classed(ErrBadRequest, "reopen the sprint first")

// ErrSprintMembership is returned by a write that would nest tasks of
// different sprints (the backlog counts as one), or give a subtask a sprint of
// its own: a subtask follows its task. Its class is ErrBadRequest.
var ErrSprintMembership = classed(ErrBadRequest, "tasks in different sprints cannot nest")

// ErrDuplicateSprint is returned by a write that would give a sprint a name
// another sprint already holds. Its class is ErrBadRequest.
var ErrDuplicateSprint = classed(ErrBadRequest, "duplicate sprint name")

// ErrSprintHasDone is returned by a write that would remove a sprint that
// still holds done tasks. Its class is ErrBadRequest.
var ErrSprintHasDone = classed(ErrBadRequest, "the sprint holds done tasks; close it or prune them first")

// ErrSprintNotDone matches a SprintNotDoneError with errors.Is.
var ErrSprintNotDone = errors.New("sprint has tasks not done")

// SprintNotDoneError is returned by a write that would close a sprint that has
// tasks not done. Its class is ErrBadRequest.
type SprintNotDoneError struct {
	// Remaining is how many tasks and subtasks are not done.
	Remaining int
}

// Error returns "N tasks not done", or "1 task not done".
func (e *SprintNotDoneError) Error() string {
	if e.Remaining == 1 {
		return "1 task not done"
	}
	return fmt.Sprintf("%d tasks not done", e.Remaining)
}

// Is reports whether target is ErrBadRequest or ErrSprintNotDone.
func (e *SprintNotDoneError) Is(target error) bool {
	return target == ErrBadRequest || target == ErrSprintNotDone
}

// classedError is a specific error that belongs to a class: errors.Is matches
// it against itself (by identity) and against its class.
type classedError struct {
	msg   string
	class error
}

// classed returns a new specific error with message msg in class.
func classed(class error, msg string) error {
	return &classedError{msg: msg, class: class}
}

// Error returns the error's message.
func (e *classedError) Error() string { return e.msg }

// Is reports whether target is e's class.
func (e *classedError) Is(target error) bool { return target == e.class }
