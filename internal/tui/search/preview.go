package search

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/keyhelp"
	"github.com/eggzec/gh-tui/pkg/bubbles/keymap"
	"github.com/eggzec/gh-tui/pkg/bubbles/pager"
	"github.com/eggzec/gh-tui/pkg/markdown"
)

// previewBody is what the preview shows of the result under the cursor.
type previewBody struct {
	// key identifies the result and the query it was found for, so that the
	// pager is filled once for each.
	key  string
	name string
	// text builds what the pager shows as it is; it is only called when the
	// key changed.
	text func() string
	// issue, for an issue or a pull request, builds the header and the
	// markdown body, which the pager shows rendered.
	issue func() (head, body string)
	// find is the text the pager jumps to, which is the query where the
	// result's text holds it.
	find string
}

// What the pager says of -N over a rendered body: its lines are rendered
// rows, and only wide code blocks are chopped.
const (
	renderedNumbersNote = "Rows numbered"
	renderedChopNote    = "code only"
)

// newPager returns the pager of the preview, which reads the file keys of
// the preview's context.
func newPager(k KeyMap) pager.Model {
	// A rendered body renders again at once on a new width: the section's
	// host has no step that settles a resize.
	return pager.New(pager.WithKeyMap(k.pager), pager.WithRenderedNotes(renderedNumbersNote, renderedChopNote))
}

// previewed returns what the preview shows for the result under the cursor,
// with the key first and the text left to be built once the key is new.
func (s *Section) previewed() previewBody {
	if l, ok := s.visibleCode(); ok {
		h, ok := l.feed.Selected()
		if !ok {
			return previewBody{}
		}
		return previewBody{
			key:  "code " + h.Repo.String() + ":" + h.Path + "@" + h.SHA + " " + s.text,
			name: h.Path,
			text: func() string { return codeText(h) },
			find: s.text,
		}
	}
	l, ok := s.visibleHits()
	if !ok {
		return previewBody{}
	}
	h, ok := l.feed.Selected()
	if !ok {
		return previewBody{}
	}
	if h.Kind == core.SearchRepos {
		suffix, add := s.extraText(h)
		return previewBody{key: "repo " + hitKey(h) + suffix, name: h.Repo.Ref.String(), text: func() string { return repoText(h.Repo) + add }}
	}
	if h.Kind == core.SearchTopics {
		return previewBody{key: "topic " + h.Topic.Name, name: h.Topic.Name, text: func() string { return topicText(h.Topic) }}
	}
	if h.Kind == core.SearchCommits {
		return previewBody{key: "commit " + hitKey(h), name: h.Commit.Repo.String() + "@" + h.Commit.SHA, text: func() string { return commitText(h.Commit) }}
	}
	if h.Kind == core.SearchUsers {
		return previewBody{key: "user " + h.User.Login, name: h.User.Login, text: func() string { return userText(h.User) }}
	}
	return previewBody{
		key:   "issue " + hitKey(h) + " " + s.text,
		name:  hitKey(h),
		issue: func() (string, string) { return issueParts(h) },
		find:  s.text,
	}
}

// markdownRender returns what renders an issue's header as it is, wrapped,
// and its body as markdown, in the style of the theme.
func (s *Section) markdownRender(head, body string) pager.Render {
	return func(width int) string {
		if s.md == nil {
			s.md = markdown.New(s.theme.Thread(s.icons).Markdown)
			s.md.SetGlyphs(s.icons.Markdown)
		}
		out := ansi.Wrap(strings.TrimRight(head, "\n"), width, "")
		if body != "" {
			out += "\n\n" + s.md.Render(body, width)
		}
		return out
	}
}

// topicText describes a topic from what its hit holds.
func topicText(t core.SearchTopic) string {
	if t.ShortDescription == "" {
		return t.Name + "\n"
	}
	return t.Name + "\n\n" + t.ShortDescription + "\n"
}

// commitText describes a commit from what its hit holds: the search gives
// its message's first line, and nothing more is read.
func commitText(c core.SearchCommit) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n\n", c.Subject)
	fmt.Fprintf(&b, "Author: %s\n", c.Author)
	if !c.Date.IsZero() {
		fmt.Fprintf(&b, "Date: %s\n", c.Date.Format("2006-01-02 15:04"))
	}
	fmt.Fprintf(&b, "Repository: %s\nCommit: %s\n", c.Repo, c.SHA)
	return b.String()
}

