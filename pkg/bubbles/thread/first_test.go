package thread

import (
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestSetFirstShowsWithoutFetch(t *testing.T) {
	src := newSource(1, 3)
	m := newTest(src, nil, 60, 40)
	items := slices.Clone(src.chunks[0])
	m.SetFirst(items, "")
	// The caller's slice is its own.
	items[0].author = "changed"

	cmd := m.SetDocument("  Title", testBody)
	view := ansi.Strip(m.View())
	for _, want := range []string{"@user0", "@user2"} {
		if !strings.Contains(view, want) {
			t.Errorf("view lacks %q before any fetch:\n%s", want, view)
		}
	}
	if strings.Contains(view, "Loading") || strings.Contains(view, "@changed") {
		t.Errorf("view shows loading or the caller's later change:\n%s", view)
	}
	m = drain(t, m, cmd)
	if calls := src.calls(); len(calls) != 0 {
		t.Errorf("fetched %q, want no fetch: the only chunk is set", calls)
	}

	// Reload revalidates the chunk that was set.
	m = drain(t, m, m.Reload())
	if calls := src.calls(); !slices.Equal(calls, []string{""}) {
		t.Errorf("reload fetched %q, want the first chunk", calls)
	}
	if !strings.Contains(ansi.Strip(m.View()), "@user0") {
		t.Error("view lost the comments after the reload")
	}
}

func TestSetFirstLoadsTheRest(t *testing.T) {
	src := newSource(3, 2)
	m := newTest(src, nil, 60, 40)
	m.SetFirst(src.chunks[0], "1")
	m = drain(t, m, m.SetDocument("  Title", "Body."))
	if calls := src.calls(); !slices.Equal(calls, []string{"1", "2"}) {
		t.Errorf("fetched %q, want the chunks after the first", calls)
	}
}

func TestSetFirstAfterFetchDoesNothing(t *testing.T) {
	src := newSource(1, 2)
	m := loaded(t, src, nil, 60, 40)
	before := m.TotalLines()
	m.SetFirst([]comment{{author: "late", body: "Too late."}}, "")
	if m.TotalLines() != before || strings.Contains(ansi.Strip(m.View()), "@late") {
		t.Error("SetFirst changed a thread whose first chunk was fetched")
	}
}
