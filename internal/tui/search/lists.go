package search

import (
	"context"
	"errors"
	"log/slog"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/obs"
	"github.com/eggzec/gh-tui/internal/service/search"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/feed"
)

// Lines of a result: a repository, issue or pull request takes two, and a
// file its path, three lines of what matched and a gap.
const (
	hitHeight       = 2
	codeHeight      = 5
	fragmentLines   = codeHeight - 2
	defaultPrefetch = 5
)

// hitList holds the results of one kind for one query, paged as they
// scroll.
type hitList struct {
	text string
	kind core.SearchKind
	feed feed.Model[core.SearchHit]
}

// codeList holds the files that match one query.
type codeList struct {
	text string
	feed feed.Model[core.CodeHit]
}

// hitKey identifies a result, so that a reload keeps the cursor on it.
func hitKey(h core.SearchHit) string {
	if h.Kind == core.SearchRepos {
		return h.Repo.Ref.String()
	}
	return h.Issue.Repo.String() + "#" + strconv.Itoa(h.Issue.Number)
}

// showKind shows the results of kind k for the query, reading them if
// they aren't shown yet: the first page of every kind but code comes with
// the first search, so this costs no request. Code is searched when its
// kind is shown, unless GitHub has run out of code searches.
func (s *Section) showKind(k core.SearchKind) tea.Cmd {
	s.kind = k
	if s.text == "" {
		return nil
	}
	s.seen.Opened(othersKey{text: s.text, kind: k})
	if k == core.SearchCode {
		return s.searchCode()
	}
	return s.ensureHits(k)
}

// ensureHits makes the list of kind k for the query, and starts it.
func (s *Section) ensureHits(k core.SearchKind) tea.Cmd {
	if l := s.hits[k]; l != nil && l.text == s.text {
		return nil
	}
	svc, text := s.svc, s.text
	fetch := func(ctx context.Context, cursor string) ([]core.SearchHit, string, error) {
		ctx, end := obs.Begin(ctx, "search."+string(k))
		r, err := svc.Search(ctx, search.Query{Text: text, Kind: k, Cursor: cursor})
		end(err, "span", "tui", "first", cursor == "", "items", len(r.Items))
		if err != nil {
			return nil, "", err
		}
		return r.Items, r.Next, nil
	}
	s.keepStale(k)
	l := &hitList{text: text, kind: k}
	l.feed = feed.New(fetch, s.renderHit,
		feed.WithContext(s.textCtx),
		feed.WithKey(hitKey),
		feed.WithItemHeight(hitHeight),
		feed.WithPrefetch(defaultPrefetch),
		feed.WithKeyMap(s.keys.feed),
		feed.WithStyles(s.theme.Feed(s.icons)),
		feed.WithEmptyText(emptyText(k)),
		feed.WithErrorText(ui.ErrorText("search", "", s.voice)),
	)
	s.sizeList(&l.feed)
	if s.focused && s.area == resultsArea && s.kind == k {
		l.feed.Focus()
	}
	s.hits[k] = l
	return l.feed.Init()
}

// searchCode searches code for the query, unless it has, or code search
// ran out of requests and the countdown to when they resume runs.
func (s *Section) searchCode() tea.Cmd {
	if s.text == "" {
		return nil
	}
	// A search GitHub refused for want of requests is tried again once
	// they are back.
	if s.code != nil && s.code.text == s.text && !errors.Is(s.code.feed.Err(), core.ErrRateLimited) {
		return nil
	}
	if s.limited() {
		return nil
	}
	svc, text := s.svc, s.text
	fetch := func(ctx context.Context, cursor string) ([]core.CodeHit, string, error) {
		ctx, end := obs.Begin(ctx, "search.code")
		p, err := svc.Code(ctx, search.CodeQuery{Text: text, Cursor: cursor})
		end(err, "span", "tui", "first", cursor == "", "items", len(p.Items))
		if err != nil {
			return nil, "", err
		}
		return p.Items, p.Next, nil
	}
	s.keepStale(core.SearchCode)
	l := &codeList{text: text}
	l.feed = feed.New(fetch, s.renderCode,
		feed.WithContext(s.textCtx),
		feed.WithKey(func(h core.CodeHit) string { return h.Repo.String() + "/" + h.Path }),
		feed.WithItemHeight(codeHeight),
		feed.WithPrefetch(defaultPrefetch),
		feed.WithKeyMap(s.keys.feed),
		feed.WithStyles(s.theme.Feed(s.icons)),
		feed.WithEmptyText(emptyText(core.SearchCode)),
		feed.WithErrorText(ui.ErrorText("search the code", "", s.voice)),
	)
	s.sizeList(&l.feed)
	if s.focused && s.area == resultsArea && s.kind == core.SearchCode {
		l.feed.Focus()
	}
	s.code = l
	return l.feed.Init()
}

