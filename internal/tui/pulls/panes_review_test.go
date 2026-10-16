package pulls

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/composer"
	"github.com/eggzec/gh-tui/pkg/bubbles/keymap"
)

// find runs cmd, and the commands of a batch, and returns the first message
// of type T.
func find[T tea.Msg](cmd tea.Cmd) (T, bool) {
	var zero T
	if cmd == nil {
		return zero, false
	}
	switch msg := cmd().(type) {
	case tea.BatchMsg:
		for _, c := range msg {
			if v, ok := find[T](c); ok {
				return v, true
			}
		}
	case T:
		return msg, true
	}
	return zero, false
}

// reviewPanes returns a modal whose viewer may comment, over a loaded pull
// request with a head.
func reviewPanes(t *testing.T) (*panesModal, *fakeService) {
	t.Helper()
	m := sizedPanes(t, 160, 40)
	m.caps = writeCaps
	m.loaded = true
	m.detail.HeadSHA = "abc123"
	return m, m.svc.(*fakeService)
}

func startReview(m *panesModal) {
	m.Update(keymap.ActionPress("pull_modal.review"))
}

// typeText types text into the composer on top.
func typeText(m *panesModal, text string) {
	for _, r := range text {
		m.Update(keyMsg(string(r)))
	}
}

// submit presses the submit key and delivers the composer's answer.
func submit(m *panesModal) tea.Cmd {
	cmd := m.Update(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
	if msg, ok := find[composer.SubmitMsg](cmd); ok {
		return m.Update(msg)
	}
	return nil
}

func TestPlanReview(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		verdict int
		summary string
		refused string
		ask     string
	}{
		{"comment needs a summary", 0, "  ", "Nothing to send: GitHub refuses a review that only comments with no summary and no comments.", ""},
		{"request changes needs a summary", 2, "", "Nothing to send: GitHub refuses a review that only requests changes with no summary and no comments.", ""},
		{"approve needs none", 1, "", "", "Submit review (Approve) on #7?"},
		{"comment with a summary", 0, " ok ", "", "Submit review (Comment) on #7?"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p, why := planReview(tt.verdict, tt.summary, "abc", 7)
			if why != tt.refused {
				t.Fatalf("refusal = %q, want %q", why, tt.refused)
			}
			if why == "" && p.question() != tt.ask {
				t.Errorf("question = %q, want %q", p.question(), tt.ask)
			}
			if p.body != strings.TrimSpace(tt.summary) {
				t.Errorf("body = %q", p.body)
			}
		})
	}
}

func TestReviewVerdictsClose(t *testing.T) {
	t.Parallel()
	for _, k := range []string{"esc", "backspace"} {
		// Backspace is the back intent, which the modal takes from the app.
		m, _ := reviewPanes(t)
		startReview(m)
		if _, ok := m.top().(*verdictOverlay); !ok {
			t.Fatalf("the review command opened %T", m.top())
		}
		m.Update(keyMsg("down"))
		m.Update(keyMsg("up"))
		m.Update(keyMsg(k))
		if m.top() != nil || m.closed {
			t.Errorf("%s: open %v, modal closed %v; want only the verdicts closed", k, m.top() != nil, m.closed)
		}
	}
}

func TestReviewSendsTheVerdictAndTheSummary(t *testing.T) {
	t.Parallel()
	m, svc := reviewPanes(t)
	startReview(m)
	m.Update(keyMsg("down"))
	m.Update(keyMsg("enter"))
	c, ok := m.top().(*composerOverlay)
	if !ok || c.verdict != 1 {
		t.Fatalf("choosing opened %T, want the composer of the second verdict", m.top())
	}
	if _, handled := m.Act(ui.ActDismiss); handled {
		t.Fatal("dismiss was taken from the composer, which has its own cancel key")
	}
	typeText(m, "ok")
	submit(m)
	if _, ok := m.top().(*confirmOverlay); !ok {
		t.Fatalf("submit left %T on top, want the question", m.top())
	}
	if len(svc.sends()) != 0 {
		t.Fatal("the review was sent before the answer")
	}
	cmd := m.Update(keyMsg("y"))
	msg, ok := find[reviewedMsg](cmd)
	if !ok {
		t.Fatal("yes sent nothing")
	}
	if !m.sending {
		t.Error("not sending while GitHub has the review")
	}
	if got := svc.sends(); len(got) != 1 || got[0].event != core.ReviewEventApprove || got[0].body != "ok" || got[0].head != "abc123" || got[0].number != m.number {
		t.Fatalf("sent %+v", got)
	}
	cmd = m.Update(msg)
	if m.sending || m.top() != nil {
		t.Errorf("after the answer: sending %v, open %v", m.sending, m.top() != nil)
	}
	if n, ok := find[ui.NotifyMsg](cmd); !ok || !strings.Contains(n.Text, "Review submitted") {
		t.Errorf("notice %v, %v", n, ok)
	}
}

