package search

import (
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// commitsSection returns a page over commits, on the Commits kind with the
// results focused, and the query not yet submitted.
func commitsSection(t *testing.T) (*Section, *fakeService) {
	t.Helper()
	f := newFake()
	repo := core.RepoRef{Owner: "acme", Name: "tea"}
	f.commits = []core.SearchCommit{
		{Repo: repo, SHA: "0123456789abcdef", Subject: "tea brewing fix", Author: "ana"},
		{Repo: repo, SHA: "fedcba9876543210", Subject: "tea leaves", Author: "bo"},
	}
	s := newSection(t, f, 100, 24)
	press(t, s, "esc")
	press(t, s, strconv.Itoa(slices.Index(kinds, core.SearchCommits)+1))
	return s, f
}

func TestCommitsKindLoadsOnSubmit(t *testing.T) {
	t.Parallel()
	s, f := commitsSection(t)
	typeOnly(t, s, "tea")
	if len(f.commitQueries) != 0 {
		t.Fatalf("searched commits %v before the query was submitted", f.commitQueries)
	}
	press(t, s, "enter")
	if len(f.commitQueries) != 1 || f.commitQueries[0].Text != "tea" {
		t.Fatalf("searched commits with %+v, want one query for tea", f.commitQueries)
	}
}

func TestCommitRow(t *testing.T) {
	t.Parallel()
	s, _ := commitsSection(t)
	typeText(t, s, "tea")
	var row string
	for line := range strings.Lines(s.View()) {
		if strings.Contains(line, "tea brewing fix") {
			row = line
		}
	}
	if !strings.Contains(row, s.icons.Glyph(ui.GlyphCommit)) || !strings.Contains(row, "0123456") || strings.Contains(row, "0123456789") {
		t.Errorf("row of a commit = %q, want the glyph and a 7-character SHA", row)
	}
}

func TestCommitEnterOpensCommit(t *testing.T) {
	t.Parallel()
	s, _ := commitsSection(t)
	typeText(t, s, "tea")
	press(t, s, "down")
	app := press(t, s, "enter")
	if len(app) != 1 {
		t.Fatalf("enter sent %v, want one message", app)
	}
	got, ok := app[0].(ui.OpenCommitMsg)
	if !ok || got.SHA != "fedcba9876543210" || got.Repo.String() != "acme/tea" {
		t.Errorf("enter sent %#v, want the second commit opened", app[0])
	}
}

func TestCommitsFilterSpec(t *testing.T) {
	t.Parallel()
	sp := spec(core.SearchCommits, "")
	keys := make([]string, 0, len(sp.Fields))
	for _, f := range sp.Fields {
		keys = append(keys, f.Key)
	}
	if !slices.Contains(keys, "author") || !slices.Contains(keys, "repo") {
		t.Errorf("fields %v lack author or repo", keys)
	}
	if sp.Sort == nil || len(sp.Sort.Options) != 3 {
		t.Fatalf("sort = %+v, want best match, authored and committed", sp.Sort)
	}
}

func TestCommitsNoCount(t *testing.T) {
	t.Parallel()
	s, _ := commitsSection(t)
	typeText(t, s, "tea")
	if m := regexp.MustCompile(`Commits\W*\d`).FindString(s.View()); m != "" {
		t.Errorf("view shows a count for the commits: %q", m)
	}
}
