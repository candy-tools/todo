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
// newline, tab or escape among them) or another unsafe character (see
// unsafeRune).
var ErrInvalidTitle = errors.New("invalid title")

// CleanTitle returns title trimmed of surrounding whitespace, or
// ErrInvalidTitle when the trimmed title is empty, longer than MaxTitleLen
// characters, or holds a control or other unsafe character (see unsafeRune).
// Every store write that sets a title stores what it returns.
func CleanTitle(title string) (string, error) {
	t := strings.TrimSpace(title)
	switch {
	case t == "":
		return "", fmt.Errorf("%w: it must not be empty", ErrInvalidTitle)
	case strings.IndexFunc(t, unsafeRune) >= 0:
		return "", fmt.Errorf("%w: it must not hold control characters such as newlines, tabs, escapes or bidi overrides", ErrInvalidTitle)
	case utf8.RuneCountInString(t) > MaxTitleLen:
		return "", fmt.Errorf("%w: it must be at most %d characters", ErrInvalidTitle, MaxTitleLen)
	}
	return t, nil
}

// unsafeRune reports whether r must not appear in a title: a control
// character (C0, DEL, C1), a bidi embedding, override or isolate control
// (U+202A-U+202E, U+2066-U+2069), which reorders the rest of a terminal row
// and can disguise text, a line or paragraph separator (U+2028, U+2029), which
// breaks the row in some terminals, or a byte order mark (U+FEFF). The rest of
// category Cf stays allowed on purpose: it holds the zero width joiner of
// emoji sequences, the zero width non-joiner and the soft hyphen, which
// legitimate titles use.
func unsafeRune(r rune) bool {
	switch {
	case unicode.IsControl(r):
		return true
	case r >= '\u202a' && r <= '\u202e', r >= '\u2066' && r <= '\u2069':
		return true
	case r == '\u2028', r == '\u2029', r == '\ufeff':
		return true
	}
	return false
}

// importTitle makes a title the import parsed acceptable to CleanTitle: a tab
// becomes a space, other control and unsafe characters are removed, the result is trimmed
// and cut to MaxTitleLen characters, and an empty title becomes untitled. It
// calls note with the reason for each change it makes beyond trimming.
func importTitle(raw string, note func(string)) string {
	t := strings.TrimSpace(raw)
	if strings.IndexFunc(t, unsafeRune) >= 0 {
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
// among them) and unsafe character (see unsafeRune) is removed, so a title
// stored before CleanTitle rejected them cannot inject terminal escapes or
// reorder the row. It is for display only; stored data is left
// as it is.
func Printable(s string) string {
	if strings.IndexFunc(s, unsafeRune) < 0 {
		return s
	}
	return strings.Map(func(r rune) rune {
		switch {
		case r == '\t':
			return ' '
		case unsafeRune(r):
			return -1
		}
		return r
	}, s)
}

// PrintableLines is Printable for multi-line text such as a description: it
// keeps the newlines, drops a carriage return ending a line (a CRLF line end),
// expands each tab to the next multiple of tabStop columns (counted in runes),
// so indentation keeps its width, and removes every other control and unsafe
// character from each line. It is for display only; stored data is left as it
// is.
func PrintableLines(s string) string {
	if strings.IndexFunc(s, func(r rune) bool { return r != '\n' && unsafeRune(r) }) < 0 {
		return s
	}
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = printableLine(strings.TrimSuffix(line, "\r"))
	}
	return strings.Join(lines, "\n")
}

// printableLine is one line of PrintableLines.
func printableLine(line string) string {
	var b strings.Builder
	col := 0
	for _, r := range line {
		switch {
		case r == '\t':
			n := tabStop - col%tabStop
			b.WriteString(strings.Repeat(" ", n))
			col += n
		case unsafeRune(r):
		default:
			b.WriteRune(r)
			col++
		}
	}
	return b.String()
}
