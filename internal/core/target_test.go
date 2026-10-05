package core

import (
	"errors"
	"strings"
	"testing"
)

func TestParseTarget(t *testing.T) {
	repo := RepoRef{Owner: "eggzec", Name: "gh-tui"}
	tests := []struct {
		name string
		in   string
		host string
		want Target
		err  string // a substring of the error; empty when none is wanted
	}{
		{name: "repo", in: "eggzec/gh-tui", want: Target{Repo: repo}},
		{name: "repo and number", in: "eggzec/gh-tui#12", want: Target{Repo: repo, Number: 12}},
		{name: "number", in: "#12", want: Target{Number: 12}},
		{name: "leading zeros", in: "#007", want: Target{Number: 7}},
		{name: "largest number", in: "#2147483647", want: Target{Number: 2147483647}},
		{name: "spaces trimmed", in: " \t eggzec/gh-tui#3 \n", want: Target{Repo: repo, Number: 3}},
		{name: "case kept", in: "EggZec/GH-TUI", want: Target{Repo: RepoRef{Owner: "EggZec", Name: "GH-TUI"}}},
		{name: "dotted name", in: "eggzec/.github", want: Target{Repo: RepoRef{Owner: "eggzec", Name: ".github"}}},

		{name: "repo link", in: "https://github.com/eggzec/gh-tui", want: Target{Repo: repo}},
		{name: "repo link slash", in: "https://github.com/eggzec/gh-tui/", want: Target{Repo: repo}},
		{name: "clone link", in: "https://github.com/eggzec/gh-tui.git", want: Target{Repo: repo}},
		{name: "pull link", in: "https://github.com/eggzec/gh-tui/pull/12", want: Target{Repo: repo, Number: 12, Kind: KindPull}},
		{name: "pulls link", in: "https://github.com/eggzec/gh-tui/pulls/12", want: Target{Repo: repo, Number: 12, Kind: KindPull}},
		{name: "issue link", in: "https://github.com/eggzec/gh-tui/issues/12", want: Target{Repo: repo, Number: 12, Kind: KindIssue}},
		{name: "pull files", in: "https://github.com/eggzec/gh-tui/pull/12/files", want: Target{Repo: repo, Number: 12, Kind: KindPull}},
		{name: "commit in pull", in: "https://github.com/eggzec/gh-tui/pull/12/commits/abc123", want: Target{Repo: repo, Number: 12, Kind: KindPull}},
		{name: "comment fragment", in: "https://github.com/eggzec/gh-tui/issues/12#issuecomment-1", want: Target{Repo: repo, Number: 12, Kind: KindIssue}},
		{name: "query", in: "https://github.com/eggzec/gh-tui/pull/12?w=1", want: Target{Repo: repo, Number: 12, Kind: KindPull}},
		{name: "repo fragment", in: "https://github.com/eggzec/gh-tui#readme", want: Target{Repo: repo}},
		{name: "http", in: "http://github.com/eggzec/gh-tui/pull/12", want: Target{Repo: repo, Number: 12, Kind: KindPull}},
		{name: "no scheme", in: "github.com/eggzec/gh-tui#3", want: Target{Repo: repo}},
		{name: "no scheme issue", in: "github.com/eggzec/gh-tui/issues/3", want: Target{Repo: repo, Number: 3, Kind: KindIssue}},
		{name: "host case", in: "HTTPS://GitHub.com/eggzec/gh-tui", want: Target{Repo: repo}},
		{name: "www", in: "https://www.github.com/eggzec/gh-tui", want: Target{Repo: repo}},
		{name: "link case kept", in: "https://github.com/EggZec/GH-TUI/pull/1", want: Target{Repo: RepoRef{Owner: "EggZec", Name: "GH-TUI"}, Number: 1, Kind: KindPull}},
		{name: "link spaces", in: "  https://github.com/eggzec/gh-tui/pull/12  ", want: Target{Repo: repo, Number: 12, Kind: KindPull}},
		{name: "enterprise", in: "https://ghe.example.com/eggzec/gh-tui/pull/5", host: "ghe.example.com", want: Target{Repo: repo, Number: 5, Kind: KindPull}},
		{name: "enterprise no scheme", in: "ghe.example.com/eggzec/gh-tui", host: "ghe.example.com", want: Target{Repo: repo}},
		{name: "enterprise port", in: "https://ghe.example.com:8443/eggzec/gh-tui", host: "ghe.example.com:8443", want: Target{Repo: repo}},
		{name: "enterprise short", in: "eggzec/gh-tui#2", host: "ghe.example.com", want: Target{Repo: repo, Number: 2}},
		{name: "dotless host", in: "ghe/eggzec/gh-tui", host: "ghe", want: Target{Repo: repo}},
		{name: "owner named like dotless host", in: "ghe/gh-tui", host: "ghe", want: Target{Repo: RepoRef{Owner: "ghe", Name: "gh-tui"}}},
		{name: "short clone name", in: "eggzec/gh-tui.git", want: Target{Repo: repo}},
		{name: "short clone name number", in: "eggzec/gh-tui.git#4", want: Target{Repo: repo, Number: 4}},
		{name: "default port", in: "https://github.com:443/eggzec/gh-tui/pull/12", want: Target{Repo: repo, Number: 12, Kind: KindPull}},
		{name: "default port no scheme", in: "github.com:443/eggzec/gh-tui", want: Target{Repo: repo}},
		{name: "default http port", in: "http://github.com:80/eggzec/gh-tui", want: Target{Repo: repo}},
		{name: "enterprise default port", in: "https://ghe.example.com:443/eggzec/gh-tui", host: "ghe.example.com", want: Target{Repo: repo}},
		{name: "managed user", in: "octocat_acme/gh-tui", want: Target{Repo: RepoRef{Owner: "octocat_acme", Name: "gh-tui"}}},
		{name: "api repo", in: "https://api.github.com/repos/eggzec/gh-tui", want: Target{Repo: repo}},
		{name: "api pull", in: "https://api.github.com/repos/eggzec/gh-tui/pulls/12", want: Target{Repo: repo, Number: 12, Kind: KindPull}},
		{name: "api issue", in: "api.github.com/repos/eggzec/gh-tui/issues/12", want: Target{Repo: repo, Number: 12, Kind: KindIssue}},
		{name: "api pull files", in: "https://api.github.com/repos/eggzec/gh-tui/pulls/12/files?per_page=1", want: Target{Repo: repo, Number: 12, Kind: KindPull}},
		{name: "api tenant", in: "https://api.acme.ghe.com/repos/eggzec/gh-tui/issues/3", host: "acme.ghe.com", want: Target{Repo: repo, Number: 3, Kind: KindIssue}},
		{name: "enterprise api repo", in: "https://ghe.example.com/api/v3/repos/Org/Repo", host: "ghe.example.com", want: Target{Repo: RepoRef{Owner: "Org", Name: "Repo"}}},
		{name: "enterprise api pull", in: "https://ghe.example.com/api/v3/repos/eggzec/gh-tui/pulls/12", host: "ghe.example.com", want: Target{Repo: repo, Number: 12, Kind: KindPull}},
		{name: "enterprise api issue", in: "ghe.example.com/api/v3/repos/eggzec/gh-tui/issues/12/", host: "ghe.example.com", want: Target{Repo: repo, Number: 12, Kind: KindIssue}},
		{name: "api owner on github.com", in: "https://github.com/api/v3", want: Target{Repo: RepoRef{Owner: "api", Name: "v3"}}},

		{name: "api root", in: "https://api.github.com/", err: "not an API link to an owner, repository, pull request or issue"},
		{name: "api commits", in: "https://api.github.com/repos/eggzec/gh-tui/commits/abc", err: "not an API link to an owner, repository, pull request or issue"},
		{name: "api pulls list", in: "https://api.github.com/repos/eggzec/gh-tui/pulls", err: "not an API link to an owner, repository, pull request or issue"},
		{name: "api not a number", in: "https://api.github.com/repos/eggzec/gh-tui/issues/x", err: "not an API link to an owner, repository, pull request or issue"},
		{name: "api issue comment", in: "https://api.github.com/repos/eggzec/gh-tui/issues/comments/5", err: "not an API link to an owner, repository, pull request or issue"},
		{name: "api pull comment", in: "https://api.github.com/repos/eggzec/gh-tui/pulls/comments/5", err: "not an API link to an owner, repository, pull request or issue"},
		{name: "api zero", in: "https://api.github.com/repos/eggzec/gh-tui/issues/0", err: "issue numbers"},
		{name: "enterprise api graphql", in: "https://ghe.example.com/api/graphql", host: "ghe.example.com", err: "not an API link to an owner, repository, pull request or issue"},
		{name: "api of another host", in: "https://api.github.com/repos/eggzec/gh-tui", host: "ghe.example.com", err: "not a link to ghe.example.com"},
		{name: "empty", in: "", err: "nothing to open"},
		{name: "blank", in: "   ", err: "nothing to open"},
		{name: "owner only", in: "eggzec", err: "not a repository"},
		{name: "empty name", in: "eggzec/", err: "not a repository"},
		{name: "empty owner", in: "/gh-tui", err: "not a repository"},
		{name: "three parts", in: "a/b/c", err: "not a repository"},
		{name: "space in owner", in: "egg zec/gh-tui", err: "not a repository"},
		{name: "bad name char", in: "eggzec/gh~tui", err: "not a repository"},
		{name: "unicode name", in: "eggzec/gh-tüi", err: "not a repository"},
		{name: "leading hyphen", in: "-eggzec/gh-tui", err: "not a repository"},
		{name: "dot name", in: "eggzec/..", err: "not a repository"},
		{name: "dotted owner", in: "egg.zec/gh-tui", err: "not a link to github.com"},
		{name: "dotted word", in: "foo.bar", err: "not a repository"},
		{name: "host alone", in: "github.com", err: "not a repository"},
		{name: "dotted number", in: "#1.2", err: "not an issue number"},
		{name: "number with slash", in: "#1/2.3", err: "not an issue number"},
		{name: "repo dotted number", in: "eggzec/gh-tui#1.2", err: "not an issue number"},
		{name: "hash only", in: "#", err: "missing issue number"},
		{name: "repo hash", in: "eggzec/gh-tui#", err: "missing issue number"},
		{name: "zero", in: "#0", err: "issue numbers are positive"},
		{name: "zeros", in: "#000", err: "issue numbers are positive"},
		{name: "negative", in: "#-3", err: "issue numbers are positive"},
		{name: "plus", in: "#+3", err: "not an issue number"},
		{name: "letters", in: "#abc", err: "not an issue number"},
		{name: "two hashes", in: "eggzec/gh-tui#1#2", err: "not an issue number"},
		{name: "inner space", in: "eggzec/gh-tui# 3", err: "not an issue number"},
		{name: "overflow int32", in: "#2147483648", err: "issue number too large"},
		{name: "overflow int64", in: "#99999999999999999999999", err: "issue number too large"},

		{name: "twice .git", in: "eggzec/gh-tui.git.git", err: `may not end in ".git"`},
		{name: "other port", in: "https://github.com:8443/eggzec/gh-tui", err: "not a link to github.com"},
		{name: "https port on http", in: "http://github.com:443/eggzec/gh-tui", err: "not a link to github.com"},
		{name: "other host", in: "https://gitlab.com/eggzec/gh-tui", err: "not a link to github.com"},
		{name: "other host no scheme", in: "gitlab.com/eggzec/gh-tui", err: "not a link to github.com"},
		{name: "github on enterprise", in: "https://github.com/eggzec/gh-tui", host: "ghe.example.com", err: "not a link to ghe.example.com"},
		{name: "enterprise wrong port", in: "https://ghe.example.com:8443/eggzec/gh-tui", host: "ghe.example.com", err: "not a link to"},
		{name: "look-alike host", in: "https://github.com.evil.example/eggzec/gh-tui", err: "not a link to github.com"},
		{name: "gist", in: "https://gist.github.com/eggzec/0123abcd", err: "not a link to github.com"},
		{name: "enterprise gist", in: "https://ghe.example.com/gist/eggzec/0123abcd", host: "ghe.example.com", err: "not a repository link"},
		{name: "ftp", in: "ftp://github.com/eggzec/gh-tui", err: "not a web link"},
		{name: "host only", in: "https://github.com", err: "not a repository link"},
		{name: "settings", in: "https://github.com/settings/profile", err: "not a repository link"},
		{name: "empty segment", in: "https://github.com//gh-tui", err: "not a repository link"},
		{name: "tree", in: "https://github.com/eggzec/gh-tui/tree/main/src", want: Target{Repo: repo}},
		{name: "blob", in: "https://github.com/eggzec/gh-tui/blob/main/x.go#L3", want: Target{Repo: repo}},
		{name: "actions", in: "https://github.com/eggzec/gh-tui/actions", want: Target{Repo: repo}},
		{name: "commit", in: "https://github.com/eggzec/gh-tui/commit/abc123", want: Target{Repo: repo}},
		{name: "release", in: "https://github.com/eggzec/gh-tui/releases/tag/v1", want: Target{Repo: repo}},
		{name: "issue list", in: "https://github.com/eggzec/gh-tui/issues", want: Target{Repo: repo}},
		{name: "pull list", in: "https://github.com/eggzec/gh-tui/pulls?q=is%3Aopen", want: Target{Repo: repo}},
		{name: "new issue", in: "https://github.com/eggzec/gh-tui/issues/new", want: Target{Repo: repo}},
		{name: "new pull", in: "https://github.com/eggzec/gh-tui/pull/new/branch", want: Target{Repo: repo}},
		{name: "discussion", in: "https://github.com/eggzec/gh-tui/discussions/4", want: Target{Repo: repo}},
		{name: "clone link pull", in: "https://github.com/eggzec/gh-tui.git/pull/1", want: Target{Repo: repo, Number: 1, Kind: KindPull}},
		{name: "clone link tree", in: "https://github.com/eggzec/gh-tui.git/tree/main", want: Target{Repo: repo}},
		{name: "link bad owner", in: "https://github.com/egg.zec/gh-tui", err: "not a repository"},
		{name: "link zero", in: "https://github.com/eggzec/gh-tui/pull/0", err: "issue numbers are positive"},
		{name: "link negative", in: "https://github.com/eggzec/gh-tui/issues/-1", want: Target{Repo: repo}},
		{name: "link zeros", in: "https://github.com/eggzec/gh-tui/issues/000", err: "issue numbers are positive"},
		{name: "clone link only", in: "https://github.com/eggzec/.git", err: "not a repository"},
		{name: "link empty number", in: "https://github.com/eggzec/gh-tui/pull//12", err: "missing number after /pull/"},
		{name: "link overflow", in: "https://github.com/eggzec/gh-tui/pull/99999999999", err: "issue number too large"},
		{name: "owner", in: "@octocat", want: Target{Owner: "octocat"}},
		{name: "owner case kept", in: "@OctoCat", want: Target{Owner: "OctoCat"}},
		{name: "owner spaces", in: " @octocat ", want: Target{Owner: "octocat"}},
		{name: "owner underscore", in: "@octo_cat", want: Target{Owner: "octo_cat"}},
		{name: "owner managed user", in: "@octocat_acme", want: Target{Owner: "octocat_acme"}},
		{name: "profile managed user", in: "https://acme.ghe.com/octocat_acme", host: "acme.ghe.com", want: Target{Owner: "octocat_acme"}},
		{name: "profile real account like a page", in: "https://github.com/integrations", want: Target{Owner: "integrations"}},
		{name: "owner hyphen", in: "@octo-org", want: Target{Owner: "octo-org"}},
		{name: "owner digits", in: "@0x1", want: Target{Owner: "0x1"}},
		{name: "owner one char", in: "@a", want: Target{Owner: "a"}},
		{name: "owner longest", in: "@aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", want: Target{Owner: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}},
		{name: "profile link", in: "https://github.com/octocat", want: Target{Owner: "octocat"}},
		{name: "profile link slash", in: "https://github.com/octocat/", want: Target{Owner: "octocat"}},
		{name: "profile tab", in: "github.com/octocat?tab=repositories", want: Target{Owner: "octocat"}},
		{name: "profile fragment", in: "https://github.com/octocat#readme", want: Target{Owner: "octocat"}},
		{name: "profile www", in: "https://www.github.com/octocat", want: Target{Owner: "octocat"}},
		{name: "profile http", in: "http://github.com/octocat", want: Target{Owner: "octocat"}},
		{name: "profile named api", in: "https://github.com/api", want: Target{Owner: "api"}},
		{name: "orgs link", in: "https://github.com/orgs/github", want: Target{Owner: "github"}},
		{name: "orgs people", in: "https://github.com/orgs/github/people", want: Target{Owner: "github"}},
		{name: "orgs team", in: "https://github.com/orgs/github/teams/x", want: Target{Owner: "github"}},
		{name: "orgs case", in: "https://github.com/ORGS/github", want: Target{Owner: "github"}},
		{name: "users link", in: "https://github.com/users/octocat", want: Target{Owner: "octocat"}},
		{name: "users projects", in: "github.com/users/octocat/projects/1", want: Target{Owner: "octocat"}},
		{name: "enterprise profile", in: "https://ghe.example.com/octocat", host: "ghe.example.com", want: Target{Owner: "octocat"}},
		{name: "enterprise profile no scheme", in: "ghe.example.com/octocat", host: "ghe.example.com", want: Target{Owner: "octocat"}},
		{name: "enterprise profile port", in: "https://ghe.example.com:8443/octocat", host: "ghe.example.com:8443", want: Target{Owner: "octocat"}},
		{name: "enterprise orgs", in: "https://ghe.example.com/orgs/acme", host: "ghe.example.com", want: Target{Owner: "acme"}},
		{name: "api user", in: "https://api.github.com/users/octocat", want: Target{Owner: "octocat"}},
		{name: "api org", in: "api.github.com/orgs/github", want: Target{Owner: "github"}},
		{name: "api org repos", in: "https://api.github.com/orgs/github/repos", want: Target{Owner: "github"}},
		{name: "api tenant user", in: "https://api.acme.ghe.com/users/octocat", host: "acme.ghe.com", want: Target{Owner: "octocat"}},
		{name: "enterprise api user", in: "https://ghe.example.com/api/v3/users/octocat", host: "ghe.example.com", want: Target{Owner: "octocat"}},
		{name: "enterprise api orgs", in: "https://ghe.example.com/api/v3/orgs/acme", host: "ghe.example.com", want: Target{Owner: "acme"}},

		{name: "owner bare", in: "octocat", err: "not a repository"},
		{name: "owner at only", in: "@", err: "missing login after '@'"},
		{name: "owner too long", in: "@aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", err: "at most 39 characters"},
		{name: "owner leading hyphen", in: "@-octo", err: "may not start or end with '-'"},
		{name: "owner trailing hyphen", in: "@octo-", err: "may not start or end with '-'"},
		{name: "owner dot", in: "@octo.cat", err: "only letters, digits, '-' and '_'"},
		{name: "owner slash", in: "@octocat/hello", err: "only letters, digits, '-' and '_'"},
		{name: "owner number", in: "@octocat#3", err: "only letters, digits, '-' and '_'"},
		{name: "owner unicode", in: "@öcto", err: "only letters, digits, '-' and '_'"},
		{name: "owner space", in: "@octo cat", err: "only letters, digits, '-' and '_'"},
		{name: "owner reserved", in: "@settings", err: "settings is a page of GitHub"},
		{name: "owner reserved case", in: "@Marketplace", err: "Marketplace is a page of GitHub"},
		{name: "profile reserved", in: "https://github.com/notifications", err: "notifications is a page of GitHub"},
		{name: "profile readme", in: "https://github.com/readme", err: "readme is a page of GitHub"},
		{name: "owner reserved copilot", in: "@copilot", err: "copilot is a page of GitHub"},
		{name: "profile reserved orgs", in: "https://github.com/orgs", err: "orgs is a page of GitHub"},
		{name: "profile bad login", in: "https://github.com/octo.cat", err: "only letters, digits, '-' and '_'"},
		{name: "profile too long", in: "https://github.com/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", err: "at most 39 characters"},
		{name: "orgs empty login", in: "https://github.com/orgs//people", err: "not a repository link"},
		{name: "orgs bad login", in: "https://github.com/orgs/-acme", err: "may not start or end with '-'"},
		{name: "users reserved", in: "https://github.com/users/settings", err: "settings is a page of GitHub"},
		{name: "enterprise profile reserved", in: "https://ghe.example.com/explore", host: "ghe.example.com", err: "explore is a page of GitHub"},
		{name: "profile other host", in: "https://gitlab.com/octocat", err: "not a link to github.com"},
		{name: "api bad login", in: "https://api.github.com/users/octo-", err: "may not start or end with '-'"},
		{name: "api users list", in: "https://api.github.com/users", err: "not an API link to an owner, repository, pull request or issue"},
		{name: "api users empty", in: "https://api.github.com/users//repos", err: "not an API link to an owner, repository, pull request or issue"},
		{name: "api orgs case", in: "https://api.github.com/ORGS/github", err: "not an API link to an owner, repository, pull request or issue"},
		{name: "long bad owner", in: strings.Repeat("-", 5000) + "/r", err: "may not start with '-'"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseTarget(tt.in, tt.host)
			if tt.err != "" {
				if err == nil || !strings.Contains(err.Error(), tt.err) {
					t.Fatalf("ParseTarget(%q, %q) error = %v, want one containing %q", tt.in, tt.host, err, tt.err)
				}
				// The reason is for the user, who knows what they typed, so
				// it is short and quotes none of it.
				e, ok := errors.AsType[*TargetError](err)
				if in := strings.TrimSpace(tt.in); !ok || e.Input != in || e.Reason == "" || len(e.Reason) > 60 || strings.Contains(e.Reason, `"`) {
					t.Errorf("ParseTarget(%q, %q) error = %#v, want a *TargetError of the input with a reason that doesn't repeat it", tt.in, tt.host, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseTarget(%q, %q) error = %v", tt.in, tt.host, err)
			}
			if got != tt.want {
				t.Errorf("ParseTarget(%q, %q) = %+v, want %+v", tt.in, tt.host, got, tt.want)
			}
		})
	}
}

// TestParseTargetSaysRepoOnce checks that an error about a repository
// names it once, with what is wrong, rather than repeating itself.
func TestParseTargetSaysRepoOnce(t *testing.T) {
	for in, want := range map[string]string{
		"eggzec":                        `not a repository: "eggzec": want owner/name`,
		"-eggzec/gh-tui":                `not a repository: "-eggzec/gh-tui": owner "-eggzec" may not start with '-'`,
		"https://github.com/a/b~c/pull": `not a repository: "a/b~c": name "b~c" may hold only letters, digits, '.', '-' and '_'`,
	} {
		_, err := ParseTarget(in, "")
		if err == nil || err.Error() != want {
			t.Errorf("ParseTarget(%q) error = %v, want %q", in, err, want)
		}
	}
}

func TestTargetString(t *testing.T) {
	repo := RepoRef{Owner: "eggzec", Name: "gh-tui"}
	tests := []struct {
		t    Target
		want string
	}{
		{Target{Repo: repo}, "eggzec/gh-tui"},
		{Target{Repo: repo, Number: 12}, "eggzec/gh-tui#12"},
		{Target{Number: 12}, "#12"},
		{Target{Repo: repo, Number: 12, Kind: KindPull}, "eggzec/gh-tui#12"},
		{Target{Number: 12, Kind: KindIssue}, "#12"},
		{Target{}, ""},
		{Target{Owner: "octocat"}, "@octocat"},
	}
	for _, tt := range tests {
		if got := tt.t.String(); got != tt.want {
			t.Errorf("%+v.String() = %q, want %q", tt.t, got, tt.want)
		}
	}
}

func TestTargetHas(t *testing.T) {
	repo := RepoRef{Owner: "eggzec", Name: "gh-tui"}
	tests := []struct {
		t                      Target
		repo, hasNumber, owner bool
	}{
		{Target{}, false, false, false},
		{Target{Repo: repo}, true, false, false},
		{Target{Number: 3}, false, true, false},
		{Target{Repo: repo, Number: 3}, true, true, false},
		{Target{Owner: "octocat"}, false, false, true},
	}
	for _, tt := range tests {
		if got := tt.t.HasRepo(); got != tt.repo {
			t.Errorf("%+v.HasRepo() = %v, want %v", tt.t, got, tt.repo)
		}
		if got := tt.t.HasNumber(); got != tt.hasNumber {
			t.Errorf("%+v.HasNumber() = %v, want %v", tt.t, got, tt.hasNumber)
		}
		if got := tt.t.HasOwner(); got != tt.owner {
			t.Errorf("%+v.HasOwner() = %v, want %v", tt.t, got, tt.owner)
		}
	}
}

func TestTargetSame(t *testing.T) {
	a := Target{Repo: RepoRef{Owner: "eggzec", Name: "gh-tui"}, Number: 3}
	if !a.Same(Target{Repo: RepoRef{Owner: "EGGZEC", Name: "Gh-Tui"}, Number: 3}) {
		t.Error("targets differing only in case are not the same")
	}
	if a.Same(Target{Repo: a.Repo, Number: 4}) {
		t.Error("targets with different numbers are the same")
	}
	if a.Same(Target{Number: 3}) {
		t.Error("a target with a repository is the same as one without")
	}
	if !a.Same(Target{Repo: a.Repo, Number: 3, Kind: KindPull}) {
		t.Error("a kind hint makes targets differ")
	}
	if !(Target{Owner: "octocat"}).Same(Target{Owner: "OctoCat"}) {
		t.Error("owners differing only in case are not the same")
	}
	if (Target{Owner: "octocat"}).Same(Target{Owner: "octo-org"}) {
		t.Error("different owners are the same")
	}
}

func FuzzParseTarget(f *testing.F) {
	for _, s := range []string{
		"eggzec/gh-tui", "eggzec/gh-tui#12", "#12", "#0", "#-1", "#99999999999",
		"https://github.com/eggzec/gh-tui/pull/12/files",
		"http://github.com/eggzec/gh-tui/issues/3#issuecomment-1",
		"github.com/eggzec/gh-tui#3", "https://gist.github.com/a/b",
		"https://github.com/eggzec/gh-tui.git", "  a/b  ", "ghe/a/b", "://",
		"@octocat", "@", "https://github.com/octocat", "https://github.com/orgs/github/people",
		"https://api.github.com/users/octocat",
	} {
		f.Add(s, "")
		f.Add(s, "ghe")
	}
	f.Fuzz(func(t *testing.T, s, host string) {
		got, err := ParseTarget(s, host)
		if err != nil {
			if got != (Target{}) {
				t.Errorf("ParseTarget(%q, %q) = %+v with error %v, want the zero Target", s, host, got, err)
			}
			return
		}
		if got.HasOwner() && (got.HasRepo() || got.HasNumber()) {
			t.Fatalf("ParseTarget(%q, %q) = %+v, an owner with a repository or number", s, host, got)
		}
		if !got.HasRepo() && !got.HasNumber() && !got.HasOwner() {
			t.Fatalf("ParseTarget(%q, %q) = %+v, names nothing", s, host, got)
		}
		if got.Kind != "" && (!got.Kind.Known() || !got.HasNumber()) {
			t.Fatalf("ParseTarget(%q, %q) = %+v, a kind without a number", s, host, got)
		}
		// The short form has no kind, so it comes back unknown.
		want := got
		want.Kind = ""
		back, err := ParseTarget(got.String(), host)
		if err != nil || back != want {
			t.Errorf("ParseTarget(%q, %q) = %+v, but its String %q parses to %+v, %v", s, host, got, got.String(), back, err)
		}
	})
}
