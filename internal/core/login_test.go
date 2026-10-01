package core

import (
	"strings"
	"testing"
)

func TestValidLogin(t *testing.T) {
	for s, want := range map[string]bool{
		"octocat": true, "Octo-Cat": true, "ali_corp": true, "a": true, strings.Repeat("a", 64): true,
		"": false, strings.Repeat("a", 65): false, "octo cat": false, "octo\x1b[31mcat": false,
		"octo\ncat": false, "a@b": false, "ünïcode": false, "../x": false,
	} {
		if got := ValidLogin(s); got != want {
			t.Errorf("ValidLogin(%q) = %v, want %v", s, got, want)
		}
	}
}
