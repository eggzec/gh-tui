package pulls

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tray"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/composer"
	"github.com/eggzec/gh-tui/pkg/bubbles/diff"
	"github.com/eggzec/gh-tui/pkg/bubbles/toast"
)

// draftPrefix starts the ID of the block of a draft in the diff.
const draftPrefix = "draft:"

// draftReviewer is what a service that sends the review comments with the
// review adds to Service: the real one has it, and a service without it
// sends no comments.
type draftReviewer interface {
	SubmitReview(ctx context.Context, repo core.RepoRef, number int, head string, event core.ReviewEvent, body string, drafts []tray.Draft) error
}

// draftStore returns where the review comments of the modal are kept, or nil
// for a modal that has no section.
func (m *panesModal) draftStore() ReviewDrafts {
	if m.list == nil {
		return nil
	}
	return m.list.reviewDrafts
}

// draftKey is the key of the review comments of the pull request. The store is
// already of the host and the account.
func (m *panesModal) draftKey() string { return tray.Key(m.repo.String(), m.number) }

// reviewDrafts reads the review comments written on the pull request.
func (m *panesModal) reviewDrafts() []tray.Draft {
	store := m.draftStore()
	if store == nil {
		return nil
	}
	drafts, err := store.Load(m.draftKey())
	if err != nil && !errors.Is(err, tray.ErrDamaged) {
		slog.WarnContext(m.ctx, "review drafts not read", "span", "tui", "err", ui.ShortPath(err.Error()))
	}
	return drafts
}

// staleDrafts counts the drafts that were written on another commit than head,
// which the service refuses to send as one review.
func staleDrafts(drafts []tray.Draft, head string) int {
	n := 0
	for i := range drafts {
		d := &drafts[i]
		if d.Commit != "" && d.Commit != head {
			n++
		}
	}
	return n
}

// plural returns n with noun, in the plural unless n is 1.
func plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return strconv.Itoa(n) + " " + noun + "s"
}

// draftRange says which lines a draft is on.
func draftRange(d tray.Draft) string {
	switch {
	case d.Line == 0:
		return "file"
	case d.StartLine > 0 && d.StartLine != d.Line:
		return "L" + strconv.Itoa(d.StartLine) + "–" + strconv.Itoa(d.Line)
	}
	return "L" + strconv.Itoa(d.Line)
}

// draftWhere names where a draft is, for a question.
func draftWhere(d tray.Draft) string {
	if d.Line == 0 {
		return d.Path + " (file)"
	}
	return d.Path + ":" + draftRange(d)
}

// isDraft says whether b is the block of a draft.
func isDraft(b diff.Block) bool { return strings.HasPrefix(b.ID, draftPrefix) }

// sideOf is the side of the diff a draft of side s is on.
func sideOf(s string) diff.Side {
	if s == tray.Left {
		return diff.OldSide
	}
	return diff.NewSide
}

// traySide is the side of a draft written on a line of side s.
func traySide(s diff.Side) string {
	if s == diff.OldSide {
		return tray.Left
	}
	return tray.Right
}

// noteCount is how many drafts the diff shows.
func (o *diffOverlay) noteCount() int {
	n := 0
	for i := range o.drafts {
		d := &o.drafts[i]
		if d.ThreadID == "" {
			n++
		}
	}
	return n
}

// readDrafts reads the drafts again and puts them in the diffs that show them:
// this one, and the other, which the modal has open or the zoomed Files pane
// shows beside its tree.
func (o *diffOverlay) readDrafts() { o.m.refreshDrafts(o) }

