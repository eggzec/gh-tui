package pulls

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/pulls"
	"github.com/eggzec/gh-tui/internal/tray"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/keymap"
)

// reviewCall is a review that the fake service was asked to submit.
type reviewCall struct {
	number int
	head   string
	event  core.ReviewEvent
	body   string
	drafts []tray.Draft
	// pendingID is the pending review the drafts were added to, or empty for a
	// review of their own.
	pendingID string
}

// SubmitReview records the review, and fails with submitErr if it is set.
func (f *fakeService) SubmitReview(_ context.Context, _ core.RepoRef, number int, head string, event core.ReviewEvent, body string, drafts []tray.Draft) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.submitted = append(f.submitted, reviewCall{number, head, event, body, slices.Clone(drafts), ""})
	return f.submitErr
}

// SubmitIntoPending records the review with the ID of the pending one, and
// fails with submitErr if it is set.
func (f *fakeService) SubmitIntoPending(_ context.Context, _ core.RepoRef, number int, head string, pending core.PendingReview, event core.ReviewEvent, body string, drafts []tray.Draft) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.submitted = append(f.submitted, reviewCall{number, head, event, body, slices.Clone(drafts), pending.ID})
	return f.submitErr
}

// PendingReview serves the pending review that the test set.
func (f *fakeService) PendingReview(context.Context, core.RepoRef, int) (core.PendingReview, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.pending, f.pendingFound, nil
}

// setPending sets the pending review that the fake serves.
func (f *fakeService) setPending(p core.PendingReview) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pending, f.pendingFound = p, p.ID != ""
}

func (f *fakeService) reviewCalls() []reviewCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.submitted)
}

// The drafts of the review tests, on the head a1b2c3d of #142.
var (
	reviewLine   = tray.Draft{ID: "line", Commit: "a1b2c3d", Path: "cmd/gh-tui/main.go", Side: tray.Right, Line: 10, Body: "why cfg"}
	reviewRange  = tray.Draft{ID: "range", Commit: "a1b2c3d", Path: "cmd/gh-tui/main.go", Side: tray.Left, StartLine: 11, Line: 12, Body: "drop both"}
	reviewReply  = tray.Draft{ID: "reply", Commit: "a1b2c3d", Path: "cmd/gh-tui/main.go", Side: tray.Right, Line: 10, ThreadID: "T1", Body: "agreed"}
	reviewLegacy = tray.Draft{ID: "legacy", Path: "internal/cache/disk.go", Side: tray.Right, Line: 45, Body: "no commit"}
	reviewOld    = tray.Draft{ID: "old", Commit: "0ld0ld0", Outdated: true, Path: "cmd/gh-tui/main.go", Side: tray.Right, Line: 3, Body: "moved away"}
)

// reviewing returns the modal of #142 with the drafts in the store and its
// service, ready for :review.
func reviewing(t *testing.T, store *memTray, d *memDrafts) (*host, *detailModal, *fakeService) {
	t.Helper()
	svc := newFakeService()
	svc.pulls[0].HeadSHA = "a1b2c3d"
	svc.changed = sampleFiles()
	h := started(t, svc, 116, 40, WithChecks(&fakeChecks{}), WithIcons(ui.NewIcons(config.IconsUnicode)), WithTray(store), WithDrafts(d, "host/me"))
	h.SetTheme(theme(true))
	press(t, h, "enter")
	m := h.modal()
	if m == nil {
		t.Fatal("enter opened no pull request")
	}
	drain(t, h, m.switchTo(filesTab))
	return h, m, svc
}

// runReview presses the review action.
func runReview(t *testing.T, h *host) {
	t.Helper()
	drain(t, h, h.Update(keymap.ActionPress("pull_modal.review")))
}

// toSummary opens the step, picks the verdict that is down rows below the
// first and types the summary.
func toSummary(t *testing.T, h *host, down int, summary string) {
	t.Helper()
	runReview(t, h)
	for range down {
		press(t, h, "down")
	}
	press(t, h, "enter")
	if summary != "" {
		typeText(t, h, summary)
	}
}

// submitSummary submits the summary with its key.
func submitSummary(t *testing.T, h *host) []tea.Msg {
	t.Helper()
	return drain(t, h, h.Update(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl}))
}

