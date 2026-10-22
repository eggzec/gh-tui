package pulls

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/codeowners"
	"github.com/eggzec/gh-tui/internal/core"
	pullsvc "github.com/eggzec/gh-tui/internal/service/pulls"
	"github.com/eggzec/gh-tui/pkg/bubbles/panes"
)

// ownersFake serves a CODEOWNERS file once Owners has read it, as the service
// does, and the members of a team the same way.
type ownersFake struct {
	mu      sync.Mutex
	text    string
	err     error
	read    bool
	members map[string][]core.Person
	reads   int
}

func (o *ownersFake) CachedOwners(pullsvc.OwnersQuery) (pullsvc.OwnersFile, bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if !o.read {
		return pullsvc.OwnersFile{}, false
	}
	return pullsvc.OwnersFile{File: codeowners.Parse(o.text), Path: ".github/CODEOWNERS"}, true
}

func (o *ownersFake) Owners(context.Context, pullsvc.OwnersQuery) (pullsvc.OwnersFile, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.reads++
	if o.err != nil {
		return pullsvc.OwnersFile{}, o.err
	}
	o.read = true
	return pullsvc.OwnersFile{File: codeowners.Parse(o.text), Path: ".github/CODEOWNERS"}, nil
}

func (o *ownersFake) CachedTeamMembers(pullsvc.TeamQuery) (pullsvc.TeamMembers, bool) {
	return pullsvc.TeamMembers{}, false
}

func (o *ownersFake) TeamMembers(_ context.Context, q pullsvc.TeamQuery) (pullsvc.TeamMembers, error) {
	return pullsvc.TeamMembers{Members: o.members[q.Org+"/"+q.Slug]}, nil
}

// reviewRequired is a modal on a pull request that waits for a code owner,
// which changes three files, and reads the code owners from fake.
func reviewRequired(t *testing.T, fake *ownersFake) (*panesModal, tea.Cmd) {
	t.Helper()
	m, svc := mergePanes(t, func(d *core.PullRequestDetail) {
		d.ReviewDecision = core.ReviewRequired
		d.Reviewers.Requested = []core.ReviewRequest{{User: core.User{Login: "alice"}, CodeOwner: true}}
	})
	svc.changed = []core.CommitFile{
		{Path: "api/server.go", Additions: 12, Deletions: 3},
		{Path: "docs/guide.md", Additions: 1},
		{Path: "go.mod", Additions: 2},
	}
	m.merge.codeOwners = fake
	m.merge.ownersKey = ""
	cmd := m.merge.refresh(m)
	return m, cmd
}

// ownersPreview is the text of the preview of the hovered row.
func ownersPreview(t *testing.T, m *panesModal) (title, body string) {
	t.Helper()
	p, ok := m.mergePreviewOf(panes.Box{Pane: paneMerge, Rect: panes.Rect{W: 82, H: 30}})
	if !ok {
		t.Fatal("the hovered row has no preview")
	}
	return p.Title, ansi.Strip(p.Body)
}

func TestMergePaneOwnersLoadFillsThePreview(t *testing.T) {
	t.Parallel()
	fake := &ownersFake{text: "api/ @alice\ndocs/ @acme/writers\n", members: map[string][]core.Person{
		"acme/writers": {{Login: "carol"}, {Login: "dave"}},
	}}
	m, cmd := reviewRequired(t, fake)
	if cmd == nil {
		t.Fatal("a cached miss started no read")
	}
	if _, body := ownersPreview(t, m); !strings.Contains(body, "Reading the code owners") {
		t.Errorf("before the read the preview is\n%s", body)
	}
	for _, msg := range results(cmd) {
		m.Update(msg)
	}
	title, body := ownersPreview(t, m)
	if title != "Required review · alice" {
		t.Errorf("title = %q, want the required owner named", title)
	}
	for _, want := range []string{"alice", "required · no review yet", "writers", "members: carol, dave", "server.go", "guide.md"} {
		if !strings.Contains(body, want) {
			t.Errorf("the preview lacks %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "go.mod") {
		t.Errorf("the preview lists a file nobody owns:\n%s", body)
	}
	if again := m.merge.refresh(m); again != nil {
		t.Error("a second refresh read the owners again")
	}
}

func TestMergePaneOwnersErrorSaysTheyCouldNotBeRead(t *testing.T) {
	t.Parallel()
	fake := &ownersFake{err: errors.New("boom")}
	m, cmd := reviewRequired(t, fake)
	for _, msg := range results(cmd) {
		m.Update(msg)
	}
	title, body := ownersPreview(t, m)
	if title != "Required review" || !strings.Contains(body, "Could not read the code owners.") || strings.Contains(body, "no code owner was") {
		t.Errorf("preview after a failed read = %q\n%s", title, body)
	}
	if again := m.merge.refresh(m); again != nil {
		t.Error("a failed read was retried at once")
	}
}
