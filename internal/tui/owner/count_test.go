package owner

import (
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/filterform"
)

func TestRepoCountForms(t *testing.T) {
	t.Parallel()
	s := newSection(t, newFake(), "octocat", 120, 40)

	forms := s.listCount()
	if len(forms) < 2 {
		t.Fatalf("forms = %q, want a long and a bare form", forms)
	}
	if bare := forms[len(forms)-1]; strings.ContainsAny(bare, "[/+") {
		t.Errorf("narrowest form = %q, want the bare total", bare)
	}

	run(t, s, s.ApplyFilter(filterform.AppliedMsg{Query: "language:go sort:stars-desc"}))
	if forms = s.listCount(); len(forms) == 0 || !strings.Contains(forms[0], "/") {
		t.Errorf("filtered forms = %q, want matched/all first", forms)
	}
}

// A fully read organizations tab has no total of its own: what it loaded is
// the total, not "?".
func TestOrgsCountExact(t *testing.T) {
	t.Parallel()
	svc := newFake()
	want := len(svc.people["octocat orgs"])
	s := newSection(t, svc, "octocat", 120, 40)
	for range 4 {
		press(t, s, "]")
	}
	if s.page.tab != orgsTab {
		t.Fatalf("tab = %v, want orgs", s.page.tab)
	}
	n := strconv.Itoa(want)
	if got, wantForms := s.listCount(), []string{n + "[" + n + "]", n}; !slices.Equal(got, wantForms) {
		t.Errorf("orgs forms = %q, want %q", got, wantForms)
	}
}

func TestRepoCountNoneWhileLoading(t *testing.T) {
	t.Parallel()
	s := New(t.Context(), newFake(), config.Default().Keys, WithNow(func() time.Time { return now }))
	setSize(s, 80, 24)
	s.Update(ui.OwnerMsg{Login: "octocat"})
	if forms := s.listCount(); len(forms) != 0 {
		t.Errorf("while loading, forms = %q", forms)
	}
}

// The list pane's top edge shows the count at its right.
func TestListPaneBorderCount(t *testing.T) {
	t.Parallel()
	s := newSection(t, newFake(), "octocat", 120, 40)
	top := ansi.Strip(s.frames[listPane][0])
	if !strings.Contains(top, "[") {
		t.Errorf("top edge %q lacks the count's [loaded]", top)
	}
}

// The tabs line is boxed, three lines of the width asked, and shows the
// language at its right while there is room.
func TestTabsLineBoxed(t *testing.T) {
	t.Parallel()
	s := newSection(t, newFake(), "octocat", 120, 40)
	l, ok := s.page.list().(*repoList)
	if !ok {
		t.Skip("repositories list not on view")
	}
	lang := ansi.Strip(s.language(&l.tableTab))
	for _, w := range []int{120, 60, 30} {
		lines := strings.Split(s.tabsLine(w), "\n")
		if len(lines) != 3 {
			t.Fatalf("width %d: %d lines, want 3", w, len(lines))
		}
		for _, ln := range lines {
			if got := ansi.StringWidth(ln); got != w {
				t.Errorf("width %d: line %q is %d wide", w, ansi.Strip(ln), got)
			}
		}
	}
	if lang != "" && !strings.Contains(ansi.Strip(s.tabsLine(120)), lang) {
		t.Errorf("wide tabs lack the language %q", lang)
	}
}
