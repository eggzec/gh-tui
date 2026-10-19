package merge

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/exp/golden"

	"github.com/eggzec/gh-tui/internal/codeowners"
	"github.com/eggzec/gh-tui/internal/core"
)

// owned is a pull request that changes four files, three owned by people
// and a team and one by nobody.
func owned() Data {
	d := Data{Detail: detail(), Behind: -1}
	d.Detail.ReviewDecision = core.ReviewRequired
	d.Owners = Owners{
		State: OwnersLoaded,
		Files: []core.CommitFile{
			{Path: "api/server.go", Additions: 12, Deletions: 3},
			{Path: "api/routes.go", Additions: 4},
			{Path: "docs/guide.md", Additions: 1, Deletions: 1},
			{Path: "README.md", Additions: 2},
		},
		Owned: []codeowners.Owned{
			{Owner: codeowners.Owner{Kind: codeowners.OwnerUser, Name: "alice"}, Paths: []string{"api/routes.go", "api/server.go"}},
			{Owner: codeowners.Owner{Kind: codeowners.OwnerUser, Name: "bob"}, Paths: []string{"docs/guide.md"}},
			{Owner: codeowners.Owner{Kind: codeowners.OwnerTeam, Name: "acme/core"}, Paths: []string{"README.md"}},
		},
	}
	return d
}

func TestOwnersPreviewGoldens(t *testing.T) {
	t.Parallel()
	required := func(d Data) Data {
		d.Detail.Reviewers.Requested = []core.ReviewRequest{{User: user("alice", ""), CodeOwner: true}}
		return d
	}
	approved := func(d Data) Data {
		d.Detail.Reviewers.Verdicts = []core.Verdict{{Author: user("bob", ""), State: core.ReviewStateApproved}}
		return d
	}
	members := []core.Person{{Login: "carol"}, {Login: "dave"}}
	cases := []struct {
		name string
		d    Data
	}{
		{"required approved and plain", approved(required(owned()))},
		{"team asked with members", func() Data {
			d := required(owned())
			d.Detail.Reviewers.Requested = append(d.Detail.Reviewers.Requested,
				core.ReviewRequest{Team: "acme/core", TeamName: "Core team", CodeOwner: true})
			d.Owners.AddMembers("acme/core", members, false)
			return d
		}()},
		{"team approved by a member", func() Data {
			d := approved(owned())
			d.Detail.Reviewers.Verdicts = append(d.Detail.Reviewers.Verdicts,
				core.Verdict{Author: user("dave", ""), State: core.ReviewStateApproved})
			d.Owners.AddMembers("acme/core", members, false)
			return d
		}()},
		{"team without members", owned()},
		{"team members unavailable", func() Data {
			d := owned()
			d.Owners.AddMembers("acme/core", members, true)
			return d
		}()},
		{"no owner", func() Data { d := owned(); d.Owners.Owned = nil; return d }()},
		{"no required owner", approved(owned())},
		{"read failed", func() Data { d := owned(); d.Owners = Owners{State: OwnersFailed}; return d }()},
		{"loading", func() Data { d := owned(); d.Owners = Owners{State: OwnersLoading}; return d }()},
	}
	for _, theme := range []struct {
		name string
		dark bool
	}{{"dark", true}, {"light", false}} {
		for _, c := range cases {
			t.Run(theme.name+"/"+c.name, func(t *testing.T) {
				t.Parallel()
				o := opts(t, 80)
				o.Styles = styles(t, theme.dark)
				p, ok := PreviewOf(c.d, idReview, o)
				if !ok || p.Body == "" {
					t.Fatalf("no preview of the review row: %+v, %v", p, ok)
				}
				golden.RequireEqual(t, fmt.Sprintf("%s | %s\n%s\n", p.Title, p.Info, p.Body))
			})
		}
	}
}

func TestOwnersPreviewOrderAndNames(t *testing.T) {
	t.Parallel()
	d := owned()
	d.Detail.Reviewers.Requested = []core.ReviewRequest{{Team: "acme/core", TeamName: "Core team", CodeOwner: true}}
	p, _ := PreviewOf(d, idReview, opts(t, 80))
	if p.Title != "Required review · Core team" || p.Info != "CODEOWNERS" {
		t.Errorf("title and info = %q | %q; want the required team first", p.Title, p.Info)
	}
	body := plain(p.Body)
	for _, want := range []string{"Core team", "alice", "bob"} {
		if !strings.Contains(body, want) {
			t.Errorf("body lacks %q:\n%s", want, body)
		}
	}
	if a, b := strings.Index(body, "alice"), strings.Index(body, "bob"); a > b {
		t.Errorf("alice after bob in\n%s", body)
	}
}
