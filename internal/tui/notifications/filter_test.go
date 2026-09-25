package notifications

import (
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/pkg/bubbles/filterform"
)

func TestParseFilter(t *testing.T) {
	tests := []struct {
		query string
		want  filter
	}{
		{"", filter{all: true}},
		{"is:unread", filter{}},
		{"is:Unread reason:Mention repo:Cli/Cli type:pr", filter{reason: "mention", repo: "cli/cli", typ: "pr"}},
		{"type:ci label:bug", filter{all: true, typ: "ci"}},
	}
	for _, tt := range tests {
		got := parseFilter(tt.query)
		tt.want.query = tt.query
		if got != tt.want {
			t.Errorf("parseFilter(%q) = %+v, want %+v", tt.query, got, tt.want)
		}
	}
}

func TestFilterKeeps(t *testing.T) {
	tests := []struct {
		query string
		want  []string
	}{
		{"", []string{"1", "2", "3", "4", "5", "6", "7"}},
		{"reason:mention", []string{"2"}},
		{"repo:eggzec/gh-tui", []string{"2", "7"}},
		{"type:pr", []string{"1"}},
		{"type:ci", []string{"6"}},
		{"type:repositoryvulnerabilityalert", []string{"7"}},
		{"type:issue repo:eggzec/gh-tui reason:mention", []string{"2"}},
		{"type:release reason:mention", nil},
	}
	for _, tt := range tests {
		f := parseFilter(tt.query)
		var got []string
		for _, n := range f.apply(inbox()) {
			got = append(got, n.ID)
		}
		if !slices.Equal(got, tt.want) {
			t.Errorf("filter %q keeps %v, want %v", tt.query, got, tt.want)
		}
	}
}

func TestFilterSpec(t *testing.T) {
	s := newSection(t, newFake(inbox()...), 80, 12)
	f, ok := s.Filter()
	if !ok || f.Query != "is:unread" {
		t.Fatalf("Filter = %q, %v; want the unread threads", f.Query, ok)
	}
	for _, q := range []string{
		"is:unread",
		"",
		"is:unread reason:review_requested repo:charmbracelet/bubbletea type:pr",
		"reason:mention type:discussion",
		// Values the form doesn't offer are shown as typed.
		"reason:invitation repo:someone/else",
	} {
		s := newSection(t, newFake(inbox()...), 80, 12)
		press(t, s, showAll+q)
		f, _ := s.Filter()
		if got := filterform.New(f.Spec, filterform.WithQuery(f.Query)).Query(); got != q {
			t.Errorf("the form of %q writes %q", q, got)
		}
	}
}

// TestFilterRepos checks that the repositories offered are those of the
// threads cached, the most recent first, and cost no request.
func TestFilterRepos(t *testing.T) {
	svc := newFake(inbox()...)
	svc.size = 3
	s := newSection(t, svc, 80, 12)
	press(t, s, showAll, "end")
	lists := svc.listCount()
	f, _ := s.Filter()
	want := []string{
		"charmbracelet/bubbletea", "eggzec/gh-tui", "golang/go", "cli/cli", "charmbracelet/lipgloss",
		"a-very-long-organization-name/with-an-even-longer-repository-name",
	}
	repos := make([]string, 0, len(want))
	for _, it := range f.Spec.Fields[2].Options[1:] {
		repos = append(repos, it.Label)
	}
	if !slices.Equal(repos, want) {
		t.Errorf("the filter offers %v, want %v", repos, want)
	}
	if svc.listCount() != lists {
		t.Error("offering the repositories listed the inbox")
	}
}

func TestFilterLists(t *testing.T) {
	svc := newFake(inbox()...)
	s := newSection(t, svc, 80, 12)
	lists := svc.listCount()

	// Unread is GitHub's filter; the reason is the section's, on the same
	// page.
	press(t, s, showAll+"is:unread reason:mention")
	if got := rows(s); !slices.Equal(got, []string{"2"}) {
		t.Errorf("rows = %v, want the mention", got)
	}
	if q := svc.lastList(); q.Filter.All || svc.listCount() != lists+1 {
		t.Errorf("listed %+v in %d requests, want the unread page once", q, svc.listCount()-lists)
	}
	view := ansi.Strip(s.View())
	if !strings.Contains(view, "Unread · mention  f filter · F clear") {
		t.Errorf("the header should name the filter:\n%s", view)
	}

	// Applying it again lists nothing, and F goes back to the unread.
	lists = svc.listCount()
	press(t, s, showAll+"is:unread reason:mention")
	if svc.listCount() != lists {
		t.Error("the same filter listed again")
	}
	press(t, s, "F")
	if got := rows(s); !slices.Equal(got, []string{"1", "2", "3", "6"}) || s.filtered() {
		t.Errorf("rows after F = %v, want the unread", got)
	}
	if strings.Contains(ansi.Strip(s.View()), "F clear") {
		t.Error("the header offers to clear the unread inbox")
	}
	lists = svc.listCount()
	press(t, s, "F")
	if svc.listCount() != lists {
		t.Error("F on the unread inbox listed it again")
	}
}

// TestFilterReadsOnWhileRoom checks that a page the filter leaves empty
// leads to the next, as long as the window has room for more.
func TestFilterReadsOnWhileRoom(t *testing.T) {
	threads := make([]core.Notification, 0, 12)
	for i := range 12 {
		repo := "charmbracelet/bubbletea"
		if i%4 == 3 {
			repo = "cli/cli"
		}
		threads = append(threads, thread(strconv.Itoa(i), repo, core.SubjectIssue, "Thread "+strconv.Itoa(i), "comment", true, 0))
	}
	svc := newFake(threads...)
	svc.size = 2
	s := newSection(t, svc, 80, 12)
	press(t, s, showAll+"is:unread repo:cli/cli")
	if got := s.feed.Len(); got != 3 {
		t.Errorf("the list holds %d threads, want the three of cli/cli", got)
	}
	if !s.feed.Done() {
		t.Error("the list should have read every page to fill its window")
	}
}
