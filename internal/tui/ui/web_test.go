package ui

import "testing"

func TestProfileURL(t *testing.T) {
	for login, want := range map[string]string{
		"":                "",
		"ann":             "https://github.com/ann",
		"dependabot[bot]": "https://github.com/apps/dependabot",
	} {
		if got := ProfileURL("github.com", login); got != want {
			t.Errorf("ProfileURL(%q) = %q, want %q", login, got, want)
		}
	}
}

func TestWebURL(t *testing.T) {
	for host, want := range map[string]string{
		"":                     "https://github.com/cli/cli",
		"github.com":           "https://github.com/cli/cli",
		"ghe.example.com:8443": "https://ghe.example.com:8443/cli/cli",
		// A GitHub run locally for development serves plain HTTP, as gh has it.
		"github.localhost": "http://github.localhost/cli/cli",
	} {
		if got := WebURL(host, "cli/cli"); got != want {
			t.Errorf("WebURL(%q) = %q, want %q", host, got, want)
		}
	}
}
