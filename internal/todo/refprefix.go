package todo

import (
	"fmt"
	"path/filepath"
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
// created under the category parentID: that of the nearest category from
// parentID upwards that has one, else the list's. It is "" when none applies,
// and when parentID names no category (a task, or a missing item): subtasks
// have no reference. The sprint a task is created in plays no part.
func (d *Document) PrefixFor(parentID string) string {
	parent, ok := d.ByID(parentID)
	if !ok || parent.Kind != Category {
		return ""
	}
	for c := parent; c != nil; c = c.Parent {
		if c.Kind == Category && c.Prefix != "" {
			return c.Prefix
		}
	}
	return d.RefPrefix
}

// defaultSprintPrefix is the sprint reference prefix of a list with no
// SprintPrefix setting and no FolderPrefix.
const defaultSprintPrefix = "SPRINT"

// SprintRefPrefix returns the reference prefix a new sprint gets its reference
// from: the SprintPrefix setting, else FolderPrefix (defaultSprintPrefix when
// that is ""). A fallback that is also a task prefix (the list's or a
// category's) gets an "S" appended (TODO becomes TODOS), else is
// defaultSprintPrefix, so a reference never names both a task and a sprint.
func (d *Document) SprintRefPrefix() string {
	if d.SprintPrefix != "" {
		return d.SprintPrefix
	}
	base := d.FolderPrefix
	if base == "" {
		base = defaultSprintPrefix
	}
	taken := d.taskPrefixes()
	for _, p := range []string{base, withS(base), defaultSprintPrefix, withS(defaultSprintPrefix)} {
		if !taken[p] {
			return p
		}
	}
	return withS(defaultSprintPrefix)
}

// withS returns p with an "S" appended, replacing its last character when p is
// already MaxPrefixLen long.
func withS(p string) string {
	if len(p) >= MaxPrefixLen {
		p = p[:MaxPrefixLen-1]
	}
	return p + "S"
}

// taskPrefixes returns the task reference prefixes of d: the list's and every
// category's.
func (d *Document) taskPrefixes() map[string]bool {
	out := map[string]bool{}
	if d.RefPrefix != "" {
		out[d.RefPrefix] = true
	}
	for _, it := range d.Enumerate() {
		if it.Kind == Category && it.Prefix != "" {
			out[it.Prefix] = true
		}
	}
	return out
}

// checkTaskPrefix refuses with ErrPrefixClash the cleaned task prefix p when it
// is the SprintPrefix setting; "" always fits.
func (d *Document) checkTaskPrefix(p string) error {
	if p != "" && p == d.SprintPrefix {
		return fmt.Errorf("%w: %s is the sprint reference prefix", ErrPrefixClash, p)
	}
	return nil
}

// FolderPrefix returns the sprint reference prefix made from the name of the
// folder that holds the file at path: its ASCII letters and digits, upper
// case, without leading digits, cut to MaxPrefixLen ("candy-tools" gives
// CANDYTOOLS). It is "" when nothing is left, or the folder is the root.
func FolderPrefix(path string) string {
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	var b strings.Builder
	for _, r := range strings.ToUpper(filepath.Base(filepath.Dir(path))) {
		switch {
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r >= '0' && r <= '9' && b.Len() > 0:
			b.WriteRune(r)
		}
		if b.Len() == MaxPrefixLen {
			break
		}
	}
	return b.String()
}

// refNumber returns the number of ref when it has the shape "<head><n>",
// compared ignoring case (head is lower case), with n at most maxRefDigits
// digits: a longer one is not a plausible counter, so a hand-typed one cannot
// overflow it.
func refNumber(ref, head string) (int, bool) {
	rest, ok := strings.CutPrefix(strings.ToLower(ref), head)
	if !ok || rest == "" || len(rest) > maxRefDigits || strings.Trim(rest, "0123456789") != "" {
		return 0, false
	}
	n, err := strconv.Atoi(rest)
	return n, err == nil
}

// refTaken reports whether a task or a sprint holds ref, ignoring case.
func (d *Document) refTaken(ref string) bool {
	if _, ok := d.ByRef(ref); ok {
		return true
	}
	_, ok := d.sprintByRef(ref)
	return ok
}

// nextRef returns the next reference for prefix, "<prefix>-<n>" with n padded
// to three digits, and records n as the last number used. n is one above the
// larger of the recorded last number and the highest number of any task or
// sprint reference of that shape (ignoring case), so a reference typed by hand
// is never reused, and above that it skips any number whose reference is
// taken. Tasks and sprints share the count of a prefix.
func (d *Document) nextRef(prefix string) string {
	last := d.RefLast[prefix]
	head := strings.ToLower(prefix) + "-"
	refs := []string{}
	for _, it := range d.Enumerate() {
		refs = append(refs, it.Ref)
	}
	for _, s := range d.Sprints {
		refs = append(refs, s.Ref)
	}
	for _, r := range refs {
		if m, ok := refNumber(r, head); ok && m > last {
			last = m
		}
	}
	n := last + 1
	ref := fmt.Sprintf("%s-%03d", prefix, n)
	for d.refTaken(ref) {
		n++
		ref = fmt.Sprintf("%s-%03d", prefix, n)
	}
	if d.RefLast == nil {
		d.RefLast = map[string]int{}
	}
	d.RefLast[prefix] = n
	return ref
}

// Settings are the list settings an UpdateSettings sets. A nil field is kept;
// "" clears one.
type Settings struct {
	// RefPrefix is the list's task reference prefix (see PrefixFor).
	RefPrefix *string
	// SprintPrefix is the sprints' reference prefix (see SprintRefPrefix).
	SprintPrefix *string
}

// UpdateSettings sets the settings s names, each cleaned as CleanPrefix does.
// changed is false when the list already held them. An invalid prefix is
// ErrInvalidPrefix; a sprint prefix that is, after the update, the list's or a
// category's prefix is ErrPrefixClash. Both are checked together, so the two
// prefixes may be swapped in one update. Nothing changes on an error.
func (d *Document) UpdateSettings(s Settings) (changed bool, err error) {
	ref, sprint := d.RefPrefix, d.SprintPrefix
	if s.RefPrefix != nil {
		if ref, err = CleanPrefix(*s.RefPrefix); err != nil {
			return false, err
		}
	}
	if s.SprintPrefix != nil {
		if sprint, err = CleanPrefix(*s.SprintPrefix); err != nil {
			return false, err
		}
	}
	if sprint != "" {
		if sprint == ref {
			return false, fmt.Errorf("%w: %s would be both the list's and the sprint reference prefix", ErrPrefixClash, sprint)
		}
		for _, it := range d.Enumerate() {
			if it.Kind == Category && it.Prefix == sprint {
				return false, fmt.Errorf("%w: %s is the reference prefix of category %q", ErrPrefixClash, sprint, it.Title)
			}
		}
	}
	changed = ref != d.RefPrefix || sprint != d.SprintPrefix
	d.RefPrefix, d.SprintPrefix = ref, sprint
	return changed, nil
}

// SetRefPrefix sets the list's reference prefix, cleaned as CleanPrefix does;
// "" clears it. changed is false when the list already held it. An invalid
// prefix is ErrInvalidPrefix, and the SprintPrefix setting ErrPrefixClash.
func (d *Document) SetRefPrefix(p string) (changed bool, err error) {
	return d.UpdateSettings(Settings{RefPrefix: &p})
}

// AppendCategoryWith adds a category like AppendCategory, titled f.Title and
// with the reference prefix f.Prefix (cleaned as CleanPrefix does), and returns
// its new ID. An invalid prefix is ErrInvalidPrefix, and the SprintPrefix
// setting ErrPrefixClash; the other errors are AppendCategory's. Nothing is
// added on an error.
func (d *Document) AppendCategoryWith(f Fields, parentID string) (string, error) {
	prefix, err := CleanPrefix(f.Prefix)
	if err != nil {
		return "", err
	}
	if err := d.checkTaskPrefix(prefix); err != nil {
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

// fillSprintRefs gives every sprint without a reference the next one of
// SprintRefPrefix, in creation order, and reports whether any got one. It
// numbers the sprints of a list written before sprints had references.
func (d *Document) fillSprintRefs() bool {
	filled := false
	for _, s := range d.Sprints {
		if s.Ref == "" {
			s.Ref = d.nextRef(d.SprintRefPrefix())
			filled = true
		}
	}
	return filled
}
