package search

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
)

// reads records which repositories had more of their preview read.
type reads struct {
	mu    sync.Mutex
	names []string
	err   error
}

func (r *reads) reader(h core.SearchHit) func(context.Context) (string, error) {
	if h.Kind != core.SearchRepos {
		return nil
	}
	name := h.Repo.Ref.String()
	return func(context.Context) (string, error) {
		r.mu.Lock()
		defer r.mu.Unlock()
		r.names = append(r.names, name)
		return "README of " + name, r.err
	}
}

func (r *reads) got() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.names)
}

func previewSection(t *testing.T, r *reads, rows int) *Section {
	t.Helper()
	p := config.Default().Prefetch
	p.Search.Preview = rows
	return newSection(t, newFake(), 140, 40, WithPreviewReader(r.reader), WithPrefetch(p))
}

func TestRepoTextHoldsTopics(t *testing.T) {
	t.Parallel()
	got := repoText(core.Repo{Description: "A tea timer", Topics: []string{"tea", "timer"}})
	if !strings.Contains(got, "Topics: tea, timer") {
		t.Errorf("repoText lacks the topics line:\n%s", got)
	}
	if strings.Contains(repoText(core.Repo{Description: "x"}), "Topics:") {
		t.Error("repoText shows a topics line for a repository with none")
	}
}

func TestPreviewReadsTheCursorAndItsNeighbours(t *testing.T) {
	t.Parallel()
	r := &reads{}
	s := previewSection(t, r, 2)
	typeText(t, s, "tea")
	total := len(newFake().of(core.SearchRepos))
	if got := r.got(); len(got) != min(3, total) {
		t.Fatalf("read %v with the cursor on the first row, want it and the two after it", got)
	}
	press(t, s, "j", "j", "j")
	if got := r.got(); len(got) != min(6, total) {
		t.Errorf("read %v with the cursor on the fourth row, want each row up to two past it, once", got)
	}
	seen := map[string]bool{}
	for _, n := range r.got() {
		if seen[n] {
			t.Errorf("%s was read twice", n)
		}
		seen[n] = true
	}
}

func TestPreviewZeroReadsOnlyTheCursorRow(t *testing.T) {
	t.Parallel()
	r := &reads{}
	s := previewSection(t, r, 0)
	typeText(t, s, "tea")
	if got := r.got(); len(got) != 1 {
		t.Errorf("read %v with preview 0, want only the row under the cursor", got)
	}
}

func TestPreviewShowsWhatWasRead(t *testing.T) {
	t.Parallel()
	r := &reads{}
	s := previewSection(t, r, 0)
	typeText(t, s, "tea")
	if got := s.View(); !strings.Contains(got, "README of ") {
		t.Errorf("the preview lacks what was read:\n%s", got)
	}
}

func TestPreviewShowsTheReadFailing(t *testing.T) {
	t.Parallel()
	r := &reads{err: errors.New("boom")}
	s := previewSection(t, r, 0)
	typeText(t, s, "tea")
	if got := s.View(); !strings.Contains(got, "Could not load more: boom") {
		t.Errorf("the preview lacks the error:\n%s", got)
	}
}

func TestPreviewShowsLoadingUntilTheReadLands(t *testing.T) {
	t.Parallel()
	r := &reads{}
	s := previewSection(t, r, 0)
	typeOnly(t, s, "tea")
	cmd := s.Update(keyPress("enter"))
	// Hold the read's message back: the preview says it is loading.
	_, held := drive(t, s, cmd, func(m tea.Msg) bool { _, ok := m.(previewReadMsg); return ok })
	if len(held) != 1 {
		t.Fatalf("held %d reads, want one", len(held))
	}
	if got := s.View(); !strings.Contains(got, "Loading more") {
		t.Errorf("the preview does not say it is loading:\n%s", got)
	}
	run(t, s, s.Update(held[0]))
	if got := s.View(); strings.Contains(got, "Loading more") || !strings.Contains(got, "README of ") {
		t.Errorf("the preview kept the loading text after the read landed:\n%s", got)
	}
}
