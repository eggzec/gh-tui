package ui

import (
	"testing"

	"charm.land/bubbles/v2/key"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/pkg/bubbles/toast"
)

func TestGate(t *testing.T) {
	bubbletea := core.RepoRef{Owner: "charmbracelet", Name: "bubbletea"}
	caps := func(p core.Permission) core.RepoCaps {
		return core.RepoCaps{Known: true, Permission: p, Issues: true, MergeCommit: true, Squash: true, Rebase: true}
	}
	archived := caps(core.PermissionAdmin)
	archived.Archived = true
	noMerge := caps(core.PermissionAdmin)
	noMerge.MergeCommit, noMerge.Squash, noMerge.Rebase = false, false, false

	// Issues read with REST, which says nothing of the viewer.
	theirs := &core.Issue{Number: 12, Author: core.User{Login: "octocat"}}
	mine := &core.Issue{Number: 13, Author: core.User{Login: "Me"}}
	locked := &core.Issue{Number: 14, Author: core.User{Login: "octocat"}, Locked: true, LockReason: "too_heated"}
	// Pull requests read with GraphQL, which says.
	refused := &core.Issue{Number: 15, Caps: core.ItemCaps{Known: true, LabelKnown: true}}
	allowed := &core.Issue{Number: 16, Caps: core.ItemCaps{Known: true, Update: true, Close: true, Label: true, LabelKnown: true}}
	// A pull request from an Enterprise Server that doesn't say who
	// labels.
	unsaid := &core.Issue{Number: 17, Caps: core.ItemCaps{Known: true, Update: true}}

	tests := []struct {
		name   string
		caps   core.RepoCaps
		viewer string
		action Action
		it     *core.Issue
		why    string
	}{
		{name: "unknown caps allow a merge", action: ActMerge},
		{name: "unknown caps allow closing another's issue", viewer: "me", action: ActClose, it: theirs},
		{name: "write merges", caps: caps(core.PermissionWrite), action: ActMerge},
		{
			name: "read can't merge", caps: caps(core.PermissionRead), action: ActMerge,
			why: "You can't merge in charmbracelet/bubbletea (read access).",
		},
		{
			name: "triage can't merge", caps: caps(core.PermissionTriage), action: ActMerge,
			why: "You can't merge in charmbracelet/bubbletea (triage access).",
		},
		{
			name: "a merge needs a method", caps: noMerge, action: ActMerge,
			why: "charmbracelet/bubbletea allows no merge method.",
		},
		{
			name: "an archived repository is read-only", caps: archived, action: ActComment, it: theirs,
			why: "charmbracelet/bubbletea is archived, so it's read-only.",
		},
		{
			name: "an archived repository can't re-run", caps: archived, action: ActRerun,
			why: "charmbracelet/bubbletea is archived, so it's read-only.",
		},
		{name: "triage closes another's issue", caps: caps(core.PermissionTriage), viewer: "me", action: ActClose, it: theirs},
		{
			name: "read can't close another's issue", caps: caps(core.PermissionRead), viewer: "me", action: ActClose, it: theirs,
			why: "You can't close #12 in charmbracelet/bubbletea (read access).",
		},
		{
			name: "read can't reopen another's issue", caps: caps(core.PermissionRead), viewer: "me", action: ActReopen, it: theirs,
			why: "You can't reopen #12 in charmbracelet/bubbletea (read access).",
		},
		{name: "the author closes their own issue", caps: caps(core.PermissionRead), viewer: "me", action: ActClose, it: mine},
		{name: "an unknown viewer may be the author", caps: caps(core.PermissionRead), action: ActClose, it: theirs},
		{
			name: "GitHub's no wins over triage", caps: caps(core.PermissionAdmin), action: ActClose, it: refused,
			why: "You can't close #15 in charmbracelet/bubbletea (admin access).",
		},
		{name: "GitHub's yes wins over read", caps: caps(core.PermissionRead), action: ActReopen, it: allowed},
		{name: "the author changes their draft", caps: caps(core.PermissionRead), viewer: "me", action: ActDraft, it: mine},
		{
			name: "GitHub says who changes a draft", caps: caps(core.PermissionRead), action: ActDraft, it: refused,
			why: "You can't change #15 in charmbracelet/bubbletea (read access).",
		},
		{name: "read comments", caps: caps(core.PermissionRead), action: ActComment, it: theirs},
		{
			name: "read can't comment on a locked issue", caps: caps(core.PermissionRead), action: ActComment, it: locked,
			why: "#14 is locked as too heated · only collaborators can comment.",
		},
		{name: "write comments on a locked issue", caps: caps(core.PermissionWrite), action: ActComment, it: locked},
		{name: "triage labels", caps: caps(core.PermissionTriage), action: ActLabel, it: theirs},
		{
			name: "read can't label, even their own", caps: caps(core.PermissionRead), viewer: "me", action: ActLabel, it: mine,
			why: "Labeling needs triage access to charmbracelet/bubbletea.",
		},
		{
			name: "GitHub says who labels", caps: caps(core.PermissionAdmin), action: ActLabel, it: refused,
			why: "Labeling needs triage access to charmbracelet/bubbletea.",
		},
		// A custom role that adds labeling to read access.
		{name: "GitHub says a reader labels", caps: caps(core.PermissionRead), action: ActLabel, it: allowed},
		{name: "triage labels where GitHub doesn't say", caps: caps(core.PermissionTriage), action: ActLabel, it: unsaid},
		{
			name: "read can't label where GitHub doesn't say", caps: caps(core.PermissionRead), action: ActLabel, it: unsaid,
			why: "Labeling needs triage access to charmbracelet/bubbletea.",
		},
		{
			name: "read can't re-run", caps: caps(core.PermissionRead), action: ActRerun,
			why: "Re-running needs write access to charmbracelet/bubbletea.",
		},
		{
			name: "triage can't cancel", caps: caps(core.PermissionTriage), action: ActCancelRun,
			why: "Cancelling needs write access to charmbracelet/bubbletea.",
		},
		{name: "write re-runs", caps: caps(core.PermissionWrite), action: ActRerun},
		{
			name: "an unknown permission leaves it to GitHub", caps: core.RepoCaps{Known: true, Rebase: true},
			action: ActMerge,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := Gate{Repo: bubbletea, Caps: tt.caps, Viewer: tt.viewer}
			ok, why := g.Allow(tt.action, tt.it)
			if ok != (tt.why == "") || why != tt.why {
				t.Errorf("Allow = %v, %q; want %v, %q", ok, why, tt.why == "", tt.why)
			}
			b := g.Gated(key.NewBinding(key.WithKeys("m")), tt.action, tt.it)
			if b.Enabled() != ok {
				t.Errorf("Gated binding enabled = %v, want %v", b.Enabled(), ok)
			}
			cmd, refused := g.Refuse(tt.action, tt.it)
			if refused == ok || (cmd == nil) == refused {
				t.Fatalf("Refuse = %v, %v; want a command only when refused", cmd != nil, refused)
			}
			if refused {
				if msg := cmd(); msg != (NotifyMsg{Level: toast.Info, Text: tt.why}) {
					t.Errorf("Refuse shows %+v, want an info toast of %q", msg, tt.why)
				}
			}
		})
	}
}
