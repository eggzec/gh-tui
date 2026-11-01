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
	gen   uint64 // moves when a sync changes an answer a view drew
	asked map[uint64]bool
	// drawn is the answer each reference the views asked of Func got, so a
	// sync that changes none of them draws nothing again.
	drawn map[drawnRef]answer
}

// drawnRef names a reference of the text of a repo.
type drawnRef struct {
	repo string
	cand autolinks.Candidate
}

// answer is what a reference is drawn with: its link, if GitHub confirmed it.
type answer struct {
	url string
	ok  bool
}

// NewAutolinks returns the autolinks of the session on host. A nil svc links
// nothing.
func NewAutolinks(svc AutolinkService, host string) *Autolinks {
	return &Autolinks{svc: svc, host: host, asked: map[uint64]bool{}, drawn: map[drawnRef]answer{}}
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
		got := a.answer(repo, c)
		a.mu.Lock()
		// Only the first draw is kept: drawn holds the answer as of the
		// last sync, and only Synced moves it forward.
		k := drawnRef{repo, c}
		if _, ok := a.drawn[k]; !ok {
			a.drawn[k] = got
		}
		a.mu.Unlock()
		return got.url, got.ok
	}
}

// Seen collects the references that the renderer of a view asked about, so
// that a view whose bodies are drawn by others, such as the rows of a tree,
// can ask GitHub about what it showed.
type Seen struct {
	a    *Autolinks
	repo string

	mu   sync.Mutex
	cand []autolinks.Candidate
}

// Seen returns a collector for the bodies of repo, or nil if links are not
// served. A nil collector gives no function and asks nothing.
func (a *Autolinks) Seen(repo string) *Seen {
	if a == nil || a.svc == nil {
		return nil
	}
	return &Seen{a: a, repo: repo}
}

// Func returns the function for the renderer: it links as [Autolinks.Func]
// does and notes each reference it is asked about.
func (s *Seen) Func() markdown.Autolink {
	if s == nil {
		return nil
	}
	link := s.a.Func(s.repo)
	return func(kind markdown.RefKind, ref string) (string, bool) {
		s.mu.Lock()
		s.cand = append(s.cand, autolinks.Candidate{Kind: verifiedKind(kind), Ref: ref})
		s.mu.Unlock()
		return link(kind, ref)
	}
}

// Ask returns the command that asks about the references noted since the
// last call and not asked about since the last answer came, or nil.
func (s *Seen) Ask(ctx context.Context) tea.Cmd {
	if s == nil || !termtext.LinksOn() {
		return nil
	}
	s.mu.Lock()
	noted := s.cand
	s.cand = nil
	s.mu.Unlock()
	var cands []autolinks.Candidate
	s.a.mu.Lock()
	for _, c := range noted {
		h := fnv.New64a()
		_, _ = h.Write([]byte(s.repo + "\x00" + string(c.Kind) + "\x00" + c.Ref))
		k := h.Sum64()
		if !s.a.asked[k] {
			s.a.asked[k] = true
			cands = append(cands, c)
		}
	}
	s.a.mu.Unlock()
	if len(cands) == 0 {
		return nil
	}
	return func() tea.Msg {
		s.a.svc.Verify(ctx, s.repo, cands)
		return nil
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
//
// One message reaches several views, so each keeps the generation it last drew
// with in drew: a view is told to link again while its generation is behind
// the answers learned, however many views asked before it.
func (a *Autolinks) Synced(msg SyncMsg, drew *uint64) bool {
	if a == nil || a.svc == nil || msg.Key != autolinks.SyncKey(a.host) {
		return false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	changed := false
	for d, was := range a.drawn {
		now := a.answer(d.repo, d.cand)
		if now != was {
			a.drawn[d] = now
			changed = true
		}
	}
	if changed {
		clear(a.asked)
		a.gen++
	}
	stale := *drew != a.gen
	*drew = a.gen
	return stale
}

// answer is what the views draw for c of repo, as [Autolinks.Func] gives it.
func (a *Autolinks) answer(repo string, c autolinks.Candidate) answer {
	r := a.svc.Cached(repo, []autolinks.Candidate{c})
	if len(r) == 0 {
		return answer{}
	}
	return answer{url: r[0].URL, ok: r[0].Known && r[0].Exists}
}