// toasts returns the text of the toasts in msgs.
func toasts(msgs []tea.Msg) []string {
	var out []string
	for _, msg := range msgs {
		if n, ok := msg.(ui.NotifyMsg); ok {
			out = append(out, n.Text)
		}
	}
	return out
}

func TestReviewCommandOpensTheVerdicts(t *testing.T) {
	t.Parallel()
	h, m, _ := reviewing(t, kept(reviewLine), &memDrafts{})
	if m.rev != nil {
		t.Fatal("the step is open before the command")
	}
	runReview(t, h)
	if m.rev == nil || m.rev.stage != reviewVerdict {
		t.Fatalf("the command left the step %+v, want it on the verdicts", m.rev)
	}
	v := modalText(h)
	for _, want := range []string{"Review #142", "Comment", "Approve", "Request changes"} {
		if !strings.Contains(v, want) {
			t.Errorf("the verdicts lack %q:\n%s", want, v)
		}
	}
	press(t, h, "esc")
	if m.rev != nil || h.modal() != m {
		t.Errorf("esc left the step %+v, or closed the modal", m.rev)
	}
}

func TestReviewVerdictChoice(t *testing.T) {
	t.Parallel()
	h, m, _ := reviewing(t, kept(reviewLine), &memDrafts{})
	runReview(t, h)
	press(t, h, "down")
	press(t, h, "down")
	press(t, h, "down")
	if m.rev.verdict != len(verdicts)-1 {
		t.Fatalf("the cursor is on %d after three downs, want the last verdict", m.rev.verdict)
	}
	press(t, h, "enter")
	if m.rev.stage != reviewSummary {
		t.Fatalf("enter left the stage %d, want the summary", m.rev.stage)
	}
	if v := modalText(h); !strings.Contains(v, "Review #142: Request changes") {
		t.Errorf("the summary doesn't name the verdict:\n%s", v)
	}
}

func TestReviewAsksWithTheCounts(t *testing.T) {
	t.Parallel()
	h, m, svc := reviewing(t, kept(reviewLine, reviewRange, reviewReply, reviewLegacy), &memDrafts{})
	toSummary(t, h, 1, "")
	submitSummary(t, h)
	want := "Submit review (Approve) with 3 comments and 1 reply on #142?"
	if m.ask == nil || m.ask.Question != want {
		t.Fatalf("submit asked %+v, want %q", m.ask, want)
	}
	if got := svc.reviewCalls(); len(got) != 0 {
		t.Fatalf("submit sent %v before the answer", got)
	}
	press(t, h, "n")
	if m.ask != nil || m.rev == nil || m.rev.stage != reviewSummary {
		t.Errorf("n left the question %+v and the step %+v, want the summary", m.ask, m.rev)
	}
	if got := svc.reviewCalls(); len(got) != 0 {
		t.Errorf("n sent %v", got)
	}
}

func TestReviewYesSendsAndClearsTheTray(t *testing.T) {
	t.Parallel()
	store := kept(reviewLine, reviewRange, reviewReply, reviewLegacy)
	d := &memDrafts{}
	h, m, svc := reviewing(t, store, d)
	toSummary(t, h, 2, "Pleasefix")
	submitSummary(t, h)
	msgs := press(t, h, "y")

	calls := svc.reviewCalls()
	if len(calls) != 1 {
		t.Fatalf("sent %d reviews, want one", len(calls))
	}
	c := calls[0]
	if c.number != 142 || c.head != "a1b2c3d" || c.event != core.ReviewEventRequestChanges || c.body != "Pleasefix" {
		t.Errorf("review = %+v, want request changes on a1b2c3d with the summary", c)
	}
	ids := make([]string, 0, len(c.drafts))
	for _, d := range c.drafts {
		ids = append(ids, d.ID)
	}
	if !slices.Equal(ids, []string{"line", "range", "reply", "legacy"}) {
		t.Errorf("sent drafts %v, want all four", ids)
	}
	if r := c.drafts[1]; r.StartLine != 11 || r.Line != 12 || r.Side != tray.Left {
		t.Errorf("the range went as %+v, want lines 11-12 on the left", r)
	}
	if c.drafts[2].ThreadID != "T1" {
		t.Errorf("the reply went as %+v, want it on thread T1", c.drafts[2])
	}
	if m.tray.drafts != nil || len(store.drafts()) != 0 {
		t.Errorf("the tray holds %v and the store %v, want both empty", m.tray.drafts, store.drafts())
	}
	if m.rev != nil {
		t.Error("the step is still open after the review went")
	}
	if text, deleted := d.get(m.summaryKey()); text != "" || len(deleted) == 0 {
		t.Errorf("the summary draft is %q, deleted %v, want it deleted", text, deleted)
	}
	if got := toasts(msgs); !slices.ContainsFunc(got, func(s string) bool { return strings.Contains(s, "Review submitted on #142") }) {
		t.Errorf("toasts %v, want one that says the review was submitted", got)
	}
}

