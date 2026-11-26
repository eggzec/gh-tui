package pulls

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/prpanes/dev"
)

// The Development pane lists what the pull request closes, what its body and
// comments name and what names it, and never the pull requests that close the
// issues it closes.
func TestDevelopmentMentionsFromWritten(t *testing.T) {
	t.Parallel()
	named := devRef(core.KindIssue, 33, "Named once")
	closed := devRef(core.KindIssue, 12, "Crash on empty input")
	refused := core.Reference{Target: core.Target{Repo: repo, Number: 34, Kind: core.KindIssue}, Problem: "not found, or private"}
	tests := []struct {
		name    string
		refs    core.References
		header  string
		missing string
		note    bool
	}{
		{"named", core.References{Written: []core.Reference{named}}, "Mentions 1", "#34", false},
		{"refused is dropped", core.References{Written: []core.Reference{named, refused}}, "Mentions 1", "#34", false},
		{"already closed stays under Closes", core.References{Written: []core.Reference{named, closed}}, "Mentions 1", "", false},
		{"unresolved", core.References{Written: []core.Reference{named}, Unresolved: 3}, "Mentions 1+", "", true},
		{"failed", core.References{Written: []core.Reference{named}, Failed: 2}, "Mentions 1+", "", true},
		{"comments cut", core.References{Written: []core.Reference{named}, CommentsRead: 100, CommentsTotal: 150}, "Mentions 1+", "", true},
		{"cut and nothing named", core.References{CommentsRead: 100, CommentsTotal: 150}, "", "Mentions", false},
		{"nothing named", core.References{}, "", "Mentions", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := openedDev(t, &fakeDev{links: devLinks()})
			m := h.modal()
			run(m, m.receiveRefs(devRefsMsg{id: m.id, refs: tt.refs}))
			m.dev.model.SetSize(80, 20)
			v := ansi.Strip(m.dev.model.View())
			if tt.header != "" && !strings.Contains(v, tt.header) {
				t.Errorf("the pane lacks %q:\n%s", tt.header, v)
			}
			if tt.missing != "" && strings.Contains(v, tt.missing) {
				t.Errorf("the pane has %q:\n%s", tt.missing, v)
			}
			if strings.Contains(v, "Closed by") {
				t.Errorf("the pane has a Closed by group:\n%s", v)
			}
			if strings.Contains(v, "Follow-up cleanup") {
				t.Errorf("the pane lists a pull request that closes an issue:\n%s", v)
			}
			if got := strings.Contains(v, "more on GitHub"); got != tt.note {
				t.Errorf("note %v, want %v:\n%s", got, tt.note, v)
			}
			if _, ok := m.dev.model.Item("mentions:" + core.RefKey(named.Target)); ok != (tt.header != "") {
				t.Errorf("item present %v, want %v", ok, tt.header != "")
			}
			if _, ok := m.dev.model.Item("mentions:" + core.RefKey(closed.Target)); ok {
				t.Error("an item under Closes is also under Mentions")
			}
		})
	}
}

// A pull request links what it closes, so every read of its references and
// mentions says it is a pull request, and is made for its update time.
func TestDevelopmentReadsAsPull(t *testing.T) {
	t.Parallel()
	fake := &fakeRefs{}
	h := started(t, newFakeService(), 120, 30, WithDevelopment(&fakeDev{links: devLinks()}), WithReferences(fake))
	press(t, h, "enter")
	press(t, h, "5")
	m := h.modal()
	m.cachedDev()
	run(m, m.readRefs(false))
	run(m, m.readMentions(false))
	if len(fake.refQueries) < 2 || len(fake.mentionQueries) < 2 {
		t.Fatalf("read %d reference and %d mention queries, want cached and fresh of each", len(fake.refQueries), len(fake.mentionQueries))
	}
	for _, q := range fake.refQueries {
		if !q.Pull {
			t.Errorf("reference query %+v is not for a pull request", q)
		}
		if !q.Updated.Equal(m.detail.UpdatedAt) {
			t.Errorf("reference query updated %v, want %v", q.Updated, m.detail.UpdatedAt)
		}
	}
	for _, q := range fake.mentionQueries {
		if !q.Pull {
			t.Errorf("mentions query %+v is not for a pull request", q)
		}
	}
}

// What names the pull request is listed once, under Mentions when the text
// names it too, and under Referenced by otherwise.
func TestDevelopmentReferencedBy(t *testing.T) {
	t.Parallel()
	h := started(t, newFakeService(), 120, 30, WithDevelopment(&fakeDev{links: devLinks()}), WithReferences(&fakeRefs{}))
	press(t, h, "enter")
	press(t, h, "5")
	m := h.modal()
	both := devRef(core.KindPull, 40, "Both ways")
	only := devRef(core.KindIssue, 41, "Names this pull request")
	run(m, m.receiveRefs(devRefsMsg{id: m.id, refs: core.References{Written: []core.Reference{both}, Mentioned: 2}}))
	run(m, m.receiveMentions(devMentionsMsg{id: m.id, page: core.Page[core.Reference]{Items: []core.Reference{both, devRef(core.KindIssue, 12, "Crash on empty input"), only}}}))
	m.dev.model.SetSize(80, 20)
	v := ansi.Strip(m.dev.model.View())
	for _, want := range []string{"Mentions 1", "Referenced by 1", "Names this pull request"} {
		if !strings.Contains(v, want) {
			t.Errorf("the pane lacks %q:\n%s", want, v)
		}
	}
	if n := strings.Count(v, "Both ways"); n != 1 {
		t.Errorf("an item named both ways is listed %d times:\n%s", n, v)
	}
	if it, ok := m.dev.model.Item("referenced:" + core.RefKey(only.Target)); !ok || it.Kind != dev.Issue {
		t.Errorf("the item that names it is %+v, %v", it, ok)
	}
}
