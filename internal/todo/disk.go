package todo

import (
	"bytes"
	"encoding/json"
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

// UnmarshalJSON decodes strictly: unknown fields fail. A version newer than
// fileVersion is reported as ErrNewerSchema before anything else, since a newer
// file may hold fields this binary does not know.
func (l *diskList) UnmarshalJSON(b []byte) error {
	var peek struct {
		Version int `json:"version"`
	}
	if err := json.Unmarshal(b, &peek); err != nil {
		return err
	}
	if peek.Version > fileVersion {
		return newerSchemaError(peek.Version)
	}
	type plain diskList
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	var p plain
	if err := dec.Decode(&p); err != nil {
		return err
	}
	*l = diskList(p)
	return nil
}
