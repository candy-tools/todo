package todotest_test

import (
	"testing"

	"github.com/candy-tools/todo/internal/todo/todotest"
)

func TestSeedDumpRoundTrip(t *testing.T) {
	md := "# A\n\n- [/] one\n  note\n  - [ ] sub\n\n## B\n\n- [>] two\n"
	if got := todotest.Dump(t, todotest.Seed(t, md)); got != md {
		t.Fatalf("got:\n%s\nwant:\n%s", got, md)
	}
}