func TestReviewFailureKeepsDraftsAndText(t *testing.T) {
	t.Parallel()
	store := kept(reviewLine, reviewReply)
	h, m, svc := reviewing(t, store, &memDrafts{})
	svc.submitErr = errors.New("boom")
	toSummary(t, h, 0, "Mytext")
	submitSummary(t, h)
	msgs := press(t, h, "y")

	if len(svc.reviewCalls()) != 1 {
		t.Fatal("the review was not tried")
	}
	if len(store.drafts()) != 2 || len(m.tray.drafts) != 2 {
		t.Errorf("the tray holds %d and the store %d drafts, want both kept at 2", len(m.tray.drafts), len(store.drafts()))
	}
	if m.rev == nil || m.rev.sending || m.rev.comp.Value() != "Mytext" {
		t.Errorf("the step is %+v, want it open on the summary with its text", m.rev)
	}
	failed := slices.ContainsFunc(msgs, func(msg tea.Msg) bool { f, ok := msg.(ui.FailMsg); return ok && f.Err != nil })
	if !failed {
		t.Errorf("no failure was reported: %v", msgs)
	}
}

// webPending is a pending review started on the web, on the head of #142,
// with a comment on a line of the first file and a summary.
var webPending = core.PendingReview{
	ID: "PRR1", Commit: "a1b2c3d", Body: "web summary",
	Comments: []core.PendingComment{{ID: "C1", Path: "cmd/gh-tui/main.go", Line: 20, Body: "from the web"}},
}

func TestReviewPendingAppearedMergesAndKeepsDrafts(t *testing.T) {
	t.Parallel()
	h, m, svc := reviewing(t, kept(reviewLine), &memDrafts{})
	// The review is started on the web after the tray last read GitHub.
	svc.submitErr = fmt.Errorf("submit review: %w: %w", pulls.ErrPendingAppeared, pulls.ErrPendingReview)
	toSummary(t, h, 1, "")
	submitSummary(t, h)
	svc.setPending(webPending)
	msgs := press(t, h, "y")
	if got := toasts(msgs); !slices.Contains(got, pendingAppearedText) {
		t.Errorf("toasts %v, want %q", got, pendingAppearedText)
	}
	if m.tray.pending.ID != "PRR1" || len(m.tray.drafts) != 2 {
		t.Errorf("pending %q and %d drafts, want the review merged beside the kept draft", m.tray.pending.ID, len(m.tray.drafts))
	}
	if byID(t, m.tray.drafts, "line").OnGitHub || !byID(t, m.tray.drafts, "C1").OnGitHub {
		t.Errorf("drafts %+v, want only the comment of the review on GitHub", m.tray.drafts)
	}
}

func TestPendingCommentsShowAsRowsAndAreNotSaved(t *testing.T) {
	t.Parallel()
	store := kept(reviewLine)
	h, m, svc := reviewing(t, store, &memDrafts{})
	svc.setPending(webPending)
	runReview(t, h)
	if !byID(t, m.tray.drafts, "C1").OnGitHub {
		t.Fatalf("drafts %+v, want the comment of the pending review among them", m.tray.drafts)
	}
	if err := m.writeTray(); err != nil {
		t.Fatal(err)
	}
	if got := store.drafts(); len(got) != 1 || got[0].ID != "line" {
		t.Errorf("the store holds %+v, want only the local draft", got)
	}
	if blocks := m.draftBlocks(); len(blocks) != 2 {
		t.Errorf("the diff has %d draft blocks, want 2", len(blocks))
	}
	press(t, h, "esc")
	press(t, h, "t")
	if v := modalText(h); !strings.Contains(v, "from the web") || !strings.Contains(v, draftOnGitHub) {
		t.Errorf("the list lacks the comment on GitHub:\n%s", v)
	}
}

