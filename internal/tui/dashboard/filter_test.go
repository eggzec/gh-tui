package dashboard

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ownerui"
	"github.com/eggzec/gh-tui/pkg/bubbles/filterform"
)

// TestFilterSpec checks that the form of the filter opens on a query and
// writes it back, and that its fields write what the filter reads.
func TestFilterSpec(t *testing.T) {
	s := newSection(t, newFake(), nil, 140, 38)
	f, ok := s.Filter()
	if !ok {
		t.Fatal("the repositories should have a filter")
	}
	if f.Subject != "Repositories" || f.Query != "" {
		t.Errorf("Filter = %q on %q, want the repositories unfiltered", f.Subject, f.Query)
	}
	langs := make([]string, 0, 4)
	for _, it := range f.Spec.Fields[5].Options {
		langs = append(langs, it.Label)
	}
	// The first page of 100 is read: 25 of each language, and 25 with
	// none, which isn't offered.
	if want := []string{"Any", "Go", "Rust", "TypeScript"}; !slices.Equal(langs, want) {
		t.Errorf("the languages offered are %v, want %v", langs, want)
	}
	for _, q := range []string{
		"repo is:private fork:false archived:false template:true language:rust sort:stars-desc",
		"is:public sort:name-asc",
		"language:go sort:updated-desc",
	} {
		form := filterform.New(f.Spec, filterform.WithQuery(q))
		if got := form.Query(); got != q {
			t.Errorf("the form of %q writes %q", q, got)
		}
	}
	// The default sort writes nothing once applied.
	run(t, s, s.ApplyFilter(filterform.AppliedMsg{Query: "language:go sort:updated-desc"}))
	if got := s.repos.filter().Query(); got != "language:go" {
		t.Errorf("the filter in force is %q, want the default sort left out", got)
	}
	// A language no repository read has is offered once it filters.
	run(t, s, s.ApplyFilter(filterform.AppliedMsg{Query: "language:zig"}))
	f, _ = s.Filter()
	if !slices.ContainsFunc(f.Spec.Fields[5].Options, func(it filterform.Item) bool { return it.Value == "zig" }) {
		t.Error("the language of the filter should be offered")
	}

	press(t, s, "3")
	if _, ok := s.Filter(); ok {
		t.Error("only the repositories should have a filter")
	}
}

func TestFilterLists(t *testing.T) {
	svc := newFake()
	s := newSection(t, svc, nil, 140, 38)
	calls := len(svc.calls)
	run(t, s, s.ApplyFilter(filterform.AppliedMsg{Query: "is:private language:go sort:stars-desc"}))

	// The filter runs over every repository: the first page is cached, and
	// the other two are read once.
	if got := svc.calls[calls:]; !slices.Equal(got, []string{"repos @me@100", "repos @me@200"}) {
		t.Errorf("the filter read %v, want the two pages the list hadn't", got)
	}
	o := s.repos.current()
	pf := ownerui.ParseFilter("is:private language:go")
	want := pf.Apply(repos("octocat", 250))
	slices.SortStableFunc(want, func(a, b core.Repo) int { return b.Stars - a.Stars })
	if o.Feed.Len() != len(want) || len(want) == 0 {
		t.Fatalf("the list holds %d repositories, want %d", o.Feed.Len(), len(want))
	}
	for i := range want {
		if r, _ := o.Feed.Item(i); r.Ref != want[i].Ref || !r.Private || r.Language != "Go" {
			t.Fatalf("row %d is %v, want %v", i, r.Ref, want[i].Ref)
		}
	}
	if view := screen(s); !strings.Contains(view, "Repositories · private · go · stars ↓") {
		t.Errorf("the pane title should name the filter:\n%s", view)
	}

	// Applying it again reads nothing, and switching tabs filters them too.
	calls = len(svc.calls)
	run(t, s, s.ApplyFilter(filterform.AppliedMsg{Query: "is:private language:go sort:stars-desc"}))
	if len(svc.calls) != calls {
		t.Errorf("the same filter read %v", svc.calls[calls:])
	}
	press(t, s, "]")
	if n := s.repos.current().Feed.Len(); n != 0 {
		t.Errorf("github lists %d repositories, want none: it has no private Go one", n)
	}
	if view := screen(s); !strings.Contains(view, "No repositories match the filters.") {
		t.Errorf("an empty filtered list should say so:\n%s", view)
	}

	// F clears the filter of every tab.
	press(t, s, "F")
	if s.repos.filter().Active() || s.repos.current().Feed.Len() != 5 {
		t.Errorf("F should clear the filter; github lists %d", s.repos.current().Feed.Len())
	}
	press(t, s, "[")
	if o := s.repos.current(); o.Feed.Len() != 100 || o.Feed.Done() {
		t.Errorf("yours list %d repositories after F, want the first page of the list", o.Feed.Len())
	}
	if strings.Contains(screen(s), "stars ↓") {
		t.Error("the chips should go with the filter")
	}
}

func TestFilterIsCapped(t *testing.T) {
	svc := newFake()
	svc.repos["@me"] = repos("octocat", 1500)
	s := newSection(t, svc, nil, 140, 38)
	run(t, s, s.ApplyFilter(filterform.AppliedMsg{Query: "sort:name-asc"}))
	if got := s.repos.current().Feed.Len(); got != 1000 {
		t.Errorf("the filter lists %d repositories, want the cap of 1000", got)
	}
	if n := svc.count("repos @me@1000"); n != 0 {
		t.Error("the filter should stop reading at the cap")
	}
}

func TestFilterFailure(t *testing.T) {
	svc := newFake()
	svc.fail["repos @me@100"] = errors.New("github: 502 Bad Gateway")
	s := newSection(t, svc, nil, 140, 38)
	run(t, s, s.ApplyFilter(filterform.AppliedMsg{Query: "is:private"}))
	if err := s.repos.current().Feed.Err(); err == nil || !strings.Contains(err.Error(), "502") {
		t.Errorf("the list's error is %v, want the failed page", err)
	}
	// Once the page reads, a retry lists what the filter keeps.
	delete(svc.fail, "repos @me@100")
	press(t, s, "r")
	if got := s.repos.current().Feed.Len(); got != 50 {
		t.Errorf("the filter lists %d repositories after the retry, want the 50 private ones", got)
	}
}
