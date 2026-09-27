package ui

import "testing"

func TestWebURL(t *testing.T) {
	for host, want := range map[string]string{
		"":                     "https://github.com/cli/cli",
		"github.com":           "https://github.com/cli/cli",
		"ghe.example.com:8443": "https://ghe.example.com:8443/cli/cli",
	} {
		if got := WebURL(host, "cli/cli"); got != want {
			t.Errorf("WebURL(%q) = %q, want %q", host, got, want)
		}
	}
}