func TestPendingCommentsAreReadOnly(t *testing.T) {
	t.Parallel()
	h, m, svc := reviewing(t, kept(), &memDrafts{})
	svc.setPending(webPending)
	runReview(t, h)
	press(t, h, "esc")
	press(t, h, "t")
	for _, k := range []string{"D", "c"} {
		msgs := press(t, h, k)
		if got := toasts(msgs); !slices.Contains(got, onGitHubText) {
			t.Errorf("%s toasts %v, want %q", k, got, onGitHubText)
		}
	}
	if len(m.tray.drafts) != 1 {
		t.Errorf("the tray holds %d drafts, want the comment kept", len(m.tray.drafts))
	}
}

func TestReviewIntoPendingQuestionsAndSends(t *testing.T) {
	t.Parallel()
	d := &memDrafts{}
	h, m, svc := reviewing(t, kept(reviewLine, reviewReply), d)
	svc.setPending(webPending)
	toSummary(t, h, 1, "")
	if got := m.rev.comp.Value(); got != "web summary" {
		t.Errorf("the summary starts as %q, want the pending review's", got)
	}
	submitSummary(t, h)
	want := "Submit review (Approve) with 1 comment and 1 reply and 1 already on GitHub on #142?"
	if m.ask == nil || m.ask.Question != want {
		t.Fatalf("submit asked %+v, want %q", m.ask, want)
	}
	press(t, h, "y")
	calls := svc.reviewCalls()
	if len(calls) != 1 || calls[0].pendingID != "PRR1" || len(calls[0].drafts) != 2 {
		t.Fatalf("calls %+v, want one into PRR1 with the two local drafts", calls)
	}
	if m.tray.drafts != nil || m.tray.pending.ID != "" {
		t.Errorf("the tray holds %v and pending %q, want both cleared", m.tray.drafts, m.tray.pending.ID)
	}
}

func TestReviewRefusesPendingOnAnOlderCommit(t *testing.T) {
	t.Parallel()
	h, m, svc := reviewing(t, kept(reviewLine), &memDrafts{})
	old := webPending
	old.Commit = "0ld0ld0"
	svc.setPending(old)
	toSummary(t, h, 1, "")
	msgs := submitSummary(t, h)
	if got := toasts(msgs); !slices.Contains(got, outdatedPendingText) {
		t.Errorf("toasts %v, want %q", got, outdatedPendingText)
	}
	if m.ask != nil || len(svc.reviewCalls()) != 0 {
		t.Errorf("the review went on: ask %+v, calls %v", m.ask, svc.reviewCalls())
	}
}

func TestReviewFailureIntoPendingDropsWhatWentIn(t *testing.T) {
	t.Parallel()
	h, m, svc := reviewing(t, kept(reviewLine, reviewRange), &memDrafts{})
	svc.setPending(webPending)
	toSummary(t, h, 1, "")
	submitSummary(t, h)
	// GitHub took the first comment, then failed.
	added := webPending
	added.Comments = append(slices.Clone(webPending.Comments), core.PendingComment{ID: "C2", Path: reviewLine.Path, Line: reviewLine.Line, Body: reviewLine.Body})
	svc.submitErr = errors.New("boom")
	svc.setPending(added)
	press(t, h, "y")
	if len(m.tray.drafts) != 3 {
		t.Fatalf("the tray holds %+v, want the range and two comments on GitHub", m.tray.drafts)
	}
	if !byID(t, m.tray.drafts, "C2").OnGitHub || slices.ContainsFunc(m.tray.drafts, func(d tray.Draft) bool { return d.ID == "line" }) {
		t.Errorf("drafts %+v, want the added draft to show as on GitHub only", m.tray.drafts)
	}
}

func TestPendingThreadsAreNotThreads(t *testing.T) {
	t.Parallel()
	pub := core.Thread{ID: "T1", Comments: []core.ThreadComment{{ID: "a"}, {ID: "b", ReplyTo: "a", Pending: true}}, TotalComments: 2}
	web := core.Thread{ID: "T2", Pending: true, Comments: []core.ThreadComment{{ID: "c", Pending: true}}}
	shown, hidden := splitPending([]core.Thread{pub, web})
	if len(shown) != 1 || shown[0].ID != "T1" || len(shown[0].Comments) != 1 || shown[0].TotalComments != 1 {
		t.Errorf("shown %+v, want T1 without its pending reply", shown)
	}
	if len(hidden) != 2 || hidden[0].ID != "T1" || hidden[1].ID != "T2" {
		t.Errorf("hidden %+v, want the pending part of T1 and T2", hidden)
	}
}

