package search

import (
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
	if got := s.countLabel(); got != "1,234 results" {
		t.Errorf("repositories count %q, want %q", got, "1,234 results")
	}
	s.kind = core.SearchIssues
	if got := s.countLabel(); got != "5 results" {
		t.Errorf("issues count %q, want %q", got, "5 results")
	}
	s.kind = core.SearchCode
	if got := s.countLabel(); got != "" {
		t.Errorf("code count %q, want none", got)
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