// repoText describes a repository from what its hit holds.
func repoText(r core.Repo) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n\n", r.Ref)
	if r.Description != "" {
		fmt.Fprintf(&b, "%s\n\n", r.Description)
	}
	if r.Language != "" {
		fmt.Fprintf(&b, "Language: %s\n", r.Language)
	}
	fmt.Fprintf(&b, "Stars: %d\n", r.Stars)
	if !r.UpdatedAt.IsZero() {
		fmt.Fprintf(&b, "Updated: %s\n", r.UpdatedAt.Format("2006-01-02"))
	}
	if len(r.Topics) > 0 {
		fmt.Fprintf(&b, "Topics: %s\n", strings.Join(r.Topics, ", "))
	}
	for _, f := range []struct {
		on   bool
		name string
	}{{r.Private, "private"}, {r.Fork, "fork"}, {r.Archived, "archived"}, {r.Template, "template"}, {r.Mirror, "mirror"}} {
		if f.on {
			fmt.Fprintf(&b, "%s\n", f.name)
		}
	}
	return b.String()
}

// userText describes a user or an organization from what its hit holds.
func userText(u core.SearchUser) string {
	kind := "User"
	if u.Kind == core.OwnerOrg {
		kind = "Organization"
	}
	return fmt.Sprintf("%s\n\nType: %s\n", u.Login, kind)
}

// issueText describes an issue or a pull request: its title, who opened it
// and in what state, its labels and its body.
func issueText(h core.SearchHit) string {
	head, body := issueParts(h)
	if body == "" {
		return head
	}
	return head + "\n" + body + "\n"
}

// issueParts splits an issue's text into its header lines and its body.
func issueParts(h core.SearchHit) (head, body string) {
	i := h.Issue
	state := fmt.Sprint(i.State)
	if h.Draft {
		state += " (draft)"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n\n", i.Title)
	fmt.Fprintf(&b, "State: %s\n", state)
	if i.Author.Login != "" {
		fmt.Fprintf(&b, "Author: %s\n", i.Author.Login)
	}
	if len(i.Labels) > 0 {
		names := make([]string, len(i.Labels))
		for j, l := range i.Labels {
			names[j] = l.Name
		}
		fmt.Fprintf(&b, "Labels: %s\n", strings.Join(names, ", "))
	}
	return b.String(), i.Body
}

// codeText is the path of a code hit with the fragments that matched.
func codeText(h core.CodeHit) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s %s\n", h.Repo, h.Path)
	for _, f := range h.Fragments {
		fmt.Fprintf(&b, "\n%s\n", strings.TrimRight(f.Text, "\n"))
	}
	return b.String()
}

// syncPreview replaces what the pager shows when the result under the cursor
// or the query changed, and jumps to the first match of the query where the
// text has one.
func (s *Section) syncPreview() tea.Cmd {
	p := s.previewed()
	if p.key == s.pvKey {
		return nil
	}
	s.pvKey = p.key
	var cmd tea.Cmd
	switch {
	case p.issue != nil:
		head, body := p.issue()
		raw := head
		if body != "" {
			raw += "\n" + body + "\n"
		}
		s.pv.SetRendered(p.name, raw, s.markdownRender(head, body))
	case p.text != nil:
		cmd = s.pv.SetContent(p.name, p.text())
	default:
		// Nothing is selected, so nothing of the last hit may stay.
		cmd = s.pv.SetContent("", "")
	}
	if p.find != "" {
		cmd = tea.Batch(cmd, s.pv.SetSearch(p.find))
	}
	return cmd
}

// pressPreview handles a key in the preview: the keys that act on the result
// still do, and the pager reads the rest.
func (s *Section) pressPreview(msg tea.KeyPressMsg) tea.Cmd {
	k := &s.keys
	if !s.pv.Capturing() && (keymap.Matches(msg, k.Select) || keymap.Matches(msg, k.Open) || keymap.Matches(msg, k.Refresh)) {
		return s.pressResults(msg)
	}
	var cmd tea.Cmd
	s.pv, cmd = s.pv.Update(msg)
	return cmd
}

// previewLayer returns the keys of the preview.
func (s *Section) previewLayer() keyhelp.Layer {
	return ui.PagerLayer("search_preview", &s.pv, s.keys.Select, s.keys.Open, s.keys.Refresh)
}