// refreshDrafts reads the drafts again and puts them in every diff that shows
// them: first, if given, the one the modal has open, and the zoomed Files
// pane's.
func (m *panesModal) refreshDrafts(first *diffOverlay) {
	drafts := m.reviewDrafts()
	var shown []*diffOverlay
	if first != nil {
		shown = append(shown, first)
	}
	if pv := &m.files.pv; pv.on && !slices.Contains(shown, pv.o) {
		shown = append(shown, pv.o)
	}
	for _, ov := range m.overlays {
		if d, ok := ov.overlay.(*diffOverlay); ok && !slices.Contains(shown, d) {
			shown = append(shown, d)
		}
	}
	for _, d := range shown {
		d.drafts = drafts
		d.setBlocks()
	}
	m.files.seq++
}

// setBlocks gives the diff one block for each draft, under the last line it is
// on, or under the header of its file. The threads and their marks come with
// it, as the range of commits decides whether the diff shows them.
func (o *diffOverlay) setBlocks() {
	o.markThreads()
	blocks := o.threadBlocks()
	for i := range o.drafts {
		d := &o.drafts[i]
		if d.ThreadID != "" {
			continue
		}
		blocks = append(blocks, o.block(*d))
	}
	o.d.SetBlocks(blocks)
}

// block is the row that shows a draft.
func (o *diffOverlay) block(d tray.Draft) diff.Block {
	m := o.m
	text := m.theme.Warning.Render("✎ draft " + draftRange(d) + ": " + strings.Join(strings.Fields(d.Body), " "))
	return diff.Block{
		ID:     draftPrefix + d.ID,
		Anchor: diff.Pos{Path: d.Path, Side: sideOf(d.Side), Line: d.Line},
		Render: func(width int, _ bool) []string {
			return []string{ansi.Truncate(text, width, m.icons.Ellipsis)}
		},
	}
}

// draftAt returns the draft the cursor is on.
func (o *diffOverlay) draftAt() (tray.Draft, bool) {
	b, ok := o.d.CursorBlock()
	if !ok || !isDraft(b) {
		return tray.Draft{}, false
	}
	for i := range o.drafts {
		d := &o.drafts[i]
		if draftPrefix+d.ID == b.ID {
			return *d, true
		}
	}
	return tray.Draft{}, false
}

// commit is the commit a draft written now is on: the last of the range shown,
// since the line numbers are those of it. A draft on an earlier commit than
// the head is refused when the review is sent, so the user is told then.
func (o *diffOverlay) commit() string {
	if o.at != "" {
		return o.at
	}
	f := &o.m.files
	cs := f.commits.Commits
	if r := f.rng; len(cs) > 0 && r.First >= 0 && r.Last < len(cs) && r.First <= r.Last {
		return cs[r.Last].OID
	}
	return o.m.detail.HeadSHA
}

// toggleRange starts a range of lines at the cursor, or ends the one started.
func (o *diffOverlay) toggleRange() tea.Cmd {
	if _, ok := o.d.Anchor(); ok {
		o.d.ClearAnchor()
		return nil
	}
	pos, ok := o.d.Position()
	if !ok {
		return ui.Notify(toast.Info, "Move to a line to start a range.")
	}
	o.d.SetAnchor(pos)
	return nil
}

// comment opens the composer of a draft on the file, the range or the line
// the cursor is on, or of the draft it is on.
func (o *diffOverlay) comment() tea.Cmd {
	if o.m.draftStore() == nil {
		return ui.Notify(toast.Info, "Review comments can't be kept here.")
	}
	if _, onBlock := o.d.CursorBlock(); onBlock {
		if d, ok := o.draftAt(); ok {
			return o.compose(d, true)
		}
		return ui.Notify(toast.Info, "Move to a line to comment on it.")
	}
	d := tray.Draft{Commit: o.commit(), Side: tray.Right}
	if o.d.OnHeader() {
		f, ok := o.d.CurrentFile()
		if !ok {
			return nil
		}
		d.Path = f.Path
		return o.compose(d, false)
	}
	pos, ok := o.d.Position()
	if !ok {
		return ui.Notify(toast.Info, "Move to a line to comment on it.")
	}
	d.Path, d.Side, d.Line = pos.Path, traySide(pos.Side), pos.Line
	d.Text, _, _ = o.d.Line(pos)
	d.HasText = true
	// A range reaches over one side of one file: from another, the line the
	// cursor is on is the draft.
	if a, ok := o.d.Anchor(); ok && a.Path == pos.Path && a.Side == pos.Side && a.Line != pos.Line {
		// GitHub refuses a range that starts in another hunk than it ends in,
		// and with it the whole review.
		if !o.d.SameHunk(a, pos) {
			return ui.Notify(toast.Info, "A range stays within one hunk.")
		}
		start, last := a, pos
		if start.Line > last.Line {
			start, last = last, start
		}
		d.StartLine, d.Line = start.Line, last.Line
		d.StartText, _, _ = o.d.Line(start)
		d.Text, _, _ = o.d.Line(last)
	}
	return o.compose(d, false)
}

