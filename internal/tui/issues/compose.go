package issues

import (
	"slices"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/prompt"
)

// composing is what the prompt under the thread is for.
type composing int

const (
	composeNone composing = iota
	composeComment
	composeLabels
)

// Heights of the comment prompt, which takes a third of the detail within
// these bounds.
const (
	minCommentHeight = 5
	maxCommentHeight = 10
)

// compose opens the prompt for what in the detail, under the thread, and
// gives it the focus.
func (s *Section) compose(what composing) tea.Cmd {
	num := "#" + strconv.Itoa(s.issue.Number)
	opts := []prompt.Option{prompt.WithStyles(s.theme.Prompt())}
	switch what {
	case composeComment:
		opts = append(opts,
			prompt.WithTitle("Comment on "+num),
			prompt.WithPlaceholder("Write a comment in markdown."))
	case composeLabels:
		names := make([]string, len(s.issue.Labels))
		for i, l := range s.issue.Labels {
			names[i] = l.Name
		}
		opts = append(opts,
			prompt.WithMode(prompt.SingleLine),
			prompt.WithTitle("Labels of "+num),
			prompt.WithPlaceholder("Comma-separated, such as bug, help wanted"),
			prompt.WithValue(strings.Join(names, ", ")))
	default:
		return nil
	}
	s.prompt = prompt.New(opts...)
	s.composing = what
	s.layoutDetail()
	s.detail.Blur()
	if !s.focused {
		return nil
	}
	return s.prompt.Focus()
}

// closePrompt closes the prompt and gives the focus back to the thread.
func (s *Section) closePrompt() {
	if s.composing == composeNone {
		return
	}
	s.composing = composeNone
	s.prompt.Blur()
	s.layoutDetail()
	if s.focused && s.inDetail {
		s.detail.Focus()
	}
}

// Capturing implements ui.Capturer: while the prompt is open, every key is
// typing.
func (s *Section) Capturing() bool {
	return s.inDetail && s.composing != composeNone && s.focused
}

// layoutDetail shares the height under the bar between the thread and the
// prompt, if it is open.
func (s *Section) layoutDetail() {
	h := s.bodyHeight()
	ph := 0
	switch s.composing {
	case composeComment:
		ph = min(max(h/3, minCommentHeight), maxCommentHeight, h)
	case composeLabels:
		ph = min(prompt.SingleLineHeight, h)
	case composeNone:
	}
	if ph > 0 {
		s.prompt.SetSize(s.width, ph)
	}
	s.detail.SetSize(s.width, h-ph)
}

// promptDone handles what the prompt reports, if it is this section's.
func (s *Section) promptDone(msg tea.Msg) tea.Cmd {
	if s.composing == composeNone {
		return nil
	}
	switch msg := msg.(type) {
	case prompt.CancelMsg:
		if msg.ID == s.prompt.ID() {
			s.closePrompt()
		}
	case prompt.SubmitMsg:
		if msg.ID != s.prompt.ID() {
			return nil
		}
		if s.composing == composeComment {
			return s.submitComment(msg.Value)
		}
		return s.submitLabels(msg.Value)
	}
	return nil
}

// submitComment sends body as a comment on the open issue. The service
// shows it in the cache at once, as sending, so the thread reloads to show
// it. An empty comment is not sent, and the prompt stays open.
func (s *Section) submitComment(body string) tea.Cmd {
	body = strings.TrimSpace(body)
	if body == "" {
		return nil
	}
	s.closePrompt()
	n := s.issue.Number
	op := s.svc.Comment(s.repo, n, body)
	return tea.Batch(s.reload(), s.detail.Reload(),
		ui.Do(s.ctx, ui.IssuesTitle, op, "comment on #"+strconv.Itoa(n)))
}

// submitLabels makes the open issue's labels the comma-separated names in
// typed: it adds the new ones in one change and removes each dropped one.
// The changes go one after another, so each answer from GitHub, which holds
// every label, includes the changes before it.
func (s *Section) submitLabels(typed string) tea.Cmd {
	s.closePrompt()
	added, removed := labelDiff(s.issue.Labels, typed)
	if len(added) == 0 && len(removed) == 0 {
		return nil
	}
	n := s.issue.Number
	num := "#" + strconv.Itoa(n)
	cmds := make([]tea.Cmd, 0, len(removed)+1)
	if len(added) > 0 {
		op := s.svc.AddLabels(s.repo, n, added)
		cmds = append(cmds, ui.Do(s.ctx, ui.IssuesTitle, op, "add "+strings.Join(added, ", ")+" to "+num))
	}
	for _, name := range removed {
		op := s.svc.RemoveLabel(s.repo, n, name)
		cmds = append(cmds, ui.Do(s.ctx, ui.IssuesTitle, op, "remove "+name+" from "+num))
	}
	return tea.Batch(s.reload(), tea.Sequence(cmds...))
}

// labelDiff compares the comma-separated label names in typed with the
// labels an issue has. GitHub treats names that differ only in case as the
// same label, so the comparison does too, and space around names doesn't
// count. Added names keep their order and removed ones the issue's.
func labelDiff(have []core.Label, typed string) (added, removed []string) {
	var want []string
	for name := range strings.SplitSeq(typed, ",") {
		name = strings.TrimSpace(name)
		if name != "" && !slices.ContainsFunc(want, sameLabel(name)) {
			want = append(want, name)
		}
	}
	for _, name := range want {
		if !slices.ContainsFunc(have, func(l core.Label) bool { return sameLabel(name)(l.Name) }) {
			added = append(added, name)
		}
	}
	for _, l := range have {
		if !slices.ContainsFunc(want, sameLabel(l.Name)) {
			removed = append(removed, l.Name)
		}
	}
	return added, removed
}

func sameLabel(name string) func(string) bool {
	return func(other string) bool { return strings.EqualFold(strings.TrimSpace(other), name) }
}
