package pulls

import (
	"errors"
	"log/slog"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/pulls"
	"github.com/eggzec/gh-tui/internal/tray"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/composer"
	"github.com/eggzec/gh-tui/pkg/bubbles/keyhelp"
	"github.com/eggzec/gh-tui/pkg/bubbles/keymap"
	"github.com/eggzec/gh-tui/pkg/bubbles/panes"
	"github.com/eggzec/gh-tui/pkg/bubbles/toast"
)

// outdatedPendingText is what the user reads when the review they started on
// GitHub is on an older commit than the head: the service refuses to send into
// it, so a review sent from here would not be one of the head.
const outdatedPendingText = "Your pending review on GitHub is on an older commit than the head; update or discard it on GitHub first."

// pendingAppearedText is what the user reads when a review sent with comments
// found one pending on GitHub, which holds comments made elsewhere: the
// drafts are kept.
const pendingAppearedText = "You have a pending review on GitHub; submit or discard it there, then send this one. Your drafts are kept."

// verdicts are the choices of the picker, in the order they show.
var verdicts = [...]struct {
	event core.ReviewEvent
	// label names the verdict, hint says what it does, and verb is what a
	// review of it only does when it has no summary.
	label, hint, verb string
}{
	{core.ReviewEventComment, "Comment", "General feedback, without an approval.", "comments"},
	{core.ReviewEventApprove, "Approve", "Approve the changes.", "approves"},
	{core.ReviewEventRequestChanges, "Request changes", "Ask for changes before it can merge.", "requests changes"},
}

// reviewPlan is what a submit would send.
type reviewPlan struct {
	event       core.ReviewEvent
	label, body string
	head        string
	number      int
	// drafts are the review comments that go with it.
	drafts []tray.Draft
}

// planReview returns what a submit of the verdict (a row of verdicts) with
// summary would send, or why it must not.
func planReview(verdict int, summary, head string, number int) (p reviewPlan, refusal string) {
	v := verdicts[verdict]
	p = reviewPlan{event: v.event, label: v.label, body: strings.TrimSpace(summary), head: head, number: number}
	if p.body == "" && p.event != core.ReviewEventApprove {
		return p, "Nothing to send: GitHub refuses a review that only " + v.verb + " with no summary and no comments."
	}
	return p, ""
}

// question asks to submit the plan.
func (p reviewPlan) question() string {
	q := "Submit review (" + p.label + ") on #" + strconv.Itoa(p.number)
	if n := len(p.drafts); n > 0 {
		q += " with " + plural(n, "comment")
	}
	return q + "?"
}

// reviewedMsg reports that GitHub answered the review sent by the modal.
type reviewedMsg struct {
	owner *panesModal
	err   error
}

// gate is what refuses an action the viewer may not take here.
func (m *panesModal) gate() ui.Gate {
	return ui.Gate{Repo: m.repo, Caps: m.caps, Token: m.token, Icons: m.icons}
}

// openReview opens the verdicts, if the viewer may comment and the head is
// known.
func (m *panesModal) openReview() tea.Cmd {
	if !m.loaded {
		return nil
	}
	if cmd, refused := m.gate().Refuse(ui.ActComment, &m.detail.Issue); refused {
		return cmd
	}
	if m.detail.HeadSHA == "" {
		return ui.Notify(toast.Info, "The head commit is not known yet; try again once the pull request has loaded.")
	}
	m.open(&verdictOverlay{m: m})
	return nil
}

// submitReview plans the review from the summary the composer holds, and asks
// about it. By the time the user says yes the head may have changed or the
// viewer lost the right, so the plan is made again, and sent only if it asks
// the same.
func (m *panesModal) submitReview(o *composerOverlay) tea.Cmd {
	if m.sending {
		return nil
	}
	p, why := m.reviewWithDrafts(o)
	if why != "" {
		return ui.Notify(toast.Info, why)
	}
	m.ask(ui.Recheck(p.question(), "the review", func() (ui.Confirm, bool, tea.Cmd) {
		if m.closed || m.sending {
			return ui.Confirm{}, false, nil
		}
		p, why := m.reviewWithDrafts(o)
		if why != "" {
			return ui.Confirm{}, false, ui.Notify(toast.Info, why)
		}
		if cmd, refused := m.gate().Refuse(ui.ActComment, &m.detail.Issue); refused {
			return ui.Confirm{}, false, cmd
		}
		return ui.Confirm{Question: p.question(), Run: func() tea.Cmd { return m.sendReview(p) }}, true, nil
	}))
	return nil
}

