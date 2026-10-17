package dev

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/termtext"
)

const (
	// pullBodyLines is how much of the body of a pull request its preview
	// shows, and commentBodyLines how much of each comment of an issue.
	pullBodyLines    = 6
	commentBodyLines = 6
	// shownComments is how many comments of an issue its preview shows.
	shownComments = 3
	tabWidth      = 4
)

// PreviewOptions say how a preview is drawn.
type PreviewOptions struct {
	// Repo is the repository of the pull request: the items in it are named by
	// number alone.
	Repo   core.RepoRef
	Styles Styles
	// Now is the time that ages are told from.
	Now time.Time
	// Width is the width of the preview in cells.
	Width int
}

// Loading is what a preview shows while its read is out.
func Loading(o PreviewOptions) string {
	return o.Styles.dim.Render("loading" + o.Styles.Glyphs.Ellipsis)
}

// Nothing is what a preview shows when there is nothing to show.
func Nothing(o PreviewOptions) string {
	return o.Styles.dim.Render("nothing here yet")
}

// HeaderPreview lists the items of a group: the issues the pull request
// closes, or the pull requests linked to it.
func HeaderPreview(group string, links core.PullLinks, o PreviewOptions) string {
	var refs []core.Reference
	if group == GroupCloses {
		for i := range links.Closes {
			refs = append(refs, links.Closes[i].Issue)
		}
	} else {
		refs = links.Linked
	}
	if len(refs) == 0 {
		return o.Styles.dim.Render("nothing linked")
	}
	b := builder{o: Options{Repo: o.Repo, Styles: o.Styles}, st: o.Styles}
	lines := make([]string, 0, len(refs))
	for i := range refs {
		r := refs[i]
		prefix := b.glyph(r) + " "
		state := o.Styles.stateWord(r)
		avail := o.Width - lipgloss.Width(prefix)
		if state != "" {
			avail -= 1 + len(state)
		}
		line := prefix + termtext.Truncate(b.text(r), max(avail, 1), o.Styles.Glyphs.Ellipsis)
		if state != "" {
			line += " " + o.Styles.dim.Render(state)
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

// IssuePreview is the preview of an issue, or of one that blocks another.
func IssuePreview(p core.IssuePreview, o PreviewOptions) string {
	st := o.Styles
	b := builder{o: Options{Repo: o.Repo, Styles: st}, st: st}
	out := st.titleLines(b.ident(p.Reference)+" "+ui.OneLine(p.Title), st.stateWord(p.Reference), o.Width)
	meta := b.glyph(p.Reference) + " " + st.stateWord(p.Reference)
	if p.Author != "" {
		meta += " " + st.Glyphs.Separator + " " + ui.OneLine(p.Author) + " opened this"
		if !p.CreatedAt.IsZero() {
			meta += " " + ui.AgoProse(p.CreatedAt, o.Now)
		}
	}
	out = append(out, meta)
	out = append(out, st.chipLines(p.Labels, o.Width)...)
	if p.BlockersSupported {
		for i := range p.Blockers {
			bl := p.Blockers[i]
			line := st.Glyphs.Blocked + " blocked by " + b.ident(bl)
			if bl.Title != "" {
				line += " " + ui.OneLine(bl.Title)
			}
			out = append(out, st.warn.Render(termtext.Truncate(line, o.Width, st.Glyphs.Ellipsis)))
		}
	}
	if body := wrapText(p.Body, o.Width); len(body) > 0 {
		out = append(out, "")
		out = append(out, body...)
	}
	out = append(out, "", st.dim.Render(strings.Repeat(st.Glyphs.Rule, max(o.Width, 0))),
		st.bold.Render(plural(p.CommentsTotal, "comment")))
	comments := p.Comments
	if len(comments) > shownComments {
		comments = comments[len(comments)-shownComments:]
	}
	for _, c := range comments {
		head := ui.OneLine(c.Author)
		if !c.CreatedAt.IsZero() {
			head += " " + st.Glyphs.Separator + " " + ui.AgoProse(c.CreatedAt, o.Now)
		}
		out = append(out, "", st.dim.Render(termtext.Truncate(head, o.Width, st.Glyphs.Ellipsis)))
		out = append(out, st.cut(wrapText(c.Body, o.Width), commentBodyLines)...)
	}
	return strings.Join(out, "\n")
}

// PullPreview is the preview of a linked pull request.
func PullPreview(p core.PullPreview, o PreviewOptions) string {
	st := o.Styles
	b := builder{o: Options{Repo: o.Repo, Styles: st}, st: st}
	var out []string
	for i, l := range wrapText(b.ident(p.Reference)+" "+ui.OneLine(p.Title), max(o.Width-2, 1)) {
		if i == 0 {
			l = b.glyph(p.Reference) + " " + l
		} else {
			l = "  " + l
		}
		out = append(out, st.bold.Render(l))
	}
	if p.Author != "" || p.Head != "" {
		line := "by " + ui.OneLine(p.Author)
		if p.Head != "" {
			line += " " + st.Glyphs.Separator + " " + ui.OneLine(p.Head) + " " + st.Glyphs.Arrow + " " + ui.OneLine(p.Base)
		}
		out = append(out, termtext.Truncate(line, o.Width, st.Glyphs.Ellipsis))
	}
	if t := p.Checks.Total(); t > 0 {
		line := fmt.Sprintf("checks: %d of %d passing", p.Checks.Passed, t)
		if p.Checks.Failed > 0 {
			line += fmt.Sprintf(", %d failing", p.Checks.Failed)
		}
		if p.Checks.Pending > 0 {
			line += fmt.Sprintf(", %d pending", p.Checks.Pending)
		}
		out = append(out, line)
	}
	if line := reviewLine(p); line != "" {
		out = append(out, termtext.Truncate(line, o.Width, st.Glyphs.Ellipsis))
	}
	if p.ChangedFiles > 0 || p.Additions > 0 || p.Deletions > 0 {
		out = append(out, fmt.Sprintf("changes: +%d %s%d in %s", p.Additions, st.Glyphs.Minus, p.Deletions, plural(p.ChangedFiles, "file")))
	}
	if chips := st.chipLines(p.Labels, max(o.Width-len("labels: "), 1)); len(chips) > 0 {
		for i, l := range chips {
			if i == 0 {
				l = "labels: " + l
			} else {
				l = "        " + l
			}
			out = append(out, l)
		}
	}
	if body := wrapText(p.Body, o.Width); len(body) > 0 {
		out = append(out, st.dim.Render(strings.Repeat(st.Glyphs.Rule, max(o.Width, 0))))
		out = append(out, st.cut(body, pullBodyLines)...)
	}
	return strings.Join(out, "\n")
}

// reviewLine says what the reviews of a pull request amount to, or is empty
// when none is required or given.
func reviewLine(p core.PullPreview) string {
	names := func(s core.ReviewState) string {
		var n []string
		for _, r := range p.Reviews {
			if r.State == s {
				n = append(n, ui.OneLine(r.Author))
			}
		}
		return strings.Join(n, ", ")
	}
	var text string
	switch p.ReviewDecision {
	case core.ReviewApproved:
		text = "approved"
		if n := names(core.ReviewStateApproved); n != "" {
			text += " by " + n
		}
	case core.ReviewChangesRequested:
		text = "changes requested"
		if n := names(core.ReviewStateChangesRequested); n != "" {
			text += " by " + n
		}
	case core.ReviewRequired:
		text = "required"
	default:
		return ""
	}
	return "review: " + text
}

func plural(n int, word string) string {
	if n == 1 {
		return "1 " + word
	}
	return strconv.Itoa(n) + " " + word + "s"
}

// wrapText cleans text of what a terminal would act on, wraps it to width,
// and drops the blank lines at its ends and the repeats within.
func wrapText(text string, width int) []string {
	width = max(width, 1)
	var out []string
	for line := range strings.SplitSeq(termtext.Clean(text, tabWidth), "\n") {
		line = strings.TrimRight(line, " \r")
		if line == "" {
			if len(out) > 0 && out[len(out)-1] != "" {
				out = append(out, "")
			}
			continue
		}
		out = append(out, strings.Split(ansi.Wrap(line, width, ""), "\n")...)
	}
	for len(out) > 0 && out[len(out)-1] == "" {
		out = out[:len(out)-1]
	}
	return out
}

// cut keeps the first n lines, and ends them with a dim ellipsis when there
// were more.
func (s Styles) cut(lines []string, n int) []string {
	if len(lines) <= n {
		return lines
	}
	return append(append([]string(nil), lines[:n]...), s.dim.Render(s.Glyphs.Ellipsis))
}

// titleLines is title wrapped and in bold, with state dim at the right of its
// first line.
func (s Styles) titleLines(title, state string, width int) []string {
	room := max(width-len(state)-1, 1)
	lines := wrapText(title, room)
	if len(lines) == 0 {
		lines = []string{""}
	}
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = s.bold.Render(l)
	}
	if gap := width - lipgloss.Width(lines[0]) - len(state); gap > 0 && state != "" {
		out[0] += strings.Repeat(" ", gap) + s.dim.Render(state)
	}
	return out
}

// chipLines lays the chips of labels out in lines of at most width cells.
func (s Styles) chipLines(labels []core.Label, width int) []string {
	var out []string
	var line string
	var used int
	for _, l := range labels {
		chip, w := s.chips.Get(l)
		if used > 0 && used+1+w > width {
			out = append(out, line)
			line, used = "", 0
		}
		if used > 0 {
			line += " "
			used++
		}
		line += chip
		used += w
	}
	if used > 0 {
		out = append(out, line)
	}
	return out
}