// prefetchMsg reports that the first page of kind for text is cached, or
// why it isn't, for the page with id.
type prefetchMsg struct {
	id   int64
	text string
	kind core.SearchKind
	err  error
}

// What started the reads of the other kinds, for the log.
const (
	othersRest  = "rest"
	othersEnter = "enter"
)

// othersKey names the first page of one kind for one query, read ahead.
type othersKey struct {
	text string
	kind core.SearchKind
}

// readOthers reads the first page of every kind but code and the one on
// view, each in a request of its own, so that switching kinds is instant.
// They are guesses, so they wait until the query rests or is entered, run
// as reads ahead, which charge the prefetch budget and wait for the
// requests the user waits for, and stop when the page is left. A query
// reads them once.
func (s *Section) readOthers(trigger string) tea.Cmd {
	if s.text == "" || s.othersFor == s.text {
		return nil
	}
	s.othersFor = s.text
	if obs.PrefetchSpent() {
		s.seen.Count(obs.PrefetchOverBudget)
		return nil
	}
	shown := s.kind
	if shown == core.SearchCode {
		// Code waits to be asked for, so the repositories are searched in
		// its place.
		shown = core.SearchRepos
	}
	todo := make([]core.SearchKind, 0, 2)
	cached := 0
	for _, k := range kinds[:3] {
		switch _, ok := s.svc.CachedSearch(search.Query{Text: s.text, Kind: k}); {
		case k == shown:
		case ok:
			cached++
			s.seen.Count(obs.PrefetchCached)
		default:
			todo = append(todo, k)
		}
	}
	if len(todo) == 0 {
		return nil
	}
	s.stopOthers()
	ctx, cancel := context.WithCancel(obs.ForPrefetch(s.textCtx))
	s.stopOthers = cancel
	svc, id, text, seen := s.svc, s.id, s.text, s.seen
	cmds := make([]tea.Cmd, 0, len(todo)+1)
	cmds = append(cmds, func() tea.Msg {
		slog.InfoContext(ctx, "prefetch", "span", "prefetch", "kind", seen.Kind(), "trigger", trigger,
			"sent", len(todo), "skipped_cached", cached)
		return nil
	})
	for _, k := range todo {
		key := othersKey{text: text, kind: k}
		seen.Started(key)
		cmds = append(cmds, func() tea.Msg {
			ctx, end := obs.Begin(ctx, "search.prefetch")
			seen.Count(obs.PrefetchSent)
			err := svc.Prefetch(ctx, search.Query{Text: text, Kind: k})
			end(err, "span", "tui", "kind", string(k))
			switch {
			case err == nil:
				seen.Read(key)
			case errors.Is(err, core.ErrRateLimited):
				seen.Count(obs.PrefetchRateLimited)
			case ctx.Err() != nil:
				seen.Count(obs.PrefetchCanceled)
			default:
				seen.Count(obs.PrefetchFailed)
			}
			if err != nil {
				seen.Dropped(key)
			}
			return prefetchMsg{id: id, text: text, kind: k, err: err}
		})
	}
	return tea.Batch(cmds...)
}

// leaveOthers stops the reads of the other kinds in flight, as the page
// leaves the screen, and lets the next rest or enter read them again.
func (s *Section) leaveOthers() {
	s.stopOthers()
	s.stopOthers = func() {}
	s.othersFor = ""
}

// keepStale keeps the results of kind k on view, if any, to show dimmed
// until those of the query that replaces them arrive.
func (s *Section) keepStale(k core.SearchKind) {
	var view string
	switch {
	case k == core.SearchCode && s.code != nil && s.code.feed.Len() > 0:
		view = s.code.feed.View()
	case k != core.SearchCode && s.hits[k] != nil && s.hits[k].feed.Len() > 0:
		view = s.hits[k].feed.View()
	}
	if view == "" {
		return
	}
	lines := strings.Split(ansi.Strip(view), "\n")
	for i, l := range lines {
		lines[i] = s.st.subtle.render(l)
	}
	s.stale[k] = lines
}

