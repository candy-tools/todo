package todo

import (
	"fmt"
	"strconv"
	"strings"
)

// MaxPrefixLen is the longest reference prefix, in characters.
const MaxPrefixLen = 10

// maxRefDigits is the most digits of a reference number nextRef takes into
// account; longer ones are ignored.
const maxRefDigits = 9

// CleanPrefix returns p trimmed and upper-cased, or ErrInvalidPrefix when the
// result is not a letter followed by up to MaxPrefixLen-1 letters or digits
// (A-Z, 0-9). The empty prefix is valid: it means none. Every write that sets a
// reference prefix stores what it returns.
func CleanPrefix(p string) (string, error) {
	p = strings.ToUpper(strings.TrimSpace(p))
	if p == "" {
		return "", nil
	}
	for i := 0; i < len(p); i++ {
		c := p[i]
		if c >= 'A' && c <= 'Z' || i > 0 && c >= '0' && c <= '9' {
			continue
		}
		return "", fmt.Errorf("%w: it must be a letter followed by letters or digits", ErrInvalidPrefix)
	}
	if len(p) > MaxPrefixLen {
		return "", fmt.Errorf("%w: it must be at most %d characters", ErrInvalidPrefix, MaxPrefixLen)
	}
	return p, nil
}

// PrefixFor returns the reference prefix a new top-level task gets when it is
// created under the category parentID in the sprint sprintID ("" for the
// backlog): the sprint's prefix, else that of the nearest category from parentID
// upwards that has one, else the list's. It is "" when none applies, and when
// parentID names no category (a task, or a missing item): subtasks have no
// reference.
func (d *Document) PrefixFor(parentID, sprintID string) string {
	parent, ok := d.ByID(parentID)
	if !ok || parent.Kind != Category {
		return ""
	}
	if s, ok := d.SprintByID(sprintID); ok && s.Prefix != "" {
		return s.Prefix
	}
	for c := parent; c != nil; c = c.Parent {
		if c.Kind == Category && c.Prefix != "" {
			return c.Prefix
		}
	}
	return d.RefPrefix
}

// nextRef returns the next reference for prefix, "<prefix>-<n>" with n padded
// to three digits, and records n as the last number used. n is one above the
// larger of the recorded last number and the highest number of any task
// reference of that shape (ignoring case), so a reference typed by hand is
// never reused, and above that it skips any number whose reference is taken.
func (d *Document) nextRef(prefix string) string {
	last := d.RefLast[prefix]
	head := strings.ToLower(prefix) + "-"
	for _, it := range d.Enumerate() {
		rest, ok := strings.CutPrefix(strings.ToLower(it.Ref), head)
		// A number too long to be a plausible counter is ignored, so a
		// hand-typed one cannot overflow it.
		if !ok || rest == "" || len(rest) > maxRefDigits || strings.Trim(rest, "0123456789") != "" {
			continue
		}
		if m, err := strconv.Atoi(rest); err == nil && m > last {
			last = m
		}
	}
	n := last + 1
	ref := fmt.Sprintf("%s-%03d", prefix, n)
	for {
		if _, taken := d.ByRef(ref); !taken {
			break
		}
		n++
		ref = fmt.Sprintf("%s-%03d", prefix, n)
	}
	if d.RefLast == nil {
		d.RefLast = map[string]int{}
	}
	d.RefLast[prefix] = n
	return ref
}

// SetRefPrefix sets the list's reference prefix, cleaned as CleanPrefix does;
// "" clears it. changed is false when the list already held it. An invalid
// prefix is ErrInvalidPrefix.
func (d *Document) SetRefPrefix(p string) (changed bool, err error) {
	p, err = CleanPrefix(p)
	if err != nil {
		return false, err
	}
	if d.RefPrefix == p {
		return false, nil
	}
	d.RefPrefix = p
	return true, nil
}

// AppendCategoryWith adds a category like AppendCategory, titled f.Title and
// with the reference prefix f.Prefix (cleaned as CleanPrefix does), and returns
// its new ID. An invalid prefix is ErrInvalidPrefix; the other errors are
// AppendCategory's. Nothing is added on an error.
func (d *Document) AppendCategoryWith(f Fields, parentID string) (string, error) {
	prefix, err := CleanPrefix(f.Prefix)
	if err != nil {
		return "", err
	}
	id, err := d.AppendCategory(f.Title, parentID)
	if err != nil {
		return "", err
	}
	if it, ok := d.ByID(id); ok {
		it.Prefix = prefix
	}
	return id, nil
}

// AddSprintWith adds a sprint like AddSprint, named f.Title, with the goal
// f.Description and the reference prefix f.Prefix (cleaned as CleanPrefix
// does), and returns its new ID. An invalid prefix is ErrInvalidPrefix; the
// other errors are AddSprint's. Nothing is added on an error.
func (d *Document) AddSprintWith(f Fields) (string, error) {
	prefix, err := CleanPrefix(f.Prefix)
	if err != nil {
		return "", err
	}
	name, err := d.checkSprintName(f.Title, nil)
	if err != nil {
		return "", err
	}
	id := newID(func(id string) bool { _, ok := d.SprintByID(id); return ok })
	d.Sprints = append(d.Sprints, &Sprint{ID: id, Name: name, Goal: cleanGoal(f.Description), Prefix: prefix})
	return id, nil
}
