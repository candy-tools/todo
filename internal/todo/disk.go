package todo

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"
)

// fileVersion is the version of the JSON list file format.
const fileVersion = 3

// diskList is the JSON form of the whole list. Categories and tasks are flat
// arrays that refer to each other by ID; field order is key order.
type diskList struct {
	Version    int            `json:"version"`
	Categories []diskCategory `json:"categories"`
	Tasks      []diskTask     `json:"tasks"`
}

// diskCategory is a category; Parent is "" for a root category.
type diskCategory struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Parent string `json:"parent,omitempty"`
}

// diskTask is a task with its subtasks inline.
type diskTask struct {
	ID          string        `json:"id"`
	Category    string        `json:"category"`
	Title       string        `json:"title"`
	Status      string        `json:"status"`
	Plan        string        `json:"plan,omitempty"`
	Description string        `json:"description,omitempty"`
	Subtasks    []diskSubtask `json:"subtasks,omitempty"`
}

// diskSubtask is a subtask: only open or done.
type diskSubtask struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Done        bool   `json:"done,omitempty"`
	Description string `json:"description,omitempty"`
	Plan        string `json:"plan,omitempty"`
}

// toDisk converts d to its on-disk form. Categories come in pre-order, so a
// parent precedes its children; tasks come in tree order. A task's subtasks are
// its task children; a category's description and plan are not stored.
func toDisk(d *Document) diskList {
	l := diskList{Version: fileVersion, Categories: []diskCategory{}, Tasks: []diskTask{}}
	for _, it := range d.Enumerate() {
		switch {
		case it.Kind == Category:
			c := diskCategory{ID: it.ID, Title: it.Title}
			if it.Parent != nil {
				c.Parent = it.Parent.ID
			}
			l.Categories = append(l.Categories, c)
		case it.IsSubtask():
			// written under its task
		default:
			t := diskTask{
				ID: it.ID, Title: it.Title, Status: it.Status.String(),
				Plan: it.Plan, Description: it.Description,
			}
			if c := it.EnclosingCategory(); c != nil {
				t.Category = c.ID
			}
			for _, ch := range it.Children {
				if ch.Kind != Task {
					continue
				}
				t.Subtasks = append(t.Subtasks, diskSubtask{
					ID: ch.ID, Title: ch.Title, Done: ch.Status == Done,
					Description: ch.Description, Plan: ch.Plan,
				})
			}
			l.Tasks = append(l.Tasks, t)
		}
	}
	return l
}

// fromDisk builds the Document tree of l, which must have passed validate.
// Categories keep array order under their parent; a category's tasks come
// before its subcategories. Every item gets Parent, Kind, Level and Status.
func fromDisk(l diskList) *Document {
	d := &Document{}
	cats := make(map[string]*Item, len(l.Categories))
	for _, c := range l.Categories {
		cats[c.ID] = &Item{ID: c.ID, Kind: Category, Title: c.Title}
	}
	// Tasks first, so they precede subcategories under their category.
	for _, t := range l.Tasks {
		it := &Item{
			ID: t.ID, Kind: Task, Title: t.Title, Description: t.Description, Plan: t.Plan,
		}
		it.Status, _ = statusFromDisk(t.Status)
		for _, s := range t.Subtasks {
			sub := NewTask(s.Title, s.Description, s.Done)
			sub.ID, sub.Plan = s.ID, s.Plan
			it.AppendChild(sub)
		}
		cats[t.Category].AppendChild(it)
	}
	var setLevel func(it *Item, level int)
	setLevel = func(it *Item, level int) {
		it.Level = level
		for _, ch := range it.Children {
			if ch.Kind == Category {
				setLevel(ch, level+1)
			}
		}
	}
	for _, c := range l.Categories {
		it := cats[c.ID]
		if c.Parent == "" {
			d.AppendRoot(it)
		} else {
			cats[c.Parent].AppendChild(it)
		}
	}
	for _, r := range d.Roots {
		setLevel(r, 1)
	}
	return d
}

// statusFromDisk parses a canonical status name; unlike StatusFromName it does
// not accept the CLI alias "progress".
func statusFromDisk(name string) (Status, bool) {
	for s, n := range statusNames {
		if n == name {
			return s, true
		}
	}
	return Open, false
}

// byteOrderMark is the UTF-8 byte order mark some Windows editors put at the
// start of a file.
var byteOrderMark = []byte("\ufeff")

// decodeList decodes the list file's bytes into l (it is the store's
// jsonfile Decode hook). A leading byte order mark is dropped; a syntax error
// is reported at its line and column; the rest is decode, so positions count
// from the start of the file.
func decodeList(data []byte, l *diskList) error {
	data = bytes.TrimPrefix(data, byteOrderMark)
	var raw json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		var se *json.SyntaxError
		if errors.As(err, &se) {
			return syntaxErr(data, se)
		}
		return err
	}
	return l.decode(data)
}