// sendReview sends the review. The composer stays open, with the summary,
// until GitHub answers.
func (m *panesModal) sendReview(p reviewPlan) tea.Cmd {
	m.sending = true
	svc, ctx, repo, n := m.svc, m.sendCtx, m.repo, m.number
	store, dkey := m.draftStore(), m.draftKey()
	return func() tea.Msg {
		dr, ok := svc.(draftReviewer)
		if !ok || len(p.drafts) == 0 {
			return reviewedMsg{owner: m, err: svc.SendReview(ctx, repo, n, p.head, p.event, p.body)}
		}
		err := dr.SubmitReview(ctx, repo, n, p.head, p.event, p.body, p.drafts)
		if err == nil {
			// The comments are on GitHub now; a failure leaves them kept.
			for i := range p.drafts {
				if rerr := store.Remove(dkey, p.drafts[i].ID); rerr != nil {
					slog.WarnContext(ctx, "sent review draft not removed", "span", "tui", "err", ui.ShortPath(rerr.Error()))
				}
			}
		}
		return reviewedMsg{owner: m, err: err}
	}
}

// reviewed settles the review once GitHub answered. A refusal leaves the
// summary as it was, and says why; a review that went through closes the
// composer, and what it changed is read again.
func (m *panesModal) reviewed(msg reviewedMsg) tea.Cmd {
	if msg.owner != m {
		return nil
	}
	m.sending = false
	num := "#" + strconv.Itoa(m.number)
	if msg.err != nil {
		slog.WarnContext(m.ctx, "review not submitted", "span", "tui", "err", ui.ShortPath(msg.err.Error()))
		if errors.Is(msg.err, pulls.ErrPendingReviewOutdated) {
			return ui.Notify(toast.Error, outdatedPendingText)
		}
		if errors.Is(msg.err, pulls.ErrPendingAppeared) {
			return ui.Notify(toast.Error, pendingAppearedText)
		}
		return ui.Fail("submit the review of "+num, msg.err)
	}
	slog.InfoContext(m.ctx, "review submitted", "span", "tui")
	m.overlays = nil
	return tea.Batch(ui.Notify(toast.Success, "Review submitted on "+num+"."), m.revalidate())
}

// verdictOverlay picks the verdict of a review. Up and down move, select
// chooses and opens the composer of the summary, and back or dismiss closes it.
type verdictOverlay struct {
	m      *panesModal
	cursor int
	done   bool
}

func (o *verdictOverlay) title() string           { return "Review #" + strconv.Itoa(o.m.number) }
func (o *verdictOverlay) kind() panes.OverlayKind { return panes.Picker }

// items is two rows for each verdict: its label and its hint.
func (o *verdictOverlay) items() int   { return 2 * len(verdicts) }
func (o *verdictOverlay) text() int    { return 0 }
func (o *verdictOverlay) clear() bool  { return false }
func (o *verdictOverlay) closed() bool { return o.done }

func (o *verdictOverlay) layers() []keyhelp.Layer {
	k := o.m.keys
	return []keyhelp.Layer{ui.ContextLayer(ctxModal,
		[]key.Binding{k.scroll.Up, k.scroll.Down, k.choose, k.back, k.cancel},
		[]key.Binding{k.choose, k.cancel})}
}

