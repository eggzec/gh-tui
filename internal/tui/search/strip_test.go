package search

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/pkg/bubbles/panes"
)

// A digit shows its kind and focuses the results; the digit of the kind in
// view only focuses them.
func TestKindStripDigits(t *testing.T) {
	t.Parallel()
	s := newSection(t, newFake(), 120, 30)
	s.Focus()
	press(t, s, "2")
	if s.kind != core.SearchIssues || s.area != resultsArea {
		t.Fatalf("after 2: kind %v, area %v, want issues in the results", s.kind, s.area)
	}
	press(t, s, "/")
	press(t, s, "esc")
	press(t, s, "2")
	if s.kind != core.SearchIssues || s.area != resultsArea {
		t.Errorf("after 2 on issues: kind %v, area %v, want the results on issues", s.kind, s.area)
	}
	press(t, s, "1")
	if s.kind != core.SearchRepos {
		t.Errorf("after 1: kind %v, want repositories", s.kind)
	}
}

// Only repositories, issues and pull requests show a count, and only the
// one of the kind in view.
func TestCountOnlyForCountedKinds(t *testing.T) {
	t.Parallel()
	s := newSection(t, newFake(), 120, 30)
	s.text = "go"
	s.counts = map[core.SearchKind]int{core.SearchRepos: 1234, core.SearchIssues: 5, core.SearchCode: 9}
	s.kind = core.SearchRepos
	if got := s.countForms(); !slices.Equal(got, []string{"1.2k"}) {
		t.Errorf("repositories count %q, want [1.2k]", got)
	}
	s.kind = core.SearchIssues
	if got := s.countForms(); !slices.Equal(got, []string{"5"}) {
		t.Errorf("issues count %q, want [5]", got)
	}
	s.kind = core.SearchCode
	if got := s.countForms(); got != nil {
		t.Errorf("code count %q, want none", got)
	}
}

// The count is the total the search reported, with the loaded count in
// brackets while pages remain.
func TestCountFormsOfTotals(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		total int
		want  []string
	}{
		{0, []string{"0"}},
		{1, []string{"1"}},
		{999, []string{"999"}},
		{1000, []string{"1k"}},
		{12345, []string{"12k"}},
	} {
		s := newSection(t, newFake(), 120, 30)
		s.text = "go"
		s.kind = core.SearchRepos
		s.counts = map[core.SearchKind]int{core.SearchRepos: tc.total}
		got := s.countForms()
		if !slices.Equal(got, tc.want) {
			t.Errorf("total %d: forms %q, want %q", tc.total, got, tc.want)
		}
	}
}

// A quick filter on the results on view shows the rows it keeps over the
// rows known, then the total with the loaded count beside it.
func TestCountFormsUnderAQuickFilter(t *testing.T) {
	svc := newFake()
	s := newSection(t, svc, 120, 30)
	typeText(t, s, "tea")
	if got := s.countForms(); !slices.Equal(got, []string{"43 [30+]", "43"}) {
		t.Fatalf("forms %q before the filter, want [43 [30+] 43]", got)
	}
	typed := strings.Split(svc.repos[0].Repo.Ref.String(), "")
	keys := append([]string{"&"}, typed...)
	press(t, s, append(keys, "enter")...)
	l, _ := s.visibleHits()
	shown := l.feed.Shown()
	if shown == 0 || shown >= l.feed.Loaded() {
		t.Fatalf("the filter keeps %d of %d rows, want some but not all", shown, l.feed.Loaded())
	}
	want := fmt.Sprintf("%d/43 [30+]", shown)
	if got := s.countForms(); len(got) == 0 || got[0] != want {
		t.Errorf("forms %q under the filter, want %q first", got, want)
	}
}

// There is no count while the results on view load or failed to load.
func TestCountFormsWhileLoading(t *testing.T) {
	t.Parallel()
	s := newSection(t, newFake(), 120, 30)
	s.text = "go"
	s.kind = core.SearchIssues
	s.counts = map[core.SearchKind]int{core.SearchIssues: 2}
	s.hits[core.SearchIssues] = &hitList{text: "go", kind: core.SearchIssues}
	if l, ok := s.visibleHits(); !ok || l.feed.Len() != 0 {
		t.Fatal("want a visible feed with nothing loaded")
	}
	if got := s.countForms(); got != nil {
		t.Errorf("forms %q while the list loads, want none", got)
	}
}

// The narrow results pane keeps the total alone, and a total below what is
// loaded, which the search can't know, shows the loaded count alone.
func TestCountFormsNarrowAndUnknown(t *testing.T) {
	s := newSection(t, newFake(), 120, 30)
	typeText(t, s, "tea")
	if got := s.countForms(); !slices.Equal(got, []string{"43 [30+]", "43"}) {
		t.Fatalf("forms %q, want [43 [30+] 43]", got)
	}
	s.counts[core.SearchRepos] = 3
	if got := s.countForms(); !slices.Equal(got, []string{"30+"}) {
		t.Errorf("forms %q for a total below the loaded count, want [30+]", got)
	}
}

// The strip lists the kinds it is given, so a kind added to the table
// shows up in it.
func TestKindStripDataDriven(t *testing.T) {
	t.Parallel()
	var plain panes.Styles
	as := kindStripForms(plain, []core.SearchKind{core.SearchRepos, core.SearchIssues}, []string{"1", "2"}, core.SearchRepos)
	bs := kindStripForms(plain, []core.SearchKind{core.SearchRepos, core.SearchIssues, core.SearchPulls}, []string{"1", "2", "3"}, core.SearchRepos)
	a, b := ansi.Strip(as[0]), ansi.Strip(bs[0])
	if strings.Contains(a, "3") || !strings.Contains(b, "3") || !strings.Contains(b, "Pull") {
		t.Errorf("strips %q and %q: the extra kind should show only in the second", a, b)
	}
}
