package github

import (
	"encoding/json"
	"testing"

	"github.com/eggzec/gh-tui/pkg/termtext/termtexttest"
)

// A repository's owner and name from a hostile server are cleaned where
// the answer is decoded, so no view draws them raw.
func TestRepoRefCleansHostile(t *testing.T) {
	ref := repoRef(termtexttest.HostileLogin, "re\x1b]0;pwned\apo\u202e")
	if ref.Owner != termtexttest.CleanLogin || ref.Name != "repo" {
		t.Errorf("repoRef = %+v, want %q/repo", ref, termtexttest.CleanLogin)
	}
	termtexttest.AssertClean(t, ref.String(), 1000)

	var p pull
	owner, _ := json.Marshal(termtexttest.Hostile)
	body := `{"number":1,"repository":{"name":"gh-tui","owner":{"login":` + string(owner) + `}}}`
	if err := json.Unmarshal([]byte(body), &p); err != nil {
		t.Fatal(err)
	}
	termtexttest.AssertClean(t, p.core().Repo.String(), 1000)

	if ref := repoRef("eggzec", "gh-tui"); ref.String() != "eggzec/gh-tui" {
		t.Errorf("repoRef of a real name = %v, want it as it is", ref)
	}
}
