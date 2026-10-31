package ui

import (
	"context"
	"hash/fnv"
	"sync"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/autolinks"
	"github.com/eggzec/gh-tui/pkg/markdown"
	"github.com/eggzec/gh-tui/pkg/termtext"
)

// AutolinkService is what the views call of the service that verifies the
// references (#N, @login, hashes) that bodies name.
type AutolinkService interface {
	Cached(repo string, cands []autolinks.Candidate) []autolinks.Result
	Verify(ctx context.Context, repo string, cands []autolinks.Candidate) []autolinks.Result
}

// Autolinks links the references of the bodies on view that GitHub confirmed.
// The zero value, and a nil one, link nothing and ask nothing.
type Autolinks struct {
	svc  AutolinkService
	host string

	mu    sync.Mutex
	asked map[uint64]bool
}

// NewAutolinks returns the autolinks of the session on host. A nil svc links
// nothing.
func NewAutolinks(svc AutolinkService, host string) *Autolinks {
	return &Autolinks{svc: svc, host: host, asked: map[uint64]bool{}}
}

// Func returns what the renderer of a body of repo ("owner/name", or empty
// for a text that has none) calls to link a reference. It reads what is
// cached and does no I/O, and is nil if there is nothing to ask.
func (a *Autolinks) Func(repo string) markdown.Autolink {
	if a == nil || a.svc == nil {
		return nil
	}
	return func(kind markdown.RefKind, ref string) (string, bool) {
		c := autolinks.Candidate{Kind: verifiedKind(kind), Ref: ref}
		r := a.svc.Cached(repo, []autolinks.Candidate{c})
		if len(r) == 0 {
			return "", false
		}
		return r[0].URL, r[0].Known && r[0].Exists
	}
}

func verifiedKind(k markdown.RefKind) core.VerifiedKind {
	switch k {
	case markdown.RefUser:
		return core.VerifiedUser
	case markdown.RefCommit:
		return core.VerifiedCommit
	case markdown.RefIssue:
		return core.VerifiedIssue
	}
	return core.VerifiedIssue
}

// Verify returns the command that asks GitHub about the references in body,
// a text of repo, or nil if there is nothing to ask: links are off, there
// is no service, the body names nothing or it was asked about since the
// last answer came. Its answers reach the views as a sync message of
// [Autolinks.Synced].
func (a *Autolinks) Verify(ctx context.Context, repo, body string) tea.Cmd {
	if a == nil || a.svc == nil || !termtext.LinksOn() {
		return nil
	}
	refs := markdown.Refs(body)
	if len(refs) == 0 {
		return nil
	}
	h := fnv.New64a()
	_, _ = h.Write([]byte(repo + "\x00" + body))
	k := h.Sum64()
	a.mu.Lock()
	done := a.asked[k]
	a.asked[k] = true
	a.mu.Unlock()
	if done {
		return nil
	}
	cands := make([]autolinks.Candidate, len(refs))
	for i, r := range refs {
		cands[i] = autolinks.Candidate{Kind: verifiedKind(r.Kind), Ref: r.Ref}
	}
	return func() tea.Msg {
		a.svc.Verify(ctx, repo, cands)
		return nil
	}
}

// Synced reports whether msg says that answers were learned, so that the
// views link again. Bodies may be asked about again then, which costs
// nothing for what is fresh.
func (a *Autolinks) Synced(msg SyncMsg) bool {
	if a == nil || msg.Key != autolinks.SyncKey(a.host) {
		return false
	}
	a.mu.Lock()
	clear(a.asked)
	a.mu.Unlock()
	return true
}
