package ownerui

import (
	"slices"
	"testing"
	"time"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/filterform"
)

// filterNow is when the repositories of filterRepos were last updated
// from.
var filterNow = time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)

func TestParseFilter(t *testing.T) {
	tests := []struct {
		query string
		want  Filter
	}{
		{"", Filter{}},
		{"is:private", Filter{visibility: "private"}},
		{"is:Public fork:false archived:false", Filter{visibility: "public", forks: "false", archived: "false"}},
		{"template:true language:Go", Filter{templates: true, language: "go"}},
		{"is:template fork:only", Filter{templates: true, forks: "only"}},
		{`language:"jupyter notebook"`, Filter{language: "jupyter notebook"}},
		{"sort:stars-desc", Filter{sort: filterform.Sort{By: "stars", Desc: true}}},
		{"sort:name-asc", Filter{sort: filterform.Sort{By: "name"}}},
		{"sort:updated", Filter{sort: filterform.Sort{By: "updated", Desc: true}}},
		{"sort:forks-desc", Filter{}},
		{"gh tui topic:cli", Filter{words: []string{"gh", "tui"}}},
	}
	for _, tt := range tests {
		got := ParseFilter(tt.query)
		tt.want.query = tt.query
		if got.query != tt.want.query || got.visibility != tt.want.visibility || got.forks != tt.want.forks ||
			got.archived != tt.want.archived || got.templates != tt.want.templates || got.language != tt.want.language ||
			got.sort != tt.want.sort || !slices.Equal(got.words, tt.want.words) {
			t.Errorf("ParseFilter(%q) = %+v, want %+v", tt.query, got, tt.want)
		}
	}
}

// filterRepos is a small set of repositories with one of each kind.
func filterRepos() []core.Repo {
	r := func(name, lang string, stars int) core.Repo {
		return core.Repo{Ref: core.RepoRef{Owner: "octocat", Name: name}, Language: lang, Stars: stars}
	}
	all := []core.Repo{
		r("gh-tui", "Go", 40),
		r("dotfiles", "Shell", 3),
		r("secret", "Go", 1),
		r("fork-of-cli", "Go", 0),
		r("old-site", "HTML", 12),
		r("template-go", "Go", 7),
	}
	all[2].Private = true
	all[3].Fork = true
	all[4].Archived = true
	all[5].Template = true
	for i := range all {
		all[i].UpdatedAt = filterNow.Add(-time.Duration(i) * time.Hour)
	}
	return all
}

func repoNames(repos []core.Repo) []string {
	out := make([]string, len(repos))
	for i := range repos {
		out[i] = repos[i].Ref.Name
	}
	return out
}

func TestFilterApply(t *testing.T) {
	tests := []struct {
		query string
		want  []string
	}{
		{"", []string{"gh-tui", "dotfiles", "secret", "fork-of-cli", "old-site", "template-go"}},
		{"is:private", []string{"secret"}},
		{"is:public language:go", []string{"gh-tui", "fork-of-cli", "template-go"}},
		{"fork:false archived:false", []string{"gh-tui", "dotfiles", "secret", "template-go"}},
		{"fork:only", []string{"fork-of-cli"}},
		{"archived:true", []string{"old-site"}},
		{"template:true", []string{"template-go"}},
		{"language:go sort:stars-desc", []string{"gh-tui", "template-go", "secret", "fork-of-cli"}},
		{"sort:stars-asc language:go", []string{"fork-of-cli", "secret", "template-go", "gh-tui"}},
		{"sort:name-asc archived:false fork:false", []string{"dotfiles", "gh-tui", "secret", "template-go"}},
		{"sort:updated-asc is:public", []string{"template-go", "old-site", "fork-of-cli", "dotfiles", "gh-tui"}},
		// Words match names fuzzily, every word, the best matches first.
		{"ght", []string{"gh-tui"}},
		{"o t", []string{"old-site", "template-go", "dotfiles"}},
		{"fork:true", []string{"gh-tui", "dotfiles", "secret", "fork-of-cli", "old-site", "template-go"}},
		{"go is:private", []string{}},
		// Tokens the filter doesn't read keep everything.
		{"topic:cli", []string{"gh-tui", "dotfiles", "secret", "fork-of-cli", "old-site", "template-go"}},
	}
	for _, tt := range tests {
		f := ParseFilter(tt.query)
		if got := repoNames(f.Apply(filterRepos())); !slices.Equal(got, tt.want) {
			t.Errorf("filter %q keeps %v, want %v", tt.query, got, tt.want)
		}
	}
}

func TestFilterChips(t *testing.T) {
	tests := []struct{ query, want string }{
		{"", ""},
		{"is:private fork:false archived:false template:true language:go sort:stars-desc",
			"private · no forks · no archived · templates · go · stars ↓"},
		{"gh tui sort:name-asc", `"gh tui" · name ↑`},
		{"topic:cli", "topic:cli"},
	}
	for _, tt := range tests {
		f := ParseFilter(tt.query)
		if got := f.Chips(ui.NewIcons(config.IconsUnicode)); got != tt.want {
			t.Errorf("chips of %q = %q, want %q", tt.query, got, tt.want)
		}
	}
	f := ParseFilter("gh tui sort:name-asc")
	if got, want := f.Chips(ui.NewIcons(config.IconsASCII)), `"gh tui" - name ^`; got != want {
		t.Errorf("ASCII chips = %q, want %q", got, want)
	}
}
