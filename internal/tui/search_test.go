package tui

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/picker"
)

var (
	searchRepo = core.RepoRef{Owner: "eggzec", Name: "gh-tui"}
	searchHits = []picker.Item{
		{Kind: KindRepos, Title: "eggzec/gh-tui", Value: searchRepo},
		{Kind: KindIssues, Title: "tabs overflow", Value: core.SearchHit{
			Kind: core.SearchIssues, Issue: core.Issue{Repo: searchRepo, Number: 12},
		}},
		{Kind: KindPulls, Title: "watch the repo", Value: core.SearchHit{
			Kind: core.SearchPulls, Issue: core.Issue{Repo: searchRepo, Number: 37},
		}},
	}
)

func fixedSearch(queries *[]picker.Query) picker.Search {
	return func(_ context.Context, q picker.Query) ([]picker.Item, error) {
		*queries = append(*queries, q)
		return searchHits, nil
	}
}

func TestSearchChoosesRepo(t *testing.T) {
	var queries []picker.Query
	m, fakes := newTestApp(t, WithSearch(fixedSearch(&queries)))
	run(m, m.key(press("/")))
	if m.topModal() != m.searchBox {
		t.Fatal("/ didn't open the search")
	}
	if len(queries) != 1 || queries[0] != (picker.Query{}) {
		t.Errorf("queries = %+v, want one empty query", queries)
	}
	if s := onScreen(m); !strings.Contains(s, "Search") || !strings.Contains(s, "tabs overflow") {
		t.Errorf("screen lacks the search and its results:\n%s", s)
	}

	run(m, m.key(tea.KeyPressMsg{Code: tea.KeyEnter}))
	if m.topModal() != nil {
		t.Error("choosing didn't close the search")
	}
	for _, f := range fakes {
		if !f.got(func(msg tea.Msg) bool { r, ok := msg.(ui.RepoMsg); return ok && r.Repo == searchRepo }) {
			t.Errorf("%s missed the chosen repository", f.title)
		}
	}
}

func TestSearchOpensIssuesAndPulls(t *testing.T) {
	var queries []picker.Query
	m, fakes := newTestApp(t, WithSearch(fixedSearch(&queries)))
	tests := []struct {
		downs int
		want  tea.Msg
	}{
		{1, ui.OpenIssueMsg{Repo: searchRepo, Number: 12}},
		{2, ui.OpenPullMsg{Repo: searchRepo, Number: 37}},
	}
	for _, tt := range tests {
		run(m, m.key(press("/")))
		for range tt.downs {
			run(m, m.key(tea.KeyPressMsg{Code: tea.KeyDown}))
		}
		run(m, m.key(tea.KeyPressMsg{Code: tea.KeyEnter}))
		if !fakes[1].got(func(msg tea.Msg) bool { return msg == tt.want }) {
			t.Errorf("sections missed %#v", tt.want)
		}
		if m.topModal() != nil {
			t.Error("choosing didn't close the search")
		}
	}
	if len(queries) != 2 {
		t.Errorf("searched %d times, want once per opening", len(queries))
	}
}

func TestSearchCancels(t *testing.T) {
	var queries []picker.Query
	m, fakes := newApp(t, core.RepoRef{}, WithSearch(fixedSearch(&queries)))
	run(m, m.key(press("/")))
	run(m, m.key(tea.KeyPressMsg{Code: tea.KeyEscape}))
	if m.topModal() != nil {
		t.Error("esc didn't close the search")
	}
	if fakes[0].got(func(msg tea.Msg) bool { _, ok := msg.(ui.RepoMsg); return ok }) {
		t.Error("cancelling selected a repository")
	}
}

func TestSearchNeedsProducer(t *testing.T) {
	m, _ := newTestApp(t)
	run(m, m.key(press("/")))
	if m.topModal() != nil {
		t.Error("search opened without a producer")
	}
}

func TestSearchLeavesTheChosenItemOpen(t *testing.T) {
	var queries []picker.Query
	m, fakes := newTestApp(t, WithSearch(fixedSearch(&queries)))
	detail := &fakeModal{title: "#12"}
	fakes[1].reply = func(msg tea.Msg) tea.Cmd {
		if _, ok := msg.(ui.OpenIssueMsg); ok {
			return ui.OpenModal(detail)
		}
		return nil
	}
	run(m, m.key(press("/")))
	run(m, m.key(tea.KeyPressMsg{Code: tea.KeyDown}))
	chosen := m.key(tea.KeyPressMsg{Code: tea.KeyEnter})()
	// A batch could open the issue before the search closes, and closing
	// the search would close the issue with it.
	if _, ok := m.searchBox.Update(chosen)().(tea.BatchMsg); ok {
		t.Fatal("the search closes and opens the choice in a batch, not in order")
	}
	run(m, func() tea.Msg { return chosen })
	if m.topModal() != detail {
		t.Errorf("open modal = %v, want the issue the search opened", m.topModal())
	}
}
