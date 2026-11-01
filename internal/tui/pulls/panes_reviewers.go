package pulls

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"strings"
	"sync"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/facets"
	"github.com/eggzec/gh-tui/internal/service/optimistic"
	"github.com/eggzec/gh-tui/internal/service/pulls"
	"github.com/eggzec/gh-tui/internal/tui/prpanes/involved"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/picker"
	"github.com/eggzec/gh-tui/pkg/bubbles/toast"
)

// reviewerEditor is what the reviewers picker needs to change who is asked to
// review.
type reviewerEditor interface {
	reviewRequester
	// RemoveReviewRequests shows the people as no longer asked in the cache at
	// once. The returned Op sends the change.
	RemoveReviewRequests(repo core.RepoRef, number int, who []pulls.Reviewer) *optimistic.Op
	// ReviewerTeams lists the teams of the organization that match the text.
	ReviewerTeams(ctx context.Context, q pulls.ReviewerTeamsQuery) (pulls.ReviewerTeams, error)
}

// reviewerRows is how many rows a search lists besides the reviewers: the
// people it finds (the limit of facets.Service.People) and a page of teams.
const reviewerRows = 40

// errNotPending is said when the user tries to untick a review that was given.
const notPendingText = "only a pending review request can be removed"

// reviewersOverlay is the checklist of the reviewers of the pull request,
// keyed by "user:login" or "team:org/slug".
type reviewersOverlay = tickOverlay[string]

func userKey(login string) string { return "user:" + login }
func teamKey(slug string) string  { return "team:" + slug }

// openReviewers opens the picker of the people and teams asked to review the
// pull request. It keeps the current reviewers listed, ticked, whatever the
// search returns; those who reviewed already are locked.
func (m *panesModal) openReviewers() tea.Cmd {
	if !m.loaded || m.top() != nil {
		return nil
	}
	if cmd, refused := m.gate().Refuse(ui.ActReview, &m.detail.Issue); refused {
		return cmd
	}
	m.showReviewers()
	o, ok := m.top().(*reviewersOverlay)
	if !ok {
		return nil
	}
	return tea.Batch(o.p.Focus(), o.p.Init())
}

// reviewerDetail words the state of a reviewer, and marks a team.
func reviewerDetail(p involved.Person) string {
	state := p.State.String()
	switch p.State {
	case involved.Requested, involved.Rerequested:
		state = "pending"
	case involved.None, involved.Approved, involved.ChangesRequested, involved.Commented, involved.Dismissed:
	}
	if p.Team != "" {
		return strings.TrimSpace("team  " + state)
	}
	return state
}

