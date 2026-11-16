package todo_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/candy-tools/todo/internal/todo"
)

// Every specific error belongs to exactly one class, so a caller can map a
// class without listing its members; the specific error still matches itself,
// wrapped or not.
func TestErrorClasses(t *testing.T) {
	classes := []error{
		todo.ErrBadRequest, todo.ErrNotFound, todo.ErrWrongKind, todo.ErrConflict,
		todo.ErrInvalidList, todo.ErrListMissing, todo.ErrInternal,
	}
	for _, tc := range []struct {
		err, class error
	}{
		{todo.ErrTooDeep, todo.ErrBadRequest},
		{todo.ErrSubtaskStatus, todo.ErrBadRequest},
		{todo.ErrInvalidMove, todo.ErrBadRequest},
		{todo.ErrInvalidTitle, todo.ErrBadRequest},
		{todo.ErrRestoreAndCascade, todo.ErrBadRequest},
		{todo.ErrNotFound, todo.ErrNotFound},
		{todo.ErrWrongParent, todo.ErrWrongKind},
		{todo.ErrNotTask, todo.ErrWrongKind},
		{todo.ErrConflict, todo.ErrConflict},
		{todo.ErrBusy, todo.ErrConflict},
		{todo.ErrInvalidList, todo.ErrInvalidList},
		{todo.ErrNewerSchema, todo.ErrInvalidList},
		{todo.ErrListMissing, todo.ErrListMissing},
	} {
		for _, err := range []error{tc.err, fmt.Errorf("context: %w", tc.err)} {
			if !errors.Is(err, tc.err) {
				t.Errorf("%q does not match itself", err)
			}
			for _, class := range classes {
				if got, want := errors.Is(err, class), class == tc.class; got != want {
					t.Errorf("errors.Is(%q, %q) = %v, want %v", err, class, got, want)
				}
			}
		}
	}
}
