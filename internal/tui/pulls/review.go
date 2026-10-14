package pulls

import (
	"errors"
	"log/slog"
	"slices"
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
	"github.com/eggzec/gh-tui/pkg/bubbles/toast"
)

// The review step of the modal: the user picks a verdict, writes a summary
// and, after a question, every draft of the tray goes to GitHub as one review.

// outdatedPendingText is what the user reads when their pending review on
// GitHub is on an older commit than the head: a comment added to it lands on
// that commit, so a review sent from here would not be one of the head.
const outdatedPendingText = "Your pending review on GitHub is on an older commit than the head; update or discard it on GitHub first."

// pendingAppearedText is what the user reads when a pending review started on
// GitHub after they confirmed: its comments are in the tray now, and the
// drafts are kept.
const pendingAppearedText = "You have a pending review on GitHub; its comments are in the tray now. Submit again to send them with the drafts."

// partialReviewText is what the user reads when a review into their pending
// review failed part way: what was added stays there.
const partialReviewText = "Comments that went in before the failure stay in your pending review on GitHub, and leave the drafts."

// reviewStage is the part of the step on view.
type reviewStage int

const (
	// reviewVerdict picks the verdict.
	reviewVerdict reviewStage = iota
	// reviewSummary writes the summary, and a submit asks the question.
	reviewSummary
	// reviewOutdated tells that some drafts are outdated and are not sent.
	reviewOutdated
)

// verdicts are the choices of the first stage, in the order they show.
var verdicts = [...]struct {
	event       core.ReviewEvent
	label, hint string
}{
	{core.ReviewEventComment, "Comment", "General feedback, without an approval."},
	{core.ReviewEventApprove, "Approve", "Approve the changes."},
	{core.ReviewEventRequestChanges, "Request changes", "Ask for changes before it can merge."},
}

// reviewState is the open review step.
type reviewState struct {
	stage reviewStage
	// verdict is the row of verdicts that has the cursor, and choice the row of
	// the choices that the outdated stage offers.
	verdict, choice int
	// comp holds the summary, from the summary stage on.
	comp composer.Model
	// sending is set while GitHub has the review, when the step takes no key.
	sending bool
	// skipped is how many drafts the outdated stage says are not sent.
	skipped int
}

// reviewedMsg reports that GitHub answered the review sent by the modal.
type reviewedMsg struct {
	owner *detailModal
	err   error
	// sent are the IDs of the drafts that were in the review.
	sent []string
}

// reviewPlan is what a submit would send.
type reviewPlan struct {
	event       core.ReviewEvent
	label, body string
	head        string
	drafts      []tray.Draft
	comments    int
	replies     int
	skipped     int
	sentIDs     []string
	number      int
	// onGitHub counts the comments already in the pending review, which are
	// not sent, and pending is that review, with no ID if there is none.
	onGitHub int
	pending  core.PendingReview
}

// summaryKey is the key of the draft of the summary of a review of this pull
// request.
func (m *detailModal) summaryKey() string {
	return strings.Join([]string{m.draftScope, m.repo.String(), strconv.Itoa(m.number), "review"}, "/")
}

// openReview opens the review step on its verdicts, if the viewer may comment
// and the head is known.
func (m *detailModal) openReview() tea.Cmd {
	if !m.loaded || m.rev != nil || m.comp != nil || m.tray.comp != nil || m.tray.list != nil {
		return nil
	}
	if cmd, refused := m.gate().Refuse(ui.ActComment, &m.detail.Issue); refused {
		return cmd
	}
	if m.detail.HeadSHA == "" {
		return ui.Notify(toast.Info, "The head commit is not known yet; try again once the pull request has loaded.")
	}
	m.rev = &reviewState{}
	// A pending review may have been started on the web since it was last read.
	return m.loadPending()
}

// pressReview takes a key of the step, which takes every key.
func (m *detailModal) pressReview(msg tea.KeyPressMsg) tea.Cmd {
	r, k := m.rev, m.keys
	if r.sending {
		return nil
	}
	if r.stage == reviewSummary {
		var cmd tea.Cmd
		r.comp, cmd = r.comp.Update(msg)
		return cmd
	}
	cursor, n := &r.verdict, len(verdicts)
	if r.stage == reviewOutdated {
		cursor, n = &r.choice, 2
	}
	switch {
	case keymap.Matches(msg, k.overview.Up):
		*cursor = max(*cursor-1, 0)
	case keymap.Matches(msg, k.overview.Down):
		*cursor = min(*cursor+1, n-1)
	case keymap.Matches(msg, k.Back):
		return m.closeReview()
	case keymap.Matches(msg, k.Select) && r.stage == reviewVerdict:
		return m.startSummary()
	case keymap.Matches(msg, k.Select) && r.choice == 0:
		// Without the outdated drafts: the question is of what is left, and a
		// no goes back to the summary.
		r.stage = reviewSummary
		return m.askReview()
	case keymap.Matches(msg, k.Select):
		return m.closeReview()
	}
	return nil
}