// compose opens the composer of the body of d, which it keeps when the user
// saves, and which esc leaves as it was.
func (o *diffOverlay) compose(d tray.Draft, edit bool) tea.Cmd {
	m := o.m
	title := "Comment on " + draftWhere(d)
	c := composer.New("draft/"+m.repo.String()+"/"+strconv.Itoa(m.number),
		composer.WithTitle(title),
		composer.WithPlaceholder("Write the comment in markdown; it is sent with the review."),
		composer.WithKeyMap(m.keys.composer),
		composer.WithStyles(m.theme.Prompt(m.icons)),
		composer.WithSize(m.width, m.areaHeight()),
	)
	if edit {
		c.SetValue(d.Body)
	}
	co := &composerOverlay{m: m, comp: c, head: title, submit: func(_ *panesModal, co *composerOverlay) tea.Cmd {
		return o.saveDraft(d, co)
	}}
	m.open(co)
	return tea.Batch(co.comp.Init(), co.comp.Focus())
}

// saveDraft keeps the draft with the body the composer holds, and shows it.
func (o *diffOverlay) saveDraft(d tray.Draft, co *composerOverlay) tea.Cmd {
	d.Body = strings.TrimSpace(co.comp.Value())
	if d.Body == "" {
		return ui.Notify(toast.Info, "Nothing to save: the comment is empty.")
	}
	if _, err := o.m.draftStore().Put(o.m.draftKey(), d); err != nil {
		return ui.Fail("keep the draft", err)
	}
	co.done = true
	o.d.ClearAnchor()
	o.readDrafts()
	return nil
}

// askDelete asks to delete the draft the cursor is on.
func (o *diffOverlay) askDelete() tea.Cmd {
	d, ok := o.draftAt()
	if !ok {
		return ui.Notify(toast.Info, "Move to a draft to delete it.")
	}
	o.m.ask(ui.Confirm{Question: "Delete the draft " + draftWhere(d) + "?", Run: func() tea.Cmd {
		if err := o.m.draftStore().Remove(o.m.draftKey(), d.ID); err != nil {
			return ui.Fail("delete the draft", err)
		}
		o.readDrafts()
		return nil
	}})
	return nil
}

// reviewWithDrafts says what the review of the composer would send as its
// comments, and why it must not, if it must not.
func (m *panesModal) reviewWithDrafts(o *composerOverlay) (p reviewPlan, refusal string) {
	drafts := m.reviewDrafts()
	p, why := planReview(o.verdict, o.comp.Value(), m.detail.HeadSHA, m.number)
	if len(drafts) == 0 {
		return p, why
	}
	if _, ok := m.svc.(draftReviewer); !ok {
		return p, "Review comments can't be sent from here."
	}
	if n := staleDrafts(drafts, m.detail.HeadSHA); n > 0 {
		return p, fmt.Sprintf("Not sent: %s written on an older commit than the head. The drafts are kept.", plural(n, "draft"))
	}
	p.drafts = drafts
	return p, ""
}
