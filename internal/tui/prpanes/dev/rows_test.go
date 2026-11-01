package dev

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/pulls"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/treeview"
)

// The pull service reads what the pane reads.
var _ Service = (*pulls.Service)(nil)

var self = core.RepoRef{Owner: "acme", Name: "app"}

func theme() ui.Theme {
	p, err := config.Default().Palette(true)
	if err != nil {
		panic(err)
	}
	return ui.NewTheme(p, true)
}

func opts(set string) Options {
	return Options{Repo: self, Styles: NewStyles(theme(), ui.NewIcons(set))}
}

func issue(repo core.RepoRef, n int, title string, st core.State) core.Reference {
	return core.Reference{Target: core.Target{Repo: repo, Number: n, Kind: core.KindIssue}, Title: title, State: st}
}

func pull(n int, title string, st core.State, draft bool) core.Reference {
	return core.Reference{Target: core.Target{Repo: self, Number: n, Kind: core.KindPull}, Title: title, State: st, Draft: draft}
}

func fixture() core.PullLinks {
	other := core.RepoRef{Owner: "acme", Name: "lib"}
	return core.PullLinks{
		Closes: []core.LinkedIssue{
			{Issue: issue(self, 12, "Crash on empty input", core.StateOpen), Blockers: []core.Reference{issue(self, 9, "Parser rewrite", core.StateOpen)}, BlockersTotal: 3},
			{Issue: issue(other, 7, "Shared helper is slow", core.StateClosed)},
		},
		ClosesTotal: 3,
		Linked: []core.Reference{
			pull(30, "Follow-up cleanup", core.StateOpen, true),
			pull(31, "Merged sibling", core.StateMerged, false),
		},
		LinkedCapped:      true,
		BlockersSupported: true,
	}
}

func text(r Rows, width int) string {
	m := treeview.New(treeview.Source{}, treeview.WithSize(width, 40), treeview.WithRows(r.Rows))
	lines := strings.Split(ansi.Strip(m.View()), "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " ")
	}
	return strings.TrimRight(strings.Join(lines, "\n"), "\n") + "\n"
}

func ids(r Rows) []string {
	out := make([]string, 0, len(r.Rows))
	for i := range r.Rows {
		out = append(out, r.Rows[i].ID)
	}
	return out
}

func TestRowsGolden(t *testing.T) {
	t.Parallel()
	for _, set := range []string{config.IconsNerd, config.IconsASCII} {
		for _, w := range []int{80, 120, 160} {
			for name, links := range map[string]core.PullLinks{"rows": fixture(), "empty": {BlockersSupported: true}} {
				t.Run(fmt.Sprintf("%s-%s-%d", name, set, w), func(t *testing.T) {
					t.Parallel()
					golden.RequireEqual(t, text(Build(links, opts(set)), w))
				})
			}
		}
	}
}

func TestBlockersOneLevel(t *testing.T) {
	t.Parallel()
	r := Build(fixture(), opts(config.IconsASCII))
	depth := map[string]int{}
	for _, row := range r.Rows {
		depth[row.ID] = row.Depth
	}
	if depth["issue:"+core.RefKey(fixture().Closes[0].Issue.Target)] != 1 || depth["blocker:"+core.RefKey(fixture().Closes[0].Issue.Target)+">"+core.RefKey(fixture().Closes[0].Blockers[0].Target)] != 2 {
		t.Errorf("depths = %v, want issue 1 and blocker 2", depth)
	}
	for _, row := range r.Rows {
		if row.Depth > 2 {
			t.Errorf("row %s at depth %d", row.ID, row.Depth)
		}
	}
}

func TestNoBlockersWhenUnsupported(t *testing.T) {
	t.Parallel()
	links := fixture()
	links.BlockersSupported = false
	for _, id := range ids(Build(links, opts(config.IconsASCII))) {
		if strings.HasPrefix(id, "blocker:") || strings.HasPrefix(id, "more:blockers") {
			t.Errorf("row %s with blockers unsupported", id)
		}
	}
}

func TestEmptyLinkedHasNoRow(t *testing.T) {
	t.Parallel()
	r := Build(core.PullLinks{BlockersSupported: true}, opts(config.IconsASCII))
	if got, want := ids(r), []string{"header:closes", "none:closes", "header:linked"}; fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("rows = %v, want %v", got, want)
	}
}

func TestCrossRepoIdent(t *testing.T) {
	t.Parallel()
	got := text(Build(fixture(), opts(config.IconsASCII)), 120)
	if !strings.Contains(got, "acme/lib#7 ") || !strings.Contains(got, " #12 ") {
		t.Errorf("rows:\n%s", got)
	}
}

func TestCapsShowMore(t *testing.T) {
	t.Parallel()
	got := text(Build(fixture(), opts(config.IconsASCII)), 120)
	for _, want := range []string{"Closes 3", "+1 more", "+2 more", "more on GitHub", "Linked PRs 2"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}
