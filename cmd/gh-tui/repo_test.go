package main

import (
	"strings"
	"testing"

	"github.com/cli/go-gh/v2/pkg/repository"

	"github.com/eggzec/gh-tui/internal/core"
)

func TestStartRepos(t *testing.T) {
	ghTUI := core.RepoRef{Owner: "eggzec", Name: "gh-tui"}
	inRepo := func(host string) func() (repository.Repository, bool) {
		return func() (repository.Repository, bool) {
			return repository.Repository{Host: host, Owner: "eggzec", Name: "gh-tui"}, true
		}
	}
	outside := func() (repository.Repository, bool) { return repository.Repository{}, false }

	tests := []struct {
		name     string
		hostname string
		current  func() (repository.Repository, bool)
		// defaultHost is GH_HOST, else gh's default host.
		defaultHost string
		want        start
	}{
		{
			name: "--hostname", hostname: "ghe.corp", current: outside, defaultHost: "github.com",
			want: start{Host: "ghe.corp", From: "--hostname"},
		},
		{
			name: "--hostname over the current repository's host", hostname: "ghe.corp",
			current: inRepo("github.com"), defaultHost: "github.com",
			want: start{Host: "ghe.corp", From: "--hostname"},
		},
		{
			name: "--hostname of the current repository's host", hostname: "GitHub.com",
			current: inRepo("github.com"), defaultHost: "ghe.corp",
			want: start{Host: "github.com", Here: ghTUI, From: "--hostname"},
		},
		{
			name: "--hostname with a scheme and a slash", hostname: "https://GHE.corp/",
			current: outside, defaultHost: "github.com",
			want: start{Host: "ghe.corp", From: "--hostname"},
		},
		{
			name: "enterprise remote over GH_HOST", current: inRepo("ghe.corp"), defaultHost: "github.com",
			want: start{Host: "ghe.corp", Here: ghTUI, From: "repo"},
		},
		{
			name: "current repository", current: inRepo("github.com"), defaultHost: "ghe.corp",
			want: start{Host: "github.com", Here: ghTUI, From: "repo"},
		},
		{
			name: "GH_HOST or the default host", current: outside, defaultHost: "ghe.corp",
			want: start{Host: "ghe.corp", From: "hosts"},
		},
		{
			name: "github.com subdomain", current: inRepo("www.github.com"), defaultHost: "ghe.corp",
			want: start{Host: "github.com", Here: ghTUI, From: "repo"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := startRepos(tt.hostname, tt.current, func() (string, string) { return tt.defaultHost, "hosts" })
			if got != tt.want {
				t.Errorf("startRepos = %+v, want %+v", got, tt.want)
			}
		})
	}
}

// GH_REPO names the current repository, host and all, as gh reads it.
func TestCurrentRepoGHRepo(t *testing.T) {
	t.Setenv("GH_CONFIG_DIR", t.TempDir())
	t.Setenv("GH_REPO", "ghe.corp/eggzec/gh-tui")
	got := startRepos("", currentRepo, func() (string, string) { return "github.com", "default" })
	want := start{Host: "ghe.corp", Here: core.RepoRef{Owner: "eggzec", Name: "gh-tui"}, From: "repo"}
	if got != want {
		t.Errorf("startRepos with GH_REPO = %+v, want %+v", got, want)
	}
}

func TestParseRefs(t *testing.T) {
	refs, err := parseRefs([]string{"eggzec/gh-tui", "cli/cli"})
	if err != nil || len(refs) != 2 || refs[1] != (core.RepoRef{Owner: "cli", Name: "cli"}) {
		t.Errorf("parseRefs = %v, %v", refs, err)
	}
	if _, err := parseRefs([]string{"nope"}); err == nil {
		t.Error("parseRefs accepted a ref without an owner")
	}
}

func TestCheckHostname(t *testing.T) {
	for _, h := range []string{"github.com", "GHE.example.com", "ghe.example.com:8443", "https://ghe.example.com/", "github.localhost"} {
		if err := checkHostname(h); err != nil {
			t.Errorf("checkHostname(%q) = %v, want nil", h, err)
		}
	}
	for _, h := range []string{"ghe.example.com/api/v3", "https://ghe.example.com/api/v3/", "u@ghe.example.com", "ghe.example.com?x=1", "ghe.example.com#x", "ghe example.com", " "} {
		if err := checkHostname(h); err == nil || !strings.Contains(err.Error(), "names no host") {
			t.Errorf("checkHostname(%q) = %v, want it to name no host", h, err)
		}
	}
}
