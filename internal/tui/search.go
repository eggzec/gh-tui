package tui

import (
	"context"

	"charm.land/bubbles/v2/help"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/picker"
)

// Kinds of search results, which name their groups in the search and its
// scopes. A picker.Search given to WithSearch labels its items with them.
const (
	// KindPinned lists the pinned repositories, offered before any query.
	KindPinned = "Pinned"
	KindRepos  = "Repositories"
	KindIssues = "Issues"
	KindPulls  = "Pull requests"
)

// searchTitle names the search in the top edge of its frame.
const searchTitle = "Search"

// searchModal is the search popup: a picker in a modal. Choosing a
// repository selects it, and choosing an issue or pull request asks the
// sections to open it.
type searchModal struct {
	picker picker.Model
}

var _ ui.Modal = (*searchModal)(nil)

func newSearchModal(ctx context.Context, search picker.Search) *searchModal {
	return &searchModal{picker: picker.New(search,
		picker.WithContext(ctx),
		picker.WithScopes(KindRepos, KindIssues, KindPulls),
		picker.WithPlaceholder("Search repositories, issues and pull requests"),
		picker.WithEmptyText("Nothing found. Try other words, or qualifiers such as is:open."),
	)}
}

// open focuses the picker and lists the first results. first is whether
// the modal opens for the first time, when the picker has yet to start.
func (s *searchModal) open(first bool) tea.Cmd {
	focus := s.picker.Focus()
	if first {
		return tea.Batch(focus, s.picker.Init())
	}
	return tea.Batch(focus, s.picker.Reset())
}

func (s *searchModal) Title() string { return searchTitle }

func (s *searchModal) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case picker.ChosenMsg:
		if msg.ID != s.picker.ID() {
			return nil
		}
		s.picker.Blur()
		// Closing the search closes the modals over it too, so it closes
		// before a section opens the chosen item in one.
		return tea.Sequence(ui.CloseModal(s), choose(msg.Item.Value))
	case picker.CancelMsg:
		if msg.ID != s.picker.ID() {
			return nil
		}
		s.picker.Blur()
		return ui.CloseModal(s)
	}
	var cmd tea.Cmd
	s.picker, cmd = s.picker.Update(msg)
	return cmd
}

func (s *searchModal) View() string { return s.picker.View() }

func (s *searchModal) SetSize(width, height int) { s.picker.SetSize(width, height) }

func (s *searchModal) SetTheme(t ui.Theme) {
	st := t.Picker()
	// The app draws the modal's frame.
	st.Frame = lipgloss.NewStyle()
	s.picker.SetStyles(st)
}

func (s *searchModal) Help() help.KeyMap { return s.picker.KeyMap() }

// choose returns the message that opens what a search item stands for.
func choose(v any) tea.Cmd {
	var msg tea.Msg
	switch v := v.(type) {
	case core.RepoRef:
		msg = ui.RepoMsg{Repo: v}
	case core.Repo:
		msg = ui.RepoMsg{Repo: v.Ref}
	case core.SearchHit:
		switch v.Kind {
		case core.SearchRepos:
			msg = ui.RepoMsg{Repo: v.Repo.Ref}
		case core.SearchIssues:
			msg = ui.OpenIssueMsg{Repo: v.Issue.Repo, Number: v.Issue.Number}
		case core.SearchPulls:
			msg = ui.OpenPullMsg{Repo: v.Issue.Repo, Number: v.Issue.Number}
		default:
			return nil
		}
	default:
		return nil
	}
	return func() tea.Msg { return msg }
}
