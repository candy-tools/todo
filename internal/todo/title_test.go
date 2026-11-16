package todo

import (
	"context"
	"strings"
	"testing"
)

// badTitles are titles every store write must refuse with ErrInvalidTitle.
var badTitles = map[string]string{
	"empty":     "",
	"blank":     "   ",
	"newline":   "a\nb",
	"return":    "a\rb",
	"tab":       "a\tb",
	"osc":       "\x1b]0;pwned\x07",
	"csi":       "a\x1b[31mred",
	"c1":        "a\u009bb",
	"overlong":  strings.Repeat("x", MaxTitleLen+1),
	"overlong2": strings.Repeat("é", MaxTitleLen+1),
	"rlo":       "abc\u202edef",
	"lre":       "abc\u202adef",
	"lri":       "abc\u2066def",
	"pdi":       "abc\u2069def",
	"lineSep":   "a\u2028b",
	"paraSep":   "a\u2029b",
	"bom":       "a\ufeffb",
}

// TestCleanTitleKeepsJoinersAndSoftHyphen checks the format characters that
// legitimate titles use (ZWJ emoji sequences, ZWNJ, soft hyphen) are accepted.
func TestCleanTitleKeepsJoinersAndSoftHyphen(t *testing.T) {
	for _, title := range []string{"👨\u200d👩\u200d👧", "می\u200cخواهم", "co\u00adop"} {
		got, err := CleanTitle(title)
		if err != nil || got != title {
			t.Errorf("CleanTitle(%q) = %q, %v; want it unchanged", title, got, err)
		}
	}
}

func TestAddCategoryRejectsInvalidTitles(t *testing.T) {
	s := newStore(t)
	for name, title := range badTitles {
		t.Run(name, func(t *testing.T) {
			rejects(t, s, ErrInvalidTitle, func() error {
				_, _, err := s.AddCategory(context.Background(), title, "", "")
				return err
			})
		})
	}
}

func TestAddTaskRejectsInvalidTitles(t *testing.T) {
	s := newStore(t)
	cat := mustID(t)(s.AddCategory(context.Background(), "C", "", ""))
	for name, title := range badTitles {
		t.Run(name, func(t *testing.T) {
			rejects(t, s, ErrInvalidTitle, func() error {
				_, _, err := s.AddTask(context.Background(), Fields{Title: title}, Open, cat, "")
				return err
			})
		})
	}
}

func TestEditRejectsInvalidTitles(t *testing.T) {
	s := newStore(t)
	ids := addSample(t, s)
	for name, title := range badTitles {
		t.Run(name, func(t *testing.T) {
			rejects(t, s, ErrInvalidTitle, func() error {
				return errOf(s.Edit(context.Background(), ids.t1, Changes{Title: &title}))
			})
		})
	}
	if got := item(t, load(t, s), ids.t1).Title; got != "t1" {
		t.Fatalf("title after rejected edits = %q, want t1", got)
	}
}

func TestImportRejectsInvalidTitles(t *testing.T) {
	s := newStore(t)
	doc := &Document{}
	cat := &Item{Kind: Category, Level: 1, Title: "C"}
	doc.AppendRoot(cat)
	cat.AppendChild(&Item{Kind: Task, Title: "a\x1b]0;x\x07"})
	rejects(t, s, ErrInvalidTitle, func() error {
		return errOf(s.Import(context.Background(), doc, false))
	})
}

func TestTitleWritesTrimAndAcceptTheLimit(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	long := strings.Repeat("é", MaxTitleLen)
	cat := mustID(t)(s.AddCategory(ctx, "  C  ", "", ""))
	task := mustID(t)(s.AddTask(ctx, Fields{Title: long}, Open, cat, ""))
	renamed := " t "
	mustOK(t)(s.Edit(ctx, task, Changes{Title: &renamed}))
	doc := load(t, s)
	if got := item(t, doc, cat).Title; got != "C" {
		t.Errorf("category title = %q, want trimmed C", got)
	}
	if got := item(t, doc, task).Title; got != "t" {
		t.Errorf("task title = %q, want trimmed t", got)
	}
}

func TestImportMarkdownSanitizesTitles(t *testing.T) {
	src := "# A\r\n" +
		"- [ ] t\r\n" +
		"- [ ] bad\x1b]0;pwned\x07\n" +
		"- [ ] tab\there\n" +
		"- [ ] rl\u202eo\u2066x\u2069\u2028y\u2029\ufeff 👨\u200d👩\u200d👧\n" +
		"- [ ] " + strings.Repeat("x", MaxTitleLen+5) + "\n" +
		"- [ ] \n" +
		"#  \n"
	doc, rep, err := ImportMarkdown([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Roots) != 2 {
		t.Fatalf("roots = %d, want 2", len(doc.Roots))
	}
	got := titles(doc.Roots[0].Children)
	want := []string{"t", "bad]0;pwned", "tab here", "rloxy 👨\u200d👩\u200d👧", strings.Repeat("x", MaxTitleLen), untitled}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("task titles = %q, want %q", got, want)
	}
	if doc.Roots[0].Title != "A" || doc.Roots[1].Title != untitled {
		t.Errorf("category titles = %q, %q; want A, %q", doc.Roots[0].Title, doc.Roots[1].Title, untitled)
	}
	wantWarn := []string{
		"line 3: control characters removed from the title",
		"line 4: control characters removed from the title",
		"line 5: control characters removed from the title",
		"line 6: title cut to 200 characters",
		`line 7: empty title replaced with "Untitled"`,
		`line 8: empty title replaced with "Untitled"`,
	}
	if strings.Join(rep.Warnings, "\n") != strings.Join(wantWarn, "\n") {
		t.Errorf("warnings = %q\nwant %q", rep.Warnings, wantWarn)
	}
	// The sanitized document imports.
	if _, err := newStore(t).Import(context.Background(), doc, false); err != nil {
		t.Fatalf("Import of a sanitized document: %v", err)
	}
}

func TestPrintable(t *testing.T) {
	for in, want := range map[string]string{
		"plain":                          "plain",
		"a\tb":                           "a b",
		"x\x1b[31my\x1b]0;t\x07":         "x[31my]0;t",
		"c1\u009bz\x7f":                  "c1z",
		"line\r\nnext":                   "linenext",
		"ünï ✓":                          "ünï ✓",
		"abc\u202edef\u202a\u2066\u2069": "abcdef",
		"p\u2028q\u2029r\ufeff":          "pqr",
		"👨\u200d👩\u200d👧 co\u00adop":     "👨\u200d👩\u200d👧 co\u00adop",
	} {
		if got := Printable(in); got != want {
			t.Errorf("Printable(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPrintableLines(t *testing.T) {
	for in, want := range map[string]string{
		"plain\nlines":                      "plain\nlines",
		"":                                  "",
		"crlf\r\nends\r\n":                  "crlf\nends\n",
		"\tlead\n  \tmixed\nab\tc":          "    lead\n    mixed\nab  c",
		"x\x1b[31my\x07\n\x7fz\u009b":       "x[31my\nz",
		"abc\u202edef\u202a\n\u2066q\u2069": "abcdef\nq",
		"p\u2028q\u2029r\ufeff\n\n":         "pqr\n\n",
		"lone\rcr":                          "lonecr",
		"👨\u200d👩\u200d👧\tco\u00adop":       "👨\u200d👩\u200d👧   co\u00adop",
	} {
		if got := PrintableLines(in); got != want {
			t.Errorf("PrintableLines(%q) = %q, want %q", in, got, want)
		}
	}
}