// startSummary opens the composer of the summary, once a verdict is picked.
func (m *detailModal) startSummary() tea.Cmd {
	r := m.rev
	opts := []composer.Option{
		composer.WithTitle("Review #" + strconv.Itoa(m.number) + ": " + verdicts[r.verdict].label),
		composer.WithPlaceholder("Write the summary of the review in markdown."),
		composer.WithKeyMap(m.keys.composer),
		composer.WithStyles(m.theme.Prompt(m.icons)),
		composer.WithSize(m.width, m.height),
		// An approval needs no summary; a submit that needs one is refused
		// with the reason, not with the composer's note.
		composer.WithOptional(),
	}
	if m.drafts != nil {
		opts = append(opts, composer.WithDrafts(m.drafts))
	}
	r.stage = reviewSummary
	r.comp = composer.New(m.summaryKey(), opts...)
	if body := m.tray.pending.Body; body != "" && m.drafts != nil {
		// The pending review's summary starts the text, unless the user kept
		// a draft of their own, which is read here since a text in the
		// composer keeps its own draft from being restored.
		if text, err := m.drafts.Load(m.summaryKey()); err != nil || strings.TrimSpace(text) == "" {
			r.comp.SetValue(body)
		}
	} else if body != "" {
		r.comp.SetValue(body)
	}
	return tea.Batch(r.comp.Init(), r.comp.Focus())
}

// closeReview closes the step, keeping the summary as a draft.
func (m *detailModal) closeReview() tea.Cmd {
	err := m.saveSummary()
	m.rev = nil
	m.thread.Focus()
	return m.summaryWarning(err)
}

// saveSummary keeps the text of the open summary as its draft, so that the
// next review starts from it. A blank text is left alone: the draft may not
// have reached the composer yet, and saving the blank would delete it.
func (m *detailModal) saveSummary() error {
	r := m.rev
	if r == nil || r.stage == reviewVerdict || m.drafts == nil || strings.TrimSpace(r.comp.Value()) == "" {
		return nil
	}
	return r.comp.Save()
}

// summaryWarning warns that the draft of the summary could not be saved, if
// err says so.
func (m *detailModal) summaryWarning(err error) tea.Cmd {
	if err == nil {
		return nil
	}
	slog.WarnContext(m.ctx, "review summary draft not saved", "span", "tui", "err", ui.ShortPath(err.Error()))
	return ui.Notify(toast.Warning, "Couldn't save the draft of the review summary.")
}

// updateSummaryComposer takes what the composer of the summary reports, if it
// is its. A submit plans the review, and the step stays open behind the
// question or the refusal.
func (m *detailModal) updateSummaryComposer(msg tea.Msg) (tea.Cmd, bool) {
	r := m.rev
	if r == nil || r.stage == reviewVerdict {
		return nil, false
	}
	switch msg := msg.(type) {
	case composer.CancelMsg:
		if msg.ID != r.comp.ID() {
			return nil, false
		}
		m.rev = nil
		m.thread.Focus()
		return m.summaryWarning(msg.Err), true
	case composer.SubmitMsg:
		if msg.ID != r.comp.ID() {
			return nil, false
		}
		// A second submit, such as a repeated key, arrives while the first
		// one asks, or while the review is sent.
		if m.ask != nil || r.sending {
			return nil, true
		}
		return tea.Batch(m.summaryWarning(msg.Err), m.submitReview()), true
	}
	return nil, false
}

// updateSummary gives the composer of the summary the messages that are its,
// such as the draft it loaded, and not keys, which pressReview gives it.
func (m *detailModal) updateSummary(msg tea.Msg) tea.Cmd {
	r := m.rev
	if r == nil || r.stage == reviewVerdict {
		return nil
	}
	if _, isKey := msg.(tea.KeyPressMsg); isKey {
		return nil
	}
	var cmd tea.Cmd
	r.comp, cmd = r.comp.Update(msg)
	return cmd
}

// pasteReview gives a paste to the summary, unless a question is open, which
// the paste would change.
func (m *detailModal) pasteReview(msg tea.PasteMsg) tea.Cmd {
	r := m.rev
	if r.stage != reviewSummary || m.ask != nil || r.sending {
		return nil
	}
	var cmd tea.Cmd
	r.comp, cmd = r.comp.Update(msg)
	return cmd
}

