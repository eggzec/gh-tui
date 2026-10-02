package ui

import (
	"testing"

	"charm.land/bubbles/v2/key"
)

func TestEmptyWords(t *testing.T) {
	off := key.NewBinding(key.WithKeys("]"), key.WithHelp("]", "next tab"))
	off.SetEnabled(false)
	tests := []struct {
		name, got, want string
	}{
		{"none", None("open pull requests"), "No open pull requests."},
		{"no match", NoMatch("issues", "F"), "No issues match the filters. Press F to clear them."},
		{"no match, unbound", NoMatch("issues", ""), "No issues match the filters."},
		{"press", Press(None("open issues"), "]", "show closed ones"), "No open issues. Press ] to show closed ones."},
		{"press, off", Press(None("open issues"), KeyOf(Icons{}, off), "show closed ones"), "No open issues."},
		{"press, unbound", Press(None("open issues"), KeyOf(Icons{}, key.NewBinding()), "show closed ones"), "No open issues."},
		{"no match, off", NoMatch("issues", KeyOf(Icons{}, off)), "No issues match the filters."},
		{"key of a binding", KeyOf(Icons{}, key.NewBinding(key.WithKeys("F"), key.WithHelp("F", "clear"))), "F"},
	}
	for _, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("%s: %q, want %q", tt.name, tt.got, tt.want)
		}
	}
}
