package main

import (
	"slices"
	"testing"

	"github.com/eggzec/gh-tui/internal/target"
)

func TestRestartArgs(t *testing.T) {
	t.Parallel()
	to := target.Target{User: "hubot", Host: "corp.ghe.com"}
	tests := []struct {
		name string
		in   []string
		want []string
	}{
		{"none", nil, []string{"--hostname", "corp.ghe.com", "--user", "hubot"}},
		{"keeps debug", []string{"--debug"}, []string{"--debug", "--hostname", "corp.ghe.com", "--user", "hubot"}},
		{"replaces separate values", []string{"--hostname", "github.com", "--user", "me", "--debug"},
			[]string{"--debug", "--hostname", "corp.ghe.com", "--user", "hubot"}},
		{"replaces attached values", []string{"-hostname=github.com", "--user=me"},
			[]string{"--hostname", "corp.ghe.com", "--user", "hubot"}},
	}
	for _, tt := range tests {
		if got := restartArgs(tt.in, to); !slices.Equal(got, tt.want) {
			t.Errorf("%s: restartArgs(%q) = %q, want %q", tt.name, tt.in, got, tt.want)
		}
	}
}
