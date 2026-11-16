package todo

import "errors"

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
// ErrNewerSchema: a file this todo cannot read.
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

// ErrRestoreAndCascade is returned by a state change asked both to restore the
// subtasks from a snapshot and to cascade to them all: the two disagree. Its
// class is ErrBadRequest.
var ErrRestoreAndCascade = classed(ErrBadRequest, "a reopen cannot both restore the subtasks and open them all")

// ErrWrongParent is returned by a write that would place a task at the root or
// a category under a task: a task always sits under a category or a task, and
// a category under a category or at the root. Its class is ErrWrongKind.
var ErrWrongParent = classed(ErrWrongKind, "a task must sit under a category and a category cannot sit under a task")

// ErrNotTask is returned by a write that would set the status of a category:
// only a task has a status. Its class is ErrWrongKind.
var ErrNotTask = classed(ErrWrongKind, "only a task has a status")

// ErrBusy is returned by a write that gave up waiting for another process to
// release the list's write lock (after the store's busy timeout). Its class
// is ErrConflict.
var ErrBusy = classed(ErrConflict, "list busy (another todo is writing), try again")

// ErrNewerSchema is returned when the list file was written by a newer todo,
// whose format this binary does not know and must not read or write. It is
// wrapped with the version found. Its class is ErrInvalidList.
var ErrNewerSchema = classed(ErrInvalidList, "list was written by a newer todo")

// ErrNotEmpty is returned by Import when the list already holds items and the
// caller did not ask to replace them. It belongs to no class.
var ErrNotEmpty = errors.New("list is not empty")

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
