package dev

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/core"
)

func TestMentionsRows(t *testing.T) {
	links := core.PullLinks{Mentions: []core.Reference{
		issue(self, 5, "Same crash", core.StateOpen),
		pull(393, "Port the fix", core.StateMerged, false),
	}}
	rows := Build(links, hideCloses(opts("unicode")))
	text := make([]string, 0, len(rows.Rows))
	for _, r := range rows.Rows {
		text = append(text, ansi.Strip(strings.Join(r.Lines, " ")))
	}
	got := strings.Join(text, "\n")
	for _, want := range []string{"Mentions 2", "#5 Same crash", "#393 Port the fix"} {
		if !strings.Contains(got, want) {
			t.Errorf("rows lack %q:\n%s", want, got)
		}
	}
	if it := rows.Items["mentions:acme/app#393"]; it.Kind != Pull {
		t.Errorf("pull mention has kind %v, want Pull", it.Kind)
	}
	if it := rows.Items["mentions:acme/app#5"]; it.Kind != Issue {
		t.Errorf("issue mention has kind %v, want Issue", it.Kind)
	}
	if Build(core.PullLinks{}, opts("unicode")).Items[mentionsID].Kind != 0 {
		t.Error("an item that names nothing has a Mentions header")
	}
}

func hideCloses(o Options) Options {
	o.HideCloses = true
	return o
}