// loading reports whether f has nothing to show yet: no result, no end
// and no error.
func loading[T any](f *feed.Model[T]) bool {
	return f.Len() == 0 && !f.Done() && f.Err() == nil
}

// staleView returns the dimmed results of the query before, while those
// of the kind on view are still loading.
func (s *Section) staleView() ([]string, bool) {
	lines, ok := s.stale[s.kind]
	if !ok {
		return nil, false
	}
	if l, ok := s.visibleHits(); ok && !loading(&l.feed) {
		delete(s.stale, s.kind)
		return nil, false
	}
	if l, ok := s.visibleCode(); ok && !loading(&l.feed) {
		delete(s.stale, s.kind)
		return nil, false
	}
	return lines, true
}

// spinTitle starts the spinner of the title, while dimmed results wait
// for new ones.
func (s *Section) spinTitle() tea.Cmd {
	if s.spinning {
		return nil
	}
	if _, ok := s.staleView(); !ok {
		return nil
	}
	s.spinning = true
	return s.spin.Tick
}

func emptyText(k core.SearchKind) string {
	switch k {
	case core.SearchCode:
		return "No file matches. Try other words, or qualifiers such as language:go or repo:owner/name."
	case core.SearchIssues, core.SearchPulls:
		return "Nothing matches. Try other words, or qualifiers such as is:open or author:@me."
	default:
		return "No repository matches. Try other words, or qualifiers such as user:name or stars:>100."
	}
}

// codeTickMsg counts down to when code search resumes, for the page with
// id.
type codeTickMsg struct{ id int64 }

// limitCode counts down to reset, when code search resumes.
func (s *Section) limitCode(reset time.Time) tea.Cmd {
	if reset.After(s.codeReset) {
		s.codeReset = reset
	}
	return s.tick()
}

func (s *Section) tick() tea.Cmd {
	if s.ticking || !s.limited() {
		return nil
	}
	s.ticking = true
	id := s.id
	return tea.Tick(time.Second, func(time.Time) tea.Msg { return codeTickMsg{id: id} })
}

// limited reports whether code search is still out of requests.
func (s *Section) limited() bool {
	return !s.codeReset.IsZero() && s.now().Before(s.codeReset)
}

// ticked counts down, and stops once code search resumes, when the search
// that the limit refused is offered again, as one not run yet.
func (s *Section) ticked() tea.Cmd {
	s.ticking = false
	if !s.limited() {
		s.codeReset = time.Time{}
		if s.code != nil && errors.Is(s.code.feed.Err(), core.ErrRateLimited) {
			s.code = nil
		}
		return nil
	}
	return s.tick()
}

// codeFailed watches the code results for GitHub running out of code
// searches, which it says when they resume.
func (s *Section) codeFailed() tea.Cmd {
	if s.code == nil {
		return nil
	}
	rl, ok := errors.AsType[*core.RateLimitError](s.code.feed.Err())
	if !ok || rl.Reset.IsZero() || !rl.Reset.After(s.codeReset) {
		return nil
	}
	return s.limitCode(rl.Reset)
}

// visible returns the feed of the results on view, if there is one.
func (s *Section) visibleHits() (*hitList, bool) {
	if s.text == "" || s.kind == core.SearchCode {
		return nil, false
	}
	l := s.hits[s.kind]
	return l, l != nil && l.text == s.text
}

func (s *Section) visibleCode() (*codeList, bool) {
	if s.text == "" || s.kind != core.SearchCode || s.code == nil || s.code.text != s.text {
		return nil, false
	}
	return s.code, true
}

// feedKeys returns the keys of the results on view.
func (s *Section) feedKeys() feed.KeyMap {
	if l, ok := s.visibleHits(); ok {
		return l.feed.KeyMap()
	}
	if l, ok := s.visibleCode(); ok {
		return l.feed.KeyMap()
	}
	return s.keys.feed
}

// refreshCounts reads the counts of the query from the cache, which each
// search adds to.
func (s *Section) refreshCounts() {
	s.counts = nil
	if s.text == "" {
		return
	}
	for _, k := range kinds[:3] {
		if r, ok := s.svc.CachedSearch(search.Query{Text: s.text, Kind: k}); ok && r.Counts != nil {
			s.counts = r.Counts
			break
		}
	}
	if p, ok := s.svc.CachedCode(search.CodeQuery{Text: s.text}); ok {
		if s.counts == nil {
			s.counts = make(map[core.SearchKind]int, 1)
		}
		s.counts[core.SearchCode] = p.Total
	}
}