func TestReviewAskedAgainAfterNo(t *testing.T) {
	t.Parallel()
	m, svc := reviewPanes(t)
	startReview(m)
	m.Update(keyMsg("enter"))
	typeText(m, "x")
	submit(m)
	m.Update(keyMsg("n"))
	c, ok := m.top().(*composerOverlay)
	if !ok || c.comp.Value() != "x" {
		t.Fatalf("a no left %T, want the composer with its text", m.top())
	}
	if len(svc.sends()) != 0 {
		t.Error("a no sent the review")
	}
}

func TestReviewWithoutSummaryIsRefused(t *testing.T) {
	t.Parallel()
	m, _ := reviewPanes(t)
	startReview(m)
	m.Update(keyMsg("enter"))
	cmd := submit(m)
	if n, ok := find[ui.NotifyMsg](cmd); !ok || !strings.HasPrefix(n.Text, "Nothing to send") {
		t.Errorf("notice %v, %v", n, ok)
	}
	if _, ok := m.top().(*composerOverlay); !ok {
		t.Errorf("a refusal left %T on top, want the composer", m.top())
	}
}

func TestReviewFailureKeepsTheSummary(t *testing.T) {
	t.Parallel()
	m, svc := reviewPanes(t)
	svc.reviewErr = errors.New("boom")
	startReview(m)
	m.Update(keyMsg("enter"))
	typeText(m, "x")
	submit(m)
	msg, ok := find[reviewedMsg](m.Update(keyMsg("y")))
	if !ok {
		t.Fatal("yes sent nothing")
	}
	cmd := m.Update(msg)
	if _, ok := find[ui.FailMsg](cmd); !ok {
		t.Error("no failure was shown")
	}
	c, ok := m.top().(*composerOverlay)
	if !ok || c.comp.Value() != "x" || m.sending {
		t.Errorf("after a failure: top %T, sending %v", m.top(), m.sending)
	}
}

func TestReviewComposerCancels(t *testing.T) {
	t.Parallel()
	m, _ := reviewPanes(t)
	startReview(m)
	m.Update(keyMsg("enter"))
	msg, ok := find[composer.CancelMsg](m.Update(keyMsg("esc")))
	if !ok {
		t.Fatal("esc didn't cancel the composer")
	}
	m.Update(msg)
	if m.top() != nil || m.closed {
		t.Errorf("open %v, modal closed %v", m.top() != nil, m.closed)
	}
}

func TestReviewNeedsTheRightAndTheHead(t *testing.T) {
	t.Parallel()
	m, _ := reviewPanes(t)
	m.caps = writeCaps
	m.caps.Archived = true
	n, ok := find[ui.NotifyMsg](m.Update(keymap.ActionPress("pull_modal.review")))
	if !ok || !strings.Contains(n.Text, "archived") || m.top() != nil {
		t.Errorf("archived: notice %q, open %v", n.Text, m.top() != nil)
	}
	m.caps = writeCaps
	m.detail.HeadSHA = ""
	n, ok = find[ui.NotifyMsg](m.Update(keymap.ActionPress("pull_modal.review")))
	if !ok || !strings.Contains(n.Text, "head commit") || m.top() != nil {
		t.Errorf("no head: notice %q, open %v", n.Text, m.top() != nil)
	}
}
