package history

import (
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
)

// GitHub ignores the case of names, so the keys and tags of a repository
// do too, and a read under one spelling finds what another stored.
func TestKeysIgnoreCase(t *testing.T) {
	a := core.RepoRef{Owner: "eggzec", Name: "gh-tui"}
	b := core.RepoRef{Owner: "EggZec", Name: "GH-TUI"}
	for name, key := range map[string]func(core.RepoRef) string{
		"commits":  func(r core.RepoRef) string { return refPageKey(CommitsQuery{Repo: r, Ref: "main"}) },
		"branches": func(r core.RepoRef) string { return branchesKey(BranchesQuery{Repo: r}) },
		"compare":  func(r core.RepoRef) string { return compareKey(r, "main", "dev") },
		"tag":      repoTag,
		"sync":     SyncKey,
	} {
		if key(a) != key(b) {
			t.Errorf("%s keys differ: %q and %q", name, key(a), key(b))
		}
	}
}