// syntaxErr reports se at the character it is about: the offending one, or
// just past the end when the input ends too early.
func syntaxErr(data []byte, se *json.SyntaxError) error {
	end := min(int(se.Offset), len(data))
	at := len(data)
	if !endsEarly(data) {
		_, size := utf8.DecodeLastRune(data[:end]) // the offset is just past it
		at = end - size
	}
	return fmt.Errorf("%s: %w", position(data, at), se)
}

// endsEarly reports whether data, which fails to parse, fails because it ends
// too early: one more byte, which no JSON text may hold, moves the error past
// the end. It does not depend on encoding/json's message texts.
func endsEarly(data []byte) bool {
	var raw json.RawMessage
	err := json.Unmarshal(append(slices.Clip(data), 0x01), &raw)
	var se *json.SyntaxError
	return errors.As(err, &se) && se.Offset > int64(len(data))
}

// UnmarshalJSON decodes b as decode does, for a caller of json.Unmarshal.
func (l *diskList) UnmarshalJSON(b []byte) error { return l.decode(b) }

// decode decodes strictly: unknown fields fail. The version is read
// first: a missing or non-integer one fails, and one newer than fileVersion
// is reported as ErrNewerSchema before anything else, since a newer file may
// hold fields this binary does not know. A value of the wrong type is reported
// at its line and column, by its JSON path and the kind expected; an unknown
// field by its name and the item holding it. "Strict" stops at key case:
// encoding/json matches keys to fields case-insensitively, so a hand-edited
// "Version" or "TITLE" is accepted as the field it folds to. That is harmless:
// the value lands in the right field, and the next write rewrites the
// canonical lowercase keys. b must be well-formed JSON.
func (l *diskList) decode(b []byte) error {
	var peek struct {
		Version json.RawMessage `json:"version"`
	}
	if err := json.Unmarshal(b, &peek); err != nil {
		return decodeErr(b, 0, err)
	}
	version, err := parseVersion(peek.Version)
	if err != nil {
		return err
	}
	if version > fileVersion {
		return newerSchemaError(version)
	}
	type plain diskList
	// A Decoder counts its offsets from the start of the value, after any
	// leading white space, so it gets b from there.
	lead := len(b) - len(bytes.TrimLeft(b, " \t\r\n"))
	dec := json.NewDecoder(bytes.NewReader(b[lead:]))
	dec.DisallowUnknownFields()
	var p plain
	if err := dec.Decode(&p); err != nil {
		return decodeErr(b, lead, err)
	}
	*l = diskList(p)
	return nil
}

// parseVersion returns the format version raw (the "version" value) holds,
// failing when it is missing, null or not a whole number.
func parseVersion(raw json.RawMessage) (int, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return 0, fmt.Errorf(`no "version" field; this todo reads format version %d`, fileVersion)
	}
	var v int
	if err := json.Unmarshal(raw, &v); err != nil {
		shown := string(raw)
		if utf8.RuneCountInString(shown) > 20 {
			shown = string([]rune(shown)[:20]) + "…"
		}
		return 0, fmt.Errorf(`"version" must be a whole number, not %s`, shown)
	}
	return v, nil
}

// decodeErr words a decode error of the list file b for a person: a value of
// the wrong type at its line and column, by its JSON path and the kind
// expected, and an unknown field with the item that holds it. shift is added
// to the error's offset (for a Decoder that started past leading white
// space). Other errors pass through.
func decodeErr(b []byte, shift int, err error) error {
	var te *json.UnmarshalTypeError
	if errors.As(err, &te) {
		at := position(b, valueStart(b, shift+int(te.Offset), te.Value))
		got := jsonKind(te.Value)
		if te.Field == "" {
			return fmt.Errorf("%s: the file must hold a JSON object, not %s", at, got)
		}
		return fmt.Errorf("%s: %s must be %s, not %s", at, jsonPath(te.Field), wantKind(te.Type), got)
	}
	if name, ok := strings.CutPrefix(err.Error(), "json: unknown field "); ok {
		if n, uerr := strconv.Unquote(name); uerr == nil {
			return fmt.Errorf("%sunknown field %q", unknownFieldOwner(b, n), n)
		}
	}
	return err
}

