package todo

import (
	"errors"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// MaxTitleLen is the longest title, in characters, the store accepts.
const MaxTitleLen = 200

// untitled is the title an import gives an item whose title is empty.
const untitled = "Untitled"

// ErrInvalidTitle is returned by a write that would store an empty title, one
// longer than MaxTitleLen characters, or one holding a control character (a
// newline, tab or escape among them).
var ErrInvalidTitle = errors.New("invalid title")

// CleanTitle returns title trimmed of surrounding whitespace, or
// ErrInvalidTitle when the trimmed title is empty, longer than MaxTitleLen
// characters, or holds a control character. Every store write that sets a
// title stores what it returns.
func CleanTitle(title string) (string, error) {
	t := strings.TrimSpace(title)
	switch {
	case t == "":
		return "", fmt.Errorf("%w: it must not be empty", ErrInvalidTitle)
	case strings.IndexFunc(t, unicode.IsControl) >= 0:
		return "", fmt.Errorf("%w: it must not hold control characters such as newlines, tabs or escapes", ErrInvalidTitle)
	case utf8.RuneCountInString(t) > MaxTitleLen:
		return "", fmt.Errorf("%w: it must be at most %d characters", ErrInvalidTitle, MaxTitleLen)
	}
	return t, nil
}

// importTitle makes a title the import parsed acceptable to CleanTitle: a tab
// becomes a space, other control characters are removed, the result is trimmed
// and cut to MaxTitleLen characters, and an empty title becomes untitled. It
// calls note with the reason for each change it makes beyond trimming.
func importTitle(raw string, note func(string)) string {
	t := strings.TrimSpace(raw)
	if strings.IndexFunc(t, unicode.IsControl) >= 0 {
		t = strings.TrimSpace(Printable(t))
		note("control characters removed from the title")
	}
	if utf8.RuneCountInString(t) > MaxTitleLen {
		t = strings.TrimSpace(string([]rune(t)[:MaxTitleLen]))
		note(fmt.Sprintf("title cut to %d characters", MaxTitleLen))
	}
	if t == "" {
		t = untitled
		note(fmt.Sprintf("empty title replaced with %q", untitled))
	}
	return t
}

// Printable returns s made safe to write to a terminal on one line: a tab
// becomes a space and every other control character (C0, DEL and C1, escape
// among them) is removed, so a title stored before CleanTitle rejected them
// cannot inject terminal escapes. It is for display only; stored data is left
// as it is.
func Printable(s string) string {
	if strings.IndexFunc(s, unicode.IsControl) < 0 {
		return s
	}
	return strings.Map(func(r rune) rune {
		switch {
		case r == '\t':
			return ' '
		case unicode.IsControl(r):
			return -1
		}
		return r
	}, s)
}
