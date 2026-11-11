package releaselist

import (
	"context"
	"errors"
	"slices"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/releases"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

type fakeService struct {
	pages map[string]core.Page[core.Release]
	err   error
	// calls and invalidated record what the section asked, in order.
	calls       *[]releases.ListQuery
	invalidated *[]core.RepoRef
}

func (f fakeService) Invalidate(r core.RepoRef) {
	if f.invalidated != nil {
		*f.invalidated = append(*f.invalidated, r)
	}
}

func (f fakeService) List(_ context.Context, q releases.ListQuery) (core.Page[core.Release], error) {
	if f.calls != nil {
		*f.calls = append(*f.calls, q)
	}
	return f.pages[q.Cursor], f.err
}

// run feeds the messages of cmd back to s until none are left.
func run(s *Section, cmd tea.Cmd) {
	for cmd != nil {
		switch msg := cmd().(type) {
		case nil:
			return
		case tea.BatchMsg:
			for _, c := range msg {
				run(s, c)
			}
			return
		default:
			cmd = s.Update(msg)
		}
	}
}

func started(t *testing.T, svc fakeService) *Section {
	t.Helper()
	s := New(context.Background(), svc, config.Default().Keys)
	p, _ := config.Default().Palette(true)
	s.SetTheme(ui.NewTheme(p, true))
	s.SetSize(100, 10)
	s.Update(ui.RepoMsg{Repo: core.RepoRef{Owner: "o", Name: "r"}})
	return s
}

func releasesOf(n int) []core.Release {
	rs := make([]core.Release, 0, n)
	for i := range n {
		rs = append(rs, core.Release{ID: int64(i + 1), Tag: "v1." + string(rune('0'+i))})
	}
	return rs
}

func TestCountLoadedAndMore(t *testing.T) {
	t.Parallel()
	// More rows than fit, so the next page is not read ahead.
	s := started(t, fakeService{pages: map[string]core.Page[core.Release]{"": {Items: releasesOf(30), Next: "2"}}})
	if s.CountForms() != nil {
		t.Fatalf("forms = %q before the load, want none", s.CountForms())
	}
	run(s, s.Init())
	forms := s.CountForms()
	if len(forms) == 0 || slices.Contains(forms, "") {
		t.Fatalf("forms = %q, want some", forms)
	}
	if want := ui.CountForms(ui.CountSpec{Loaded: 30, More: true}); !slices.Equal(forms, want) {
		t.Errorf("forms = %q, want %q", forms, want)
	}
}

var testRepo = core.RepoRef{Owner: "o", Name: "r"}

func TestRefreshInvalidatesAndFetches(t *testing.T) {
	t.Parallel()
	var calls []releases.ListQuery
	var inv []core.RepoRef
	s := started(t, fakeService{pages: map[string]core.Page[core.Release]{"": {Items: releasesOf(2)}}, calls: &calls, invalidated: &inv})
	run(s, s.Init())
	s.Focus()
	n := len(calls)
	run(s, s.Update(tea.KeyPressMsg{Code: 'r', Text: "r"}))
	if len(inv) != 1 || inv[0] != testRepo {
		t.Errorf("invalidated = %v, want [%v]", inv, testRepo)
	}
	if len(calls) <= n {
		t.Errorf("calls = %d after refresh, want more than %d", len(calls), n)
	}
}

func TestSyncReloadsOnlyOnItsKey(t *testing.T) {
	t.Parallel()
	var calls []releases.ListQuery
	s := started(t, fakeService{pages: map[string]core.Page[core.Release]{"": {Items: releasesOf(2)}}, calls: &calls})
	run(s, s.Init())
	n := len(calls)
	other := core.RepoRef{Owner: "o", Name: "other"}
	for name, msg := range map[string]ui.SyncMsg{
		"other repo": {Key: releases.SyncKey(other)},
		"error":      {Key: releases.SyncKey(testRepo), Err: errors.New("boom")},
	} {
		run(s, s.Update(msg))
		if len(calls) != n {
			t.Fatalf("%s: calls = %d, want %d", name, len(calls), n)
		}
	}
	run(s, s.Update(ui.SyncMsg{Key: releases.SyncKey(testRepo)}))
	if len(calls) <= n {
		t.Errorf("calls = %d after a matching key, want more than %d", len(calls), n)
	}
}

func TestSecondPageLoads(t *testing.T) {
	t.Parallel()
	var calls []releases.ListQuery
	s := started(t, fakeService{pages: map[string]core.Page[core.Release]{
		"":  {Items: releasesOf(3), Next: "2"},
		"2": {Items: releasesOf(2)},
	}, calls: &calls})
	run(s, s.Init())
	for range 5 {
		run(s, s.Update(tea.KeyPressMsg{Code: 'G', Text: "G"}))
	}
	if !slices.ContainsFunc(calls, func(q releases.ListQuery) bool { return q.Cursor == "2" }) {
		t.Errorf("calls = %+v, want a read of the second page", calls)
	}
}

func TestCountNoneOnError(t *testing.T) {
	t.Parallel()
	s := started(t, fakeService{err: errors.New("boom")})
	run(s, s.Init())
	if f := s.CountForms(); f != nil {
		t.Errorf("forms = %q after an error, want none", f)
	}
}
