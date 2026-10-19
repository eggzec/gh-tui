package pulls

import (
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/codeowners"
	"github.com/eggzec/gh-tui/internal/core"
	pullsvc "github.com/eggzec/gh-tui/internal/service/pulls"
	"github.com/eggzec/gh-tui/internal/tui/prpanes/merge"
)

// mergeOwnersMsg carries the code owners that a read found, for the modal and
// the read that asked.
type mergeOwnersMsg struct {
	id     int64
	key    string
	owners merge.Owners
}

// ownersKey names a read of the owners: the base the CODEOWNERS file is read
// at and the head whose files it is matched with.
func ownersKey(base, head string) string { return base + ".." + head }

// readOwners reads who owns the changed files, once, when a review is required
// and a reader of owners is set. What the caches hold shows at once; the rest
// is read, and the preview says so until it is.
func (p *mergePane) readOwners(m *panesModal) tea.Cmd {
	d := m.detail
	if p.codeOwners == nil || !m.loaded || d.ReviewDecision != core.ReviewRequired || d.BaseSHA == "" || d.HeadSHA == "" {
		p.ownersKey, p.owners = "", merge.Owners{}
		return nil
	}
	key := ownersKey(d.BaseSHA, d.HeadSHA)
	if key == p.ownersKey {
		return nil
	}
	p.ownersKey = key
	q := pullsvc.OwnersQuery{Repo: m.repo, Base: d.BaseSHA}
	if o, ok := p.cachedOwners(m, q, d.HeadSHA); ok {
		p.owners = o
		return nil
	}
	p.owners = merge.Owners{State: merge.OwnersLoading}
	svc, pr, ctx, id := p.codeOwners, m.svc, m.ctx, m.id
	repo, number, head := m.repo, m.number, d.HeadSHA
	return func() tea.Msg {
		failed := mergeOwnersMsg{id: id, key: key, owners: merge.Owners{State: merge.OwnersFailed}}
		file, err := svc.Owners(ctx, q)
		if err != nil {
			return failed
		}
		fs, err := pullFiles(ctx, pr, repo, number, head)
		if err != nil {
			return failed
		}
		o := newOwners(file.File, fs)
		for _, ow := range o.Owned {
			if ow.Owner.Kind != codeowners.OwnerTeam {
				continue
			}
			// Without read:org, or on any error, the members are left out.
			tm, err := svc.TeamMembers(ctx, pullsvc.TeamQuery{Org: ow.Owner.Org(), Slug: ow.Owner.Slug()})
			if err == nil {
				o.AddMembers(ow.Owner.Name, tm.Members, tm.Unavailable)
			}
		}
		return mergeOwnersMsg{id: id, key: key, owners: o}
	}
}

// cachedOwners returns the owners if the file, the files and the members of
// each team are all in the caches.
func (p *mergePane) cachedOwners(m *panesModal, q pullsvc.OwnersQuery, head string) (merge.Owners, bool) {
	file, ok := p.codeOwners.CachedOwners(q)
	if !ok {
		return merge.Owners{}, false
	}
	fs, ok, _ := cachedPullFiles(m.svc, m.repo, m.number, head)
	if !ok {
		return merge.Owners{}, false
	}
	o := newOwners(file.File, fs)
	for _, ow := range o.Owned {
		if ow.Owner.Kind != codeowners.OwnerTeam {
			continue
		}
		tm, ok := p.codeOwners.CachedTeamMembers(pullsvc.TeamQuery{Org: ow.Owner.Org(), Slug: ow.Owner.Slug()})
		if !ok {
			return merge.Owners{}, false
		}
		o.AddMembers(ow.Owner.Name, tm.Members, tm.Unavailable)
	}
	return o, true
}

// newOwners matches the changed files with the rules of file.
func newOwners(file codeowners.File, fs []core.CommitFile) merge.Owners {
	paths := make([]string, 0, len(fs))
	for _, f := range fs {
		paths = append(paths, f.Path)
	}
	return merge.Owners{State: merge.OwnersLoaded, Files: fs, Owned: file.OwnedPaths(paths)}
}

// ownersTaken keeps the owners that a read brought, when it is the read the
// pane waits for.
func (p *mergePane) ownersTaken(m *panesModal, msg mergeOwnersMsg) {
	if msg.id != m.id || msg.key != p.ownersKey {
		return
	}
	p.owners = msg.owners
	p.rebuild(m, false)
}