func TestReviewLeavesOutOutdatedDrafts(t *testing.T) {
	t.Parallel()
	store := kept(reviewLine, reviewOld)
	h, m, svc := reviewing(t, store, &memDrafts{})
	toSummary(t, h, 1, "")
	submitSummary(t, h)
	if m.rev.stage != reviewOutdated || m.ask != nil {
		t.Fatalf("submit left the stage %d and the question %+v, want the outdated notice", m.rev.stage, m.ask)
	}
	v := modalText(h)
	for _, want := range []string{"1 draft is outdated", "Submit without them", "Cancel"} {
		if !strings.Contains(v, want) {
			t.Errorf("the notice lacks %q:\n%s", want, v)
		}
	}

	// Cancel closes the step with nothing sent, and keeps the summary.
	press(t, h, "down")
	press(t, h, "enter")
	if m.rev != nil || len(svc.reviewCalls()) != 0 || len(store.drafts()) != 2 {
		t.Fatalf("cancel left the step %+v, %d reviews and %d drafts", m.rev, len(svc.reviewCalls()), len(store.drafts()))
	}

	// Without them: the question counts only the rest, and the outdated one stays.
	toSummary(t, h, 1, "")
	submitSummary(t, h)
	press(t, h, "enter")
	if want := "Submit review (Approve) with 1 comment on #142?"; m.ask == nil || m.ask.Question != want {
		t.Fatalf("asked %+v, want %q", m.ask, want)
	}
	press(t, h, "y")
	calls := svc.reviewCalls()
	if len(calls) != 1 || len(calls[0].drafts) != 1 || calls[0].drafts[0].ID != "line" {
		t.Fatalf("sent %+v, want the one draft that is not outdated", calls)
	}
	if got := store.drafts(); len(got) != 1 || got[0].ID != "old" {
		t.Errorf("the store holds %+v, want only the outdated draft", got)
	}
}

func TestReviewRefusesWhatGitHubWould(t *testing.T) {
	t.Parallel()
	// No drafts and no summary: a comment is refused, an approval is not.
	h, m, _ := reviewing(t, kept(), &memDrafts{})
	toSummary(t, h, 0, "")
	msgs := submitSummary(t, h)
	if m.ask != nil || len(toasts(msgs)) != 1 || !strings.Contains(toasts(msgs)[0], "Nothing to send") {
		t.Fatalf("an empty comment asked %+v with toasts %v, want a refusal", m.ask, toasts(msgs))
	}
	press(t, h, "esc")
	toSummary(t, h, 1, "")
	submitSummary(t, h)
	if m.ask == nil || m.ask.Question != "Submit review (Approve) on #142?" {
		t.Errorf("an empty approval asked %+v, want the question", m.ask)
	}
	press(t, h, "n")
	press(t, h, "esc")

	// A draft on a thread without a line is refused.
	h, m, svc := reviewing(t, kept(tray.Draft{ID: "bad", Commit: "a1b2c3d", Path: "a.go", Body: "x"}), &memDrafts{})
	toSummary(t, h, 1, "")
	msgs = submitSummary(t, h)
	if m.ask != nil || len(toasts(msgs)) != 1 || !strings.Contains(toasts(msgs)[0], "has no line") {
		t.Errorf("a draft without a line asked %+v with toasts %v, want a refusal", m.ask, toasts(msgs))
	}
	if len(svc.reviewCalls()) != 0 {
		t.Error("a refused review was sent")
	}
}

func TestReviewSavesTheSummaryOnQuit(t *testing.T) {
	t.Parallel()
	d := &memDrafts{}
	h, m, svc := reviewing(t, kept(reviewLine), d)
	toSummary(t, h, 1, "Halfwritten")
	if err := m.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if text, _ := d.get(m.summaryKey()); text != "Halfwritten" {
		t.Errorf("the summary draft is %q, want the open text", text)
	}
	if m.ask != nil || len(svc.reviewCalls()) != 0 {
		t.Error("saving asked or sent something")
	}
	if !strings.HasSuffix(m.summaryKey(), "/gh-tui/142/review") && !strings.Contains(m.summaryKey(), "host/me/eggzec/gh-tui/142/review") {
		t.Errorf("the key is %q, want scope/repo/number/review", m.summaryKey())
	}
}
