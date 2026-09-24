package main

import (
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
)

func TestStartRepos(t *testing.T) {
	cwd := core.RepoRef{Owner: "eggzec", Name: "gh-tui"}
	inRepo := func() (core.RepoRef, bool) { return cwd, true }
	outside := func() (core.RepoRef, bool) { return core.RepoRef{}, false }

	tests := []struct {
		name    string
		arg     string
		current func() (core.RepoRef, bool)
		open    core.RepoRef
		here    core.RepoRef
		wantErr bool
	}{
		{name: "argument", arg: "cli/cli", current: inRepo, open: core.RepoRef{Owner: "cli", Name: "cli"}, here: cwd},
		{name: "bad argument", arg: "cli", current: inRepo, wantErr: true},
		{name: "current directory", current: inRepo, here: cwd},
		{name: "none", current: outside},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			open, here, err := startRepos(tt.arg, tt.current)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if open != tt.open || here != tt.here {
				t.Errorf("startRepos = %v, %v; want %v, %v", open, here, tt.open, tt.here)
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
