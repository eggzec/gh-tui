package search

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

func TestGlyphs(t *testing.T) {
	s := newSection(t, newFake(), 140, 38, WithIcons(ui.NewIcons(config.IconsASCII)))
	notPlanned := issue(core.SearchIssues, "cli/cli", 7, "Not planned", core.StateClosed, false)
	notPlanned.Issue.Reason = core.ReasonNotPlanned
	declined := issue(core.SearchPulls, "cli/cli", 8, "Declined", core.StateClosed, false)
	for _, tt := range []struct {
		hit  core.SearchHit
		want string
	}{
		{issue(core.SearchIssues, "cli/cli", 1, "Open", core.StateOpen, false), "o cli/cli#1 Open"},
		{issue(core.SearchIssues, "cli/cli", 2, "Done", core.StateClosed, false), "x cli/cli#2 Done"},
		{notPlanned, "- cli/cli#7 Not planned"},
		{issue(core.SearchPulls, "cli/cli", 3, "Open", core.StateOpen, false), "O cli/cli#3 Open"},
		{issue(core.SearchPulls, "cli/cli", 4, "Draft", core.StateOpen, true), "D cli/cli#4 Draft"},
		{issue(core.SearchPulls, "cli/cli", 5, "Merged", core.StateMerged, false), "M cli/cli#5 Merged"},
		{declined, "X cli/cli#8 Declined"},
	} {
		if got := ansi.Strip(s.renderHit(tt.hit, false, 80)); !strings.HasPrefix(got, tt.want) {
			t.Errorf("row = %q, want it to start with %q", got, tt.want)
		}
	}

	r := repo("cli", "cli", "GitHub's official command line tool", "Go", 38000, 0)
	r.Repo.Fork, r.Repo.Private, r.Repo.Archived = true, true, true
	got := ansi.Strip(s.renderHit(r, false, 80))
	for _, want := range []string{"cli/cli F P A", "o Go"} {
		if !strings.Contains(got, want) {
			t.Errorf("repository row = %q, want %q in it", got, want)
		}
	}
	if strings.Contains(got, "private") || strings.Contains(got, "fork") {
		t.Errorf("repository row = %q, want glyphs rather than words", got)
	}
}
