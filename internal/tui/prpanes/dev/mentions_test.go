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

func TestMentionsAndReferencedBy(t *testing.T) {
	a, p := issue(self, 5, "Same crash", core.StateOpen), pull(393, "Port the fix", core.StateMerged, false)
	tests := []struct {
		name  string
		links core.PullLinks
		want  []string
		not   []string
	}{
		{"empty", core.PullLinks{}, nil, []string{"Mentions", "Referenced by"}},
		{"mentions both kinds", core.PullLinks{Mentions: []core.Reference{a, p}}, []string{"Mentions 2", "#5 Same crash", "#393 Port the fix"}, []string{"Referenced by"}},
		{"capped mentions", core.PullLinks{Mentions: []core.Reference{a}, MentionsCapped: true}, []string{"Mentions 1+", "more on GitHub"}, nil},
		{"referenced by both kinds", core.PullLinks{Mentioned: []core.Reference{a, p}, MentionedTotal: 2}, []string{"Referenced by 2", "#5 Same crash", "#393 Port the fix"}, []string{"Mentions"}},
		{"in both groups", core.PullLinks{Mentions: []core.Reference{a}, Mentioned: []core.Reference{a}, MentionedTotal: 1}, []string{"Mentions 1", "Referenced by 1"}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rows := Build(tt.links, hideCloses(opts("unicode")))
			var text []string
			seen := map[string]bool{}
			for _, r := range rows.Rows {
				text = append(text, ansi.Strip(strings.Join(r.Lines, " ")))
				if seen[r.ID] {
					t.Errorf("row ID %q is used twice", r.ID)
				}
				seen[r.ID] = true
			}
			got := strings.Join(text, "\n")
			for _, w := range tt.want {
				if !strings.Contains(got, w) {
					t.Errorf("rows lack %q:\n%s", w, got)
				}
			}
			for _, n := range tt.not {
				if strings.Contains(got, n) {
					t.Errorf("rows have %q:\n%s", n, got)
				}
			}
		})
	}
}

func hideCloses(o Options) Options {
	o.HideCloses = true
	return o
}
