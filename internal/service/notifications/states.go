package notifications

import (
	"context"
	"log/slog"
	"strconv"

	"github.com/eggzec/gh-tui/internal/cache"
	"github.com/eggzec/gh-tui/internal/core"
)

// statesKind is what the service keeps the states of subjects as, and
// statesSchema the version of core.SubjectState. Bump it when the type
// changes shape.
const (
	statesKind   = "notificationstates"
	statesSchema = 1
)

// subjectRef returns what n is about, or false if its state can't be asked
// for: only pull requests, issues and discussions with a number have one.
func subjectRef(n core.Notification) (core.SubjectRef, bool) {
	switch n.Subject.Type {
	case core.SubjectPullRequest, core.SubjectIssue, core.SubjectDiscussion:
	default:
		return core.SubjectRef{}, false
	}
	if n.Subject.Number <= 0 || n.Repo.Owner == "" || n.Repo.Name == "" {
		return core.SubjectRef{}, false
	}
	return core.SubjectRef{Repo: n.Repo, Number: n.Subject.Number, Type: n.Subject.Type}, true
}

// subjectRefs returns the subjects of items whose state can be asked for,
// each once.
func subjectRefs(items []core.Notification) []core.SubjectRef {
	var refs []core.SubjectRef
	seen := make(map[core.SubjectRef]bool, len(items))
	for i := range items {
		if r, ok := subjectRef(items[i]); ok && !seen[r] {
			seen[r] = true
			refs = append(refs, r)
		}
	}
	return refs
}

// stateKey is the key of the state of r in the cache and on the shelf.
func stateKey(r core.SubjectRef) string {
	return statesKind + "?repo=" + r.Repo.Owner + "/" + r.Repo.Name +
		"&type=" + string(r.Type) + "&number=" + strconv.Itoa(r.Number)
}

// await notes that the page of items came from GitHub, so that the next
// States of its subjects asks for their state again.
func (s *Service) await(items []core.Notification) {
	refs := subjectRefs(items)
	if len(refs) == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.pending == nil {
		s.pending = make(map[core.SubjectRef]struct{}, len(refs))
	}
	for _, r := range refs {
		s.pending[r] = struct{}{}
	}
}

// due removes from the pending subjects those of refs, and returns them.
func (s *Service) due(refs []core.SubjectRef) []core.SubjectRef {
	s.mu.Lock()
	defer s.mu.Unlock()
	var ask []core.SubjectRef
	for _, r := range refs {
		if _, ok := s.pending[r]; ok {
			delete(s.pending, r)
			ask = append(ask, r)
		}
	}
	return ask
}

// CachedStates returns the state of the subjects of items that the service
// holds in memory, stale or not, without a request. It reports nothing for
// a subject it doesn't hold, and while the token may not read notifications.
func (s *Service) CachedStates(items []core.Notification) map[core.SubjectRef]core.SubjectState {
	if s.refused() != nil {
		return nil
	}
	out := make(map[core.SubjectRef]core.SubjectState)
	for _, r := range subjectRefs(items) {
		if e, st := s.states.Get(stateKey(r)); st != cache.Miss {
			out[r] = e.Value
		}
	}
	return out
}

// States returns the state of the subjects of items. What an earlier session
// kept is read into memory, and the subjects of a page that came from GitHub
// since they were last read are asked for, in one batch per
// github.SubjectSlots of them, and kept. Nothing else costs a request, so a
// page served from the cache, or kept, is shown with what is held.
//
// If the request fails, States returns the states it holds with the error:
// the subjects stay as they were. A subject GitHub doesn't answer for, such
// as one that was deleted, keeps what is held, if anything. It's meant to run
// in a tea.Cmd, since it reads the store.
func (s *Service) States(ctx context.Context, items []core.Notification) (map[core.SubjectRef]core.SubjectState, error) {
	if s.refused() != nil {
		return nil, nil
	}
	refs := subjectRefs(items)
	out := make(map[core.SubjectRef]core.SubjectState, len(refs))
	for _, r := range refs {
		key := stateKey(r)
		s.keptStates.Warm(s.states, key, true)
		if e, st := s.states.Get(key); st != cache.Miss {
			out[r] = e.Value
		}
	}
	ask := s.due(refs)
	if len(ask) == 0 {
		return out, nil
	}
	got, err := s.api.SubjectStates(ctx, ask)
	if err != nil {
		slog.WarnContext(ctx, "notification states not read", "span", "service.notifications", "subjects", len(ask), "err", err)
		return out, err
	}
	for r, st := range got {
		key := stateKey(r)
		e := cache.Entry[core.SubjectState]{Value: st}
		s.states.Set(key, e)
		if err := s.keptStates.Save(key, e); err != nil {
			slog.DebugContext(ctx, "notification state not kept", "span", "service.notifications", "err", err)
		}
		out[r] = st
	}
	return out, nil
}
