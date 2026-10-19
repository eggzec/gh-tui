package notifications

import (
	"reflect"
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
		{"is:Unread reason:Mention repo:Cli/Cli type:pr", filter{reasons: []string{"mention"}, repos: []string{"cli/cli"}, typs: []string{"pr"}}},
		{"type:ci label:bug", filter{all: true, typs: []string{"ci"}}},
		{
			"reason:mention reason:Assign owner:Cli owner:golang repo:a/b repo:c/d type:pr type:issue reason:mention",
			filter{
				all:     true,
				reasons: []string{"mention", "assign"},
				owners:  []string{"cli", "golang"},
				repos:   []string{"a/b", "c/d"},
				typs:    []string{"pr", "issue"},
			},
		},
		{"reason:mention,assign type:pr,issue", filter{all: true, reasons: []string{"mention", "assign"}, typs: []string{"pr", "issue"}}},
		{"is:unread is:participating since:1W", filter{participating: true, since: "1w"}},
		{"since:1y", filter{all: true}},
	}
	for _, tt := range tests {
		got := parseFilter(tt.query)
		tt.want.query = tt.query
		if !reflect.DeepEqual(got, tt.want) {
			t.Errorf("parseFilter(%q) = %+v, want %+v", tt.query, got, tt.want)
		}
	}
}

func TestFilterListQuery(t *testing.T) {
	f := parseFilter("is:participating since:1w")
	want := core.NotificationFilter{All: true, Participating: true, Since: "1w"}
	if q := f.listQuery("c"); q.Filter != want || q.Cursor != "c" {
		t.Errorf("listQuery = %+v, want filter %+v and cursor c", q, want)
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
		{"reason:review_requested reason:mention", []string{"1", "2"}},
		{"reason:mention reason:ci_activity type:issue type:ci", []string{"2", "6"}},
		{"owner:eggzec", []string{"2", "7"}},
		{"owner:EggZec owner:cli", []string{"2", "4", "7"}},
		{"owner:charmbracelet repo:charmbracelet/lipgloss repo:cli/cli", []string{"5"}},
		{"owner:nobody", nil},
		{"repo:cli/cli repo:golang/go", []string{"3", "4"}},
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
		"is:unread is:participating since:1w",
		"is:participating since:1m reason:mention",
		"is:unread reason:review_requested repo:charmbracelet/bubbletea type:pr",
		"reason:mention type:discussion",
		// Several values of a key are written one qualifier each.
		"reason:review_requested reason:mention owner:eggzec repo:eggzec/gh-tui type:pr type:issue",
		"owner:cli owner:golang",
		// Values the form doesn't offer are shown as typed.
		"reason:invitation owner:someone repo:someone/else type:foo",
	} {
		s := newSection(t, newFake(inbox()...), 80, 12)
		press(t, s, showAll+q)
		f, _ := s.Filter()
		if got := filterform.New(f.Spec, filterform.WithQuery(f.Query)).Query(); got != q {
			t.Errorf("the form of %q writes %q", q, got)
		}
	}
}

// TestFilterOffersEveryReason checks that the form offers each reason
// GitHub gives, as a field that picks several.
func TestFilterOffersEveryReason(t *testing.T) {
	s := newSection(t, newFake(inbox()...), 80, 12)
	f, _ := s.Filter()
	field := f.Spec.Fields[3]
	if field.Kind != filterform.Multi || !field.Each || field.Qualifier != "reason" {
		t.Fatalf("the reason field is %+v, want a multi pick of one qualifier per value", field)
	}
	got := make([]string, 0, len(field.Options))
	for _, it := range field.Options {
		got = append(got, it.Value)
	}
	want := []string{
		"approval_requested", "assign", "author", "ci_activity", "comment", "invitation", "manual",
		"member_feature_requested", "mention", "review_requested", "security_advisory_credit",
		"security_alert", "state_change", "subscribed", "team_mention",
	}
	slices.Sort(got)
	if !slices.Equal(got, want) {
		t.Errorf("the filter offers the reasons %v, want %v", got, want)
	}
}

func TestFilterChips(t *testing.T) {
	tests := []struct{ query, want string }{
		{"is:unread", ""},
		{"is:unread reason:review_requested reason:mention", "review, mention"},
		{"reason:team_mention,assign owner:cli owner:golang repo:cli/cli type:pr type:issue", "team, assign, cli, golang, cli/cli, pr, issue"},
		{"is:unread is:participating since:1w", "participating, since 1w"},
		{"is:unread since:1y", ""},
	}
	for _, tt := range tests {
		f := parseFilter(tt.query)
		if got := f.chips(", "); got != tt.want {
			t.Errorf("chips of %q = %q, want %q", tt.query, got, tt.want)
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
	for _, it := range f.Spec.Fields[5].Options {
		repos = append(repos, it.Label)
	}
	if !slices.Equal(repos, want) {
		t.Errorf("the filter offers %v, want %v", repos, want)
	}
	wantOwners := []string{"charmbracelet", "eggzec", "golang", "cli", "a-very-long-organization-name"}
	owners := make([]string, 0, len(wantOwners))
	for _, it := range f.Spec.Fields[4].Options {
		owners = append(owners, it.Label)
	}
	if !slices.Equal(owners, wantOwners) {
		t.Errorf("the filter offers the owners %v, want %v", owners, wantOwners)
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
	if !strings.Contains(view, "Unread · mention  f filter · alt+f on/off · F clear") {
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

// TestFilterToggle checks that alt+f lists the inbox as if unfiltered and
// back, f edits the filter kept, and F clears it.
func TestFilterToggle(t *testing.T) {
	svc := newFake(inbox()...)
	s := newSection(t, svc, 80, 12)

	// The unread inbox is the default, so there is nothing to switch.
	press(t, s, "alt+f")
	if s.toggle.Off() {
		t.Fatal("alt+f switched off the default filter")
	}

	press(t, s, showAll+"is:unread reason:mention")
	if got := rows(s); !slices.Equal(got, []string{"2"}) {
		t.Fatalf("rows = %v, want the mention", got)
	}
	press(t, s, "alt+f")
	if got := rows(s); !slices.Equal(got, []string{"1", "2", "3", "6"}) {
		t.Errorf("rows while off = %v, want the unread", got)
	}
	if view := ansi.Strip(s.View()); !strings.Contains(view, "mention · off") {
		t.Errorf("the header should say the filter is off:\n%s", view)
	}
	if f, _ := s.Filter(); f.Query != "is:unread reason:mention" {
		t.Errorf("the form opens on %q, want the kept filter", f.Query)
	}

	press(t, s, "alt+f")
	if got := rows(s); !slices.Equal(got, []string{"2"}) || s.toggle.Off() {
		t.Errorf("rows after switching on = %v, want the mention", got)
	}

	press(t, s, "alt+f", "F")
	if got := rows(s); !slices.Equal(got, []string{"1", "2", "3", "6"}) || s.toggle.Off() || s.filtered() {
		t.Errorf("rows after F while off = %v, want the unread, on", got)
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
