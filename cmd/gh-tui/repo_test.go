package main

import (
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
)

func TestStartRepo(t *testing.T) {
	cwd := core.RepoRef{Owner: "eggzec", Name: "gh-tui"}
	pin := core.RepoRef{Owner: "charmbracelet", Name: "bubbletea"}
	inRepo := func() (core.RepoRef, bool) { return cwd, true }
	outside := func() (core.RepoRef, bool) { return core.RepoRef{}, false }

	tests := []struct {
		name    string
		arg     string
		current func() (core.RepoRef, bool)
		pinned  []core.RepoRef
		want    core.RepoRef
		wantErr bool
	}{
		{name: "argument wins", arg: "cli/cli", current: inRepo, pinned: []core.RepoRef{pin}, want: core.RepoRef{Owner: "cli", Name: "cli"}},
		{name: "bad argument", arg: "cli", current: inRepo, wantErr: true},
		{name: "current directory", current: inRepo, pinned: []core.RepoRef{pin}, want: cwd},
		{name: "first pinned", current: outside, pinned: []core.RepoRef{pin, cwd}, want: pin},
		{name: "none", current: outside},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := startRepo(tt.arg, tt.current, tt.pinned)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("startRepo = %v, want %v", got, tt.want)
			}
		})
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
