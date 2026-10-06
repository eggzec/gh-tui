package ui

import (
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
)

func TestSelections(t *testing.T) {
	repo := core.RepoRef{Owner: "cli", Name: "cli"}
	issue := core.Issue{Repo: repo, Number: 5, URL: "https://github.com/cli/cli/issues/5"}
	authored := issue
	authored.Author = core.User{Login: "mona"}
	bot := issue
	bot.Author = core.User{Login: "dependabot", Bot: true}
	tests := []struct {
		name string
		got  Selection
		want Selection
	}{
		{
			name: "repository hit",
			got:  HitSelection(core.SearchHit{Kind: core.SearchRepos, Repo: core.Repo{Ref: repo}}, "https://github.com/cli/cli"),
			want: Selection{What: "repository", URL: "https://github.com/cli/cli", Repo: repo, Owner: "cli"},
		},
		{
			name: "issue hit",
			got:  HitSelection(core.SearchHit{Kind: core.SearchIssues, Issue: authored}, ""),
			want: Selection{What: "issue", URL: issue.URL, Repo: repo, Number: 5, Owner: "mona"},
		},
		{
			name: "issue hit of a deleted account",
			got:  HitSelection(core.SearchHit{Kind: core.SearchIssues, Issue: issue}, ""),
			want: Selection{What: "issue", URL: issue.URL, Repo: repo, Number: 5},
		},
		{
			name: "pull request hit of an app",
			got:  HitSelection(core.SearchHit{Kind: core.SearchPulls, Issue: bot}, ""),
			want: Selection{What: "pull request", URL: issue.URL, Repo: repo, Number: 5},
		},
		{
			name: "pull request hit",
			got:  HitSelection(core.SearchHit{Kind: core.SearchPulls, Issue: authored}, ""),
			want: Selection{What: "pull request", URL: issue.URL, Repo: repo, Number: 5, Owner: "mona"},
		},
		{
			name: "commit thread",
			got:  NotificationSelection(core.Notification{Repo: repo, Subject: core.Subject{Type: core.SubjectCommit, WebURL: "https://github.com/cli/cli/commit/abc", SHA: "abc"}}),
			want: Selection{What: "commit", URL: "https://github.com/cli/cli/commit/abc", Repo: repo, SHA: "abc", Owner: "cli"},
		},
		{
			name: "unknown thread",
			got:  NotificationSelection(core.Notification{Repo: repo, Subject: core.Subject{Type: "RepositoryVulnerabilityAlert", WebURL: "https://github.com/cli/cli"}}),
			want: Selection{What: "notification", URL: "https://github.com/cli/cli", Repo: repo, Owner: "cli"},
		},
	}
	for _, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("%s: %+v, want %+v", tt.name, tt.got, tt.want)
		}
	}
}