// draftOutdatedFor reports whether draft d can't go in a review on head: the
// line it was written on changed, or it was written on another commit. A reply
// is to a thread, whichever commit it was written on, and one with no commit,
// as the drafts of an earlier version are, is sent as it is.
func draftOutdatedFor(d tray.Draft, head string) bool {
	if d.ThreadID != "" || d.OnGitHub {
		return false
	}
	return d.Outdated || d.Commit != "" && d.Commit != head
}

// planReview returns what a submit would send, or why it must not: the
// verdict is the one picked, and the drafts those the head left no change to.
func (m *detailModal) planReview(summary string) (p reviewPlan, refusal string) {
	v := verdicts[m.rev.verdict]
	p = reviewPlan{event: v.event, label: v.label, body: strings.TrimSpace(summary), head: m.detail.HeadSHA, number: m.number, pending: m.tray.pending}
	if c := p.pending.Commit; p.pending.ID != "" && c != "" && c != p.head {
		return p, outdatedPendingText
	}
	for i := range m.tray.drafts {
		d := m.tray.drafts[i]
		switch {
		case d.OnGitHub:
			p.onGitHub++
			continue
		case draftOutdatedFor(d, p.head):
			p.skipped++
			continue
		case d.ThreadID != "":
			p.replies++
			// The service sends a draft that says its commit only on the head.
			d.Commit = ""
		case d.Line < 1:
			return p, "A draft on " + d.Path + " has no line; delete it from the drafts list or place it on a line."
		default:
			p.comments++
		}
		p.drafts = append(p.drafts, d)
		p.sentIDs = append(p.sentIDs, d.ID)
	}
	if len(p.drafts) == 0 && p.onGitHub == 0 && p.body == "" && p.event != core.ReviewEventApprove {
		return p, "Nothing to send: GitHub refuses a review that only " + strings.ToLower(v.label) + "s with no summary and no comments."
	}
	return p, ""
}

// question asks to submit the plan.
func (p reviewPlan) question() string {
	q := "Submit review (" + p.label + ")"
	var with []string
	if p.comments > 0 {
		with = append(with, counted(p.comments, "comment", "comments"))
	}
	if p.replies > 0 {
		with = append(with, counted(p.replies, "reply", "replies"))
	}
	if p.onGitHub > 0 {
		with = append(with, strconv.Itoa(p.onGitHub)+" already on GitHub")
	}
	if len(with) > 0 {
		q += " with " + strings.Join(with, " and ")
	}
	return q + " on #" + strconv.Itoa(p.number) + "?"
}

// counted returns n with its noun.
func counted(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return strconv.Itoa(n) + " " + many
}

// submitReview plans the review from the summary, and asks about it, after
// telling of the drafts that it leaves out.
func (m *detailModal) submitReview() tea.Cmd {
	p, why := m.planReview(m.rev.comp.Value())
	if why != "" {
		return ui.Notify(toast.Info, why)
	}
	if p.skipped > 0 {
		m.rev.stage, m.rev.choice, m.rev.skipped = reviewOutdated, 0, p.skipped
		return nil
	}
	return m.askReview()
}

// askReview asks the user to confirm the review, on the last line. By the time
// they say yes, the drafts or the head may have changed, so the plan is made
// again, and sent only if it asks the same.
func (m *detailModal) askReview() tea.Cmd {
	p, why := m.planReview(m.rev.comp.Value())
	if why != "" {
		return ui.Notify(toast.Info, why)
	}
	ask := ui.Recheck(p.question(), "the review", func() (ui.Confirm, bool, tea.Cmd) {
		if m.rev == nil {
			return ui.Confirm{}, false, nil
		}
		p, why := m.planReview(m.rev.comp.Value())
		if why != "" {
			return ui.Confirm{}, false, ui.Notify(toast.Info, why)
		}
		if cmd, refused := m.gate().Refuse(ui.ActComment, &m.detail.Issue); refused {
			return ui.Confirm{}, false, cmd
		}
		return ui.Confirm{Question: p.question(), Run: func() tea.Cmd { return m.sendReview(p) }}, true, nil
	})
	m.ask = &ask
	return nil
}

// sendReview sends the review. The step stays open, with the drafts and the
// summary, until GitHub answers.
func (m *detailModal) sendReview(p reviewPlan) tea.Cmd {
	m.rev.sending = true
	svc, ctx, repo, n := m.svc, m.sendCtx, m.repo, m.number
	return func() tea.Msg {
		var err error
		if p.pending.ID != "" {
			err = svc.SubmitIntoPending(ctx, repo, n, p.head, p.pending, p.event, p.body, p.drafts)
		} else {
			err = svc.SubmitReview(ctx, repo, n, p.head, p.event, p.body, p.drafts)
		}
		return reviewedMsg{owner: m, err: err, sent: p.sentIDs}
	}
}