// valueStart returns the offset of the first character of the JSON value a
// type error was reported for, from the error's offset end and its value
// description (encoding/json's "string", "number 3.5", "bool", "array", ...).
// For a scalar the offset is just past the value; for an array or object it is
// just past its opening bracket.
func valueStart(b []byte, end int, value string) int {
	end = min(end, len(b))
	kind, _, _ := strings.Cut(value, " ")
	i := end - 1
	switch kind {
	case "string": // back from the closing quote to the unescaped opening one
		for i--; i >= 0; i-- {
			if b[i] == '"' && escapes(b[:i])%2 == 0 {
				break
			}
		}
	case "number":
		for i > 0 && strings.IndexByte("0123456789+-.eE", b[i-1]) >= 0 {
			i--
		}
	case "bool":
		if bytes.HasSuffix(b[:end], []byte("false")) {
			i = end - len("false")
		} else {
			i = end - len("true")
		}
	}
	return max(i, 0)
}

// escapes counts the backslashes that end b.
func escapes(b []byte) int {
	return len(b) - len(bytes.TrimRight(b, `\`))
}

// position returns where byte offset at of data sits, as "line L, column C",
// both counted from 1 and the column in characters.
func position(data []byte, at int) string {
	at = min(at, len(data))
	before := data[:at]
	line := bytes.Count(before, []byte("\n")) + 1
	col := utf8.RuneCount(before[bytes.LastIndexByte(before, '\n')+1:]) + 1
	return fmt.Sprintf("line %d, column %d", line, col)
}

// jsonPath turns encoding/json's dotted field path ("tasks.0.title") into the
// form a person reads in the file: "tasks[0].title".
func jsonPath(field string) string {
	var b strings.Builder
	for i, part := range strings.Split(field, ".") {
		switch {
		case isIndex(part):
			b.WriteString("[" + part + "]")
		case i > 0:
			b.WriteString("." + part)
		default:
			b.WriteString(part)
		}
	}
	return b.String()
}

// isIndex reports whether a path part is an array index.
func isIndex(part string) bool {
	_, err := strconv.Atoi(part)
	return err == nil
}

// jsonKind names the kind of a JSON value as encoding/json describes it
// ("string", "number 3.5", "bool", "array", "object").
func jsonKind(value string) string {
	kind, _, _ := strings.Cut(value, " ")
	switch kind {
	case "bool":
		return "true or false"
	case "array", "object":
		return "an " + kind
	}
	return "a " + kind
}

// wantKind names the JSON kind a Go type is decoded from.
func wantKind(t reflect.Type) string {
	if t == nil {
		return "something else"
	}
	switch t.Kind() {
	case reflect.String:
		return "a string"
	case reflect.Bool:
		return "true or false"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return "a whole number"
	case reflect.Float32, reflect.Float64:
		return "a number"
	case reflect.Slice, reflect.Array:
		return "an array"
	case reflect.Struct, reflect.Map:
		return "an object"
	}
	return "something else"
}

// The fields each object of the file may hold.
var (
	listFields     = []string{"version", "categories", "tasks"}
	categoryFields = []string{"id", "title", "parent"}
	taskFields     = []string{"id", "category", "title", "status", "plan", "description", "subtasks"}
	subtaskFields  = []string{"id", "title", "done", "description", "plan"}
)

// unknownFieldOwner returns the item of the list file b that holds the
// unknown field name, as "task bbbbbbb: " (or by its JSON path when it has no
// valid ID), and "" for the top level or when it cannot tell.
func unknownFieldOwner(b []byte, name string) string {
	type object = map[string]json.RawMessage
	unknown := func(o object, fields []string) bool {
		if _, ok := o[name]; !ok {
			return false
		}
		return !slices.ContainsFunc(fields, func(f string) bool { return strings.EqualFold(f, name) })
	}
	var doc struct {
		Categories []object `json:"categories"`
		Tasks      []object `json:"tasks"`
	}
	var top object
	if json.Unmarshal(b, &top) != nil || unknown(top, listFields) {
		return ""
	}
	_ = json.Unmarshal(b, &doc) // the items it could decode are enough
	for i, c := range doc.Categories {
		if unknown(c, categoryFields) {
			return itemRef("category", fmt.Sprintf("categories[%d]", i), c) + ": "
		}
	}
	for i, t := range doc.Tasks {
		task := itemRef("task", fmt.Sprintf("tasks[%d]", i), t)
		if unknown(t, taskFields) {
			return task + ": "
		}
		var subs []object
		_ = json.Unmarshal(t["subtasks"], &subs)
		for j, s := range subs {
			if !unknown(s, subtaskFields) {
				continue
			}
			if ref := itemRef("subtask", "", s); ref != "" {
				return ref + " of " + task + ": "
			}
			return fmt.Sprintf("tasks[%d].subtasks[%d]: ", i, j)
		}
	}
	return ""
}

// itemRef names an item object o of the file: kind and ID when it has a
// valid one, and path (its JSON path, possibly "") otherwise.
func itemRef(kind, path string, o map[string]json.RawMessage) string {
	var id string
	if json.Unmarshal(o["id"], &id) == nil && validID(id) {
		return kind + " " + id
	}
	return path
}