func (o *verdictOverlay) view(w, h int) string {
	m := o.m
	lines := make([]string, 0, 2*len(verdicts))
	for i, v := range verdicts {
		mark := "  "
		label := v.label
		if i == o.cursor {
			mark = m.theme.Accent.Render(m.icons.Dot) + " "
			label = m.theme.Accent.Render(label)
		}
		lines = append(lines,
			ansi.Truncate(" "+mark+label, w, m.icons.Ellipsis),
			ansi.Truncate("     "+m.theme.Muted.Render(v.hint), w, m.icons.Ellipsis))
	}
	return strings.Join(ui.FitLines(lines, w, h), "\n")
}

// cancel closes the verdicts, for the back key, which the modal takes.
func (o *verdictOverlay) cancel() { o.done = true }

func (o *verdictOverlay) update(msg tea.Msg) tea.Cmd {
	press, ok := msg.(tea.KeyPressMsg)
	if !ok || o.done {
		return nil
	}
	k := o.m.keys
	switch {
	case keymap.Matches(press, k.scroll.Up):
		o.cursor = max(o.cursor-1, 0)
	case keymap.Matches(press, k.scroll.Down):
		o.cursor = min(o.cursor+1, len(verdicts)-1)
	case keymap.Matches(press, k.back), keymap.Matches(press, k.cancel):
		o.done = true
	case keymap.Matches(press, k.choose):
		o.done = true
		return o.m.writeSummary(o.cursor)
	}
	return nil
}

// writeSummary replaces the verdicts with the composer of the summary of a
// review with the verdict at row.
func (m *panesModal) writeSummary(row int) tea.Cmd {
	m.pop()
	c := composer.New("review/"+m.repo.String()+"/"+strconv.Itoa(m.number),
		composer.WithTitle("Review #"+strconv.Itoa(m.number)+": "+verdicts[row].label),
		composer.WithPlaceholder("Write the summary of the review in markdown."),
		composer.WithKeyMap(m.keys.composer),
		composer.WithStyles(m.theme.Prompt(m.icons)),
		composer.WithSize(m.width, m.areaHeight()),
		// An approval needs no summary; a submit that needs one is refused
		// with the reason, not with the composer's note.
		composer.WithOptional(),
	)
	o := &composerOverlay{m: m, comp: c, verdict: row, head: "Review #" + strconv.Itoa(m.number), submit: (*panesModal).submitReview}
	m.open(o)
	return tea.Batch(o.comp.Init(), o.comp.Focus())
}

// composerOverlay takes the summary of a review, or the text of a comment. It
// is a viewer in the way it is placed, but it types: it takes every key itself,
// and its own keys close it. head is what its frame says, and submit what its
// submit key does.
type composerOverlay struct {
	m       *panesModal
	comp    composer.Model
	verdict int
	head    string
	submit  func(m *panesModal, o *composerOverlay) tea.Cmd
	done    bool
}

func (o *composerOverlay) title() string           { return o.head }
func (o *composerOverlay) kind() panes.OverlayKind { return panes.Viewer }
func (o *composerOverlay) items() int              { return 0 }
func (o *composerOverlay) text() int               { return 0 }
func (o *composerOverlay) clear() bool             { return false }
func (o *composerOverlay) closed() bool            { return o.done }

// typing reports that the overlay takes the keys of the app's actions as text.
func (o *composerOverlay) typing() bool { return true }

func (o *composerOverlay) layers() []keyhelp.Layer {
	return []keyhelp.Layer{ui.ContextHelp("composer", &o.comp, true)}
}

func (o *composerOverlay) view(w, h int) string {
	o.comp.SetSize(w, h)
	return o.comp.View()
}

func (o *composerOverlay) update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case composer.SubmitMsg:
		if msg.ID == o.comp.ID() {
			return o.submit(o.m, o)
		}
		return nil
	case composer.CancelMsg:
		if msg.ID == o.comp.ID() && !o.m.sending {
			o.done = true
		}
		return nil
	case tea.KeyPressMsg, tea.PasteMsg:
		// GitHub has the review: the summary stays until it answers.
		if o.m.sending {
			return nil
		}
	}
	var cmd tea.Cmd
	o.comp, cmd = o.comp.Update(msg)
	return cmd
}
