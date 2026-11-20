package todo

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// MaxRefLen is the longest reference, in characters, the store accepts.
const MaxRefLen = 50

// CleanRef returns ref trimmed of surrounding whitespace, or ErrInvalidRef when
// the trimmed reference is longer than MaxRefLen characters or holds a control
// or other unsafe character (see unsafeRune). The empty reference is valid: it
// means the task has none. Apart from that a reference is free-form text, such
// as "APP-001". Every store write that sets a reference stores what it returns.
func CleanRef(ref string) (string, error) {
	r := strings.TrimSpace(ref)
	switch {
	case strings.IndexFunc(r, unsafeRune) >= 0:
		return "", fmt.Errorf("%w: it must not hold control characters such as newlines, tabs, escapes or bidi overrides", ErrInvalidRef)
	case utf8.RuneCountInString(r) > MaxRefLen:
		return "", fmt.Errorf("%w: it must be at most %d characters", ErrInvalidRef, MaxRefLen)
	}
	return r, nil
}

// refKey is the form two references are compared in: references are unique in
// a list ignoring case, so "app-001" and "APP-001" are the same reference.
func refKey(ref string) string { return strings.ToLower(strings.ToUpper(ref)) }

// ByRef returns the task whose reference equals ref ignoring case, or ok=false
// when none matches. The empty reference means "none", so it never matches.
func (d *Document) ByRef(ref string) (*Item, bool) {
	if ref == "" {
		return nil, false
	}
	key := refKey(ref)
	for _, it := range d.Enumerate() {
		if it.Ref != "" && refKey(it.Ref) == key {
			return it, true
		}
	}
	return nil, false
}

// checkRef reports whether task it may hold the cleaned reference ref: a
// subtask may not hold one (ErrSubtaskRef), and no other task of d may hold
// the same one (ErrDuplicateRef). it is nil for a task not yet added, and
// subtask says whether it is (or becomes) a subtask. The empty reference always
// fits.
func (d *Document) checkRef(it *Item, ref string, subtask bool) error {
	if ref == "" {
		return nil
	}
	if subtask {
		return ErrSubtaskRef
	}
	if s, ok := d.sprintByRef(ref); ok {
		return fmt.Errorf("%w: %q is already used by sprint %q", ErrDuplicateRef, s.Ref, s.Name)
	}
	if other, ok := d.ByRef(ref); ok && other != it {
		return fmt.Errorf("%w: %q is already used by task %q", ErrDuplicateRef, other.Ref, other.Title)
	}
	return nil
}

// DisplayTitle returns the title as it is shown: a task with a reference
// carries it in brackets before the title, as "[APP-001] Ship it".
func (it *Item) DisplayTitle() string {
	if it.Ref == "" {
		return it.Title
	}
	return "[" + it.Ref + "] " + it.Title
}
