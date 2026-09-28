package files

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
		"tree": func(r core.RepoRef) string { return treeKey(r, "main") },
		"all":  func(r core.RepoRef) string { return allKey(r, "main") },
		"blob": func(r core.RepoRef) string { return blobKey(r, "abc") },
		"ref":  func(r core.RepoRef) string { return refKey(kindTree, r, "main") },
		"tag":  repoTag,
		"sync": SyncKey,
	} {
		if key(a) != key(b) {
			t.Errorf("%s keys differ: %q and %q", name, key(a), key(b))
		}
	}
}