// reviewed settles the step once GitHub answered. A refusal leaves the drafts
// and the summary as they were, and says why. A review that went through
// takes the drafts in it out of the tray, those it left out stay, and the
// summary draft goes; what the review changed is read again.
func (m *detailModal) reviewed(msg reviewedMsg) tea.Cmd {
	if msg.owner != m {
		return nil
	}
	if m.rev != nil {
		m.rev.sending = false
	}
	num := "#" + strconv.Itoa(m.number)
	if msg.err != nil {
		slog.WarnContext(m.ctx, "review not submitted", "span", "tui", "err", ui.ShortPath(msg.err.Error()))
		switch {
		case errors.Is(msg.err, pulls.ErrPendingAppeared):
			// The drafts stay, and the comments of the review that started are
			// among them once it is read.
			return tea.Batch(m.loadPending(), ui.Notify(toast.Info, pendingAppearedText))
		case errors.Is(msg.err, pulls.ErrPendingReviewOutdated):
			return ui.Notify(toast.Error, outdatedPendingText)
		case m.tray.pending.ID != "":
			// Steps before the failing one went through: what is in the pending
			// review now is read again, and those drafts are dropped for it.
			m.tray.reconcile = msg.sent
			return tea.Batch(ui.Fail("submit the review of "+num, msg.err), ui.Notify(toast.Warning, partialReviewText), m.loadPending())
		}
		return ui.Fail("submit the review of "+num, msg.err)
	}
	slog.InfoContext(m.ctx, "review submitted", "span", "tui", "drafts", len(msg.sent))
	m.tray.drafts = slices.DeleteFunc(m.tray.drafts, func(d tray.Draft) bool { return slices.Contains(msg.sent, d.ID) })
	// The pending review went with it, comments and all.
	m.dropPending()
	m.tray.replacing, m.tray.from = "", nil
	if m.rev != nil && m.drafts != nil {
		if err := m.drafts.Delete(m.rev.comp.Key()); err != nil {
			slog.WarnContext(m.ctx, "review summary draft not removed", "span", "tui", "err", ui.ShortPath(err.Error()))
		}
	}
	m.rev = nil
	m.thread.Focus()
	repo := m.repo
	return tea.Batch(
		m.saveTray(),
		m.showThreadChange(),
		ui.Notify(toast.Success, "Review submitted on "+num+"."),
		func() tea.Msg { return changedMsg{repo: repo} },
		m.get(), m.thread.Reload(), m.reloadThreads(),
	)
}

// reviewView draws the step in the modal's size.
func (m *detailModal) reviewView() string {
	r := m.rev
	w, h := m.width, m.height
	if r.stage == reviewSummary {
		r.comp.SetSize(w, h)
		return r.comp.View()
	}
	title := " " + m.theme.Accent.Bold(true).Render("Review #"+strconv.Itoa(m.number))
	lines := []string{ansi.Truncate(title, w, ""), ""}
	var rows [][2]string
	cursor := r.verdict
	if r.stage == reviewOutdated {
		cursor = r.choice
		for _, l := range ui.Wrap(counted(r.skipped, "draft is", "drafts are")+" outdated: the lines they are on changed since they were written, so they are not sent.", max(w-2, 1)) {
			lines = append(lines, " "+m.theme.Warning.Render(l))
		}
		lines = append(lines, "")
		listKey := m.keys.Tray.Help().Key
		if listKey != "" {
			listKey = " (" + listKey + ")"
		}
		rows = [][2]string{
			{"Submit without them", "They stay in the tray."},
			{"Cancel", "Then open the drafts list" + listKey + " to place them on a new line or delete them."},
		}
	} else {
		for _, v := range verdicts {
			rows = append(rows, [2]string{v.label, v.hint})
		}
	}
	for i, row := range rows {
		mark := "  "
		if i == cursor {
			mark = m.theme.Accent.Render(m.icons.Dot) + " "
		}
		lines = append(lines, ansi.Truncate(" "+mark+m.st.title.Render(row[0]), w, m.icons.Ellipsis),
			ansi.Truncate("     "+m.st.age.Render(row[1]), w, m.icons.Ellipsis))
	}
	if r.sending {
		lines = append(lines, "", " "+m.st.age.Render("Submitting the review"+m.icons.Ellipsis))
	}
	return strings.Join(ui.FitLines(lines, w, h), "\n")
}

// reviewLayer is the keys of the open step, for help.
func (m *detailModal) reviewLayer() keyhelp.Layer {
	r, k := m.rev, m.keys
	if r.stage == reviewSummary {
		return ui.ContextHelp("composer", &r.comp, true)
	}
	choose := renamed(k.Select, "choose")
	keys := []key.Binding{k.overview.Up, k.overview.Down, choose, k.Back}
	return ui.ContextLayer(ctxModal, keys, []key.Binding{choose, k.Back})
}
