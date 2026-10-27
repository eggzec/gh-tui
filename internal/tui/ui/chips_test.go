package ui

import (
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/core"
)

func TestQueryChips(t *testing.T) {
	t.Parallel()
	faint := func(s string) string {
		return ansi.SGR(ansi.AttrFaint) + s + ansi.SGR(ansi.AttrNormalIntensity)
	}
	for _, sep := range []string{" · ", " | "} {
		tests := []struct {
			name string
			spec ChipSpec
			want []string
		}{
			{"filter on with chips", ChipSpec{Filter: "bug", FilterOn: true, SortOn: true}, []string{"bug"}},
			{"filter on with none", ChipSpec{FilterOn: true, SortOn: true}, nil},
			{"filter off keeps chips dim", ChipSpec{Filter: "bug", Sort: "sort updated", SortOn: true}, []string{faint("bug" + sep + "off"), "sort updated"}},
			{"filter off without sort off", ChipSpec{Filter: "bug"}, []string{faint("bug" + sep + "off")}},
			{"sort on with words", ChipSpec{FilterOn: true, Sort: "sort updated", SortOn: true}, []string{"sort updated"}},
			{"sort off with words", ChipSpec{FilterOn: true, Sort: "sort updated"}, []string{faint("sort updated" + sep + "off")}},
			{"sort off no words, filter on", ChipSpec{Filter: "bug", FilterOn: true}, []string{"bug", faint("sort" + sep + "off")}},
			{"sort off no words, filter off", ChipSpec{Filter: "bug"}, []string{faint("bug" + sep + "off")}},
			{"group set", ChipSpec{FilterOn: true, SortOn: true, Group: "by owner"}, []string{"by owner"}},
			{"order", ChipSpec{Filter: "bug", FilterOn: true, Sort: "sort updated", SortOn: true, Group: "by owner"}, []string{"bug", "sort updated", "by owner"}},
		}
		for _, tt := range tests {
			t.Run(tt.name+" "+sep, func(t *testing.T) {
				t.Parallel()
				got := QueryChips(Icons{Separator: sep}, tt.spec)
				if !slices.Equal(got, tt.want) {
					t.Errorf("QueryChips(%+v) = %q, want %q", tt.spec, got, tt.want)
				}
			})
		}
	}
}

func TestOffChipsWords(t *testing.T) {
	t.Parallel()
	for _, sep := range []string{" · ", " | "} {
		if got := ansi.Strip(GroupOffChip([]string{"owner", "language"}, sep)); got != "by owner, language"+sep+"off" {
			t.Errorf("GroupOffChip = %q", got)
		}
		got := SortOffChip(sep)
		if ansi.Strip(got) != "sort"+sep+"off" || !strings.HasPrefix(got, ansi.SGR(ansi.AttrFaint)) {
			t.Errorf("SortOffChip = %q, want dim %q", got, "sort"+sep+"off")
		}
	}
}

func TestPersonItemsLeaveOutTheViewer(t *testing.T) {
	users := []core.User{{Login: "octocat", Name: "The Octocat"}, {Login: "Hubot"}, {Login: "monalisa"}}
	tests := []struct {
		name, viewer string
		want         []string
	}{
		{"unknown viewer", "", []string{"octocat", "Hubot", "monalisa"}},
		{"viewer listed", "octocat", []string{"Hubot", "monalisa"}},
		{"viewer in another case", "hubot", []string{"octocat", "monalisa"}},
		{"viewer not listed", "someone", []string{"octocat", "Hubot", "monalisa"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			items := PersonItems(users, tt.viewer)
			got := make([]string, len(items))
			for i, it := range items {
				got[i] = it.Value
			}
			if len(got) != len(tt.want) {
				t.Fatalf("PersonItems(%q) = %v, want %v", tt.viewer, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("PersonItems(%q) = %v, want %v", tt.viewer, got, tt.want)
				}
			}
		})
	}
	if it := PersonItems(users[:1], "")[0]; it.Label != "octocat" || it.Detail != "The Octocat" {
		t.Errorf("item = %+v, want the login with the name beside", it)
	}
}