// showReviewers opens the picker over the people and teams of the repository.
func (m *panesModal) showReviewers() {
	have := involved.Reviewers(m.detail.Reviewers, m.detail.Author.Login)
	// The search runs in a command, off the update loop, while the change reads
	// who is in it.
	var mu sync.Mutex
	who := map[string]pulls.Reviewer{}
	remember := func(key string, r pulls.Reviewer) {
		mu.Lock()
		defer mu.Unlock()
		who[key] = r
	}
	locked := map[string]bool{}

	o := &reviewersOverlay{
		heading: "Reviewers",
		order:   strings.ToLower,
		on:      map[string]bool{},
		was:     map[string]bool{},
		rows:    len(have) + reviewerRows,
	}
	o.change = func(on, was map[string]bool) tea.Cmd {
		var add, remove []pulls.Reviewer
		mu.Lock()
		for k := range on {
			if !was[k] {
				add = append(add, who[k])
			}
		}
		for k := range was {
			if !on[k] && !locked[k] {
				remove = append(remove, who[k])
			}
		}
		mu.Unlock()
		if len(add) == 0 && len(remove) == 0 {
			return nil
		}
		byName := func(a, b pulls.Reviewer) int {
			return strings.Compare(a.Login+a.Team, b.Login+b.Team)
		}
		slices.SortFunc(add, byName)
		slices.SortFunc(remove, byName)
		return m.applyReviewers(add, remove)
	}
	o.locked = func(picker.Item) tea.Cmd { return ui.Notify(toast.Info, notPendingText) }

	haveItems := make([]picker.Item, 0, len(have))
	for _, p := range have {
		key, title := userKey(p.Login), p.Login
		if p.Team != "" {
			key, title = teamKey(p.Team), p.Name
		}
		r := pulls.Reviewer{Login: p.Login, Team: p.Team, Name: p.Name}
		remember(key, r)
		o.on[key], o.was[key] = true, true
		if p.State.Reviewed() {
			locked[key] = true
		}
		haveItems = append(haveItems, picker.Item{Title: title, Detail: reviewerDetail(p), Value: key, Locked: p.State.Reviewed()})
	}

	// The reviewers are in every answer, so that a search that does not find
	// them leaves them listed, and ticked.
	owner := m.repo.Owner
	author := m.detail.Author.Login
	rows := func(people []core.User, teams []core.Team) []picker.Item {
		items := slices.Clone(haveItems)
		listed := func(key string) bool {
			return slices.ContainsFunc(items, func(it picker.Item) bool { return strings.EqualFold(it.Value.(string), key) })
		}
		for _, u := range people {
			if strings.EqualFold(u.Login, author) || listed(userKey(u.Login)) {
				continue
			}
			remember(userKey(u.Login), pulls.Reviewer{Login: u.Login})
			items = append(items, picker.Item{Title: u.Login, Detail: u.Name, Value: userKey(u.Login)})
		}
		for _, t := range teams {
			slug := owner + "/" + t.Slug
			name := t.Name
			if name == "" {
				name = t.Slug
			}
			if listed(teamKey(slug)) {
				continue
			}
			remember(teamKey(slug), pulls.Reviewer{Team: slug, Name: name})
			items = append(items, picker.Item{Title: name, Detail: "team", Value: teamKey(slug)})
		}
		return items
	}

	var search picker.Search
	opts := []picker.Option{
		picker.WithContext(m.ctx),
		picker.WithGroupHeaders(false),
		picker.WithModes(true),
		picker.WithFilterLine(true),
		picker.WithMarks(kindOn, kindOff),
		picker.WithTicks(),
		picker.WithMarkedFirst(true),
		picker.WithDetailMatch(true),
		picker.WithKeyMap(m.keys.pick),
		// Enter before the first answer, or after a failed search, still sees
		// the reviewers as ticked.
		picker.WithKnown(rows(nil, nil)),
	}
	if f := m.list.facets; f != nil {
		repo, svc := m.repo, m.svc
		search = func(ctx context.Context, q picker.Query) ([]picker.Item, error) {
			people, err := f.People(ctx, facets.PeopleQuery{Repo: repo, Text: q.Text})
			if err != nil {
				return nil, err
			}
			// Teams are an extra: an organization that hides them, or a failed
			// read of them, leaves the people.
			var teams []core.Team
			if res, err := svc.ReviewerTeams(ctx, pulls.ReviewerTeamsQuery{Repo: repo, Text: q.Text}); err == nil {
				teams = res.Teams
			}
			return rows(people, teams), nil
		}
	} else {
		opts = append(opts, picker.WithItems(rows(nil, nil)))
	}
	st := m.theme.Picker(m.icons)
	st.Frame = lipgloss.NewStyle()
	opts = append(opts, picker.WithStyles(st))
	o.p = picker.New(search, opts...)
	o.p.SetMarked(o.marks())
	m.open(o)
}

// applyReviewers asks the people in add for a review and withdraws the requests
// to those in remove, which the pane of the people shows at once. A request
// that is pending already, or a withdrawal of one that is not, is told as such,
// not as a failure.
func (m *panesModal) applyReviewers(add, remove []pulls.Reviewer) tea.Cmd {
	var ops []*optimistic.Op
	var labels []string
	if len(remove) > 0 {
		ops = append(ops, m.svc.RemoveReviewRequests(m.repo, m.number, remove))
		labels = append(labels, "remove review requests on #"+strconv.Itoa(m.number))
	}
	if len(add) > 0 {
		ops = append(ops, m.svc.RequestReviews(m.repo, m.number, add))
		labels = append(labels, "request reviews on #"+strconv.Itoa(m.number))
	}
	if d, ok := m.svc.CachedGet(m.repo, m.number); ok {
		m.detail = d
		m.refreshInvolved()
	}
	about := core.Target{Repo: m.repo, Number: m.number}.String()
	cmds := make([]tea.Cmd, len(ops))
	for i, op := range ops {
		send := ui.Do(m.sendCtx, ui.PullsTitle, ui.About(about, op), labels[i])
		cmds[i] = func() tea.Msg {
			msg := send()
			if d, ok := msg.(ui.DoneMsg); ok {
				switch {
				case errors.Is(d.Err, pulls.ErrAlreadyRequested):
					return ui.Notify(toast.Info, pulls.ErrAlreadyRequested.Error())()
				case errors.Is(d.Err, pulls.ErrNotPending):
					return ui.Notify(toast.Info, notPendingText)()
				}
			}
			return msg
		}
	}
	return tea.Batch(cmds...)
}
