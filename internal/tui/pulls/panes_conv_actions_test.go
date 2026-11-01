package pulls

import (
	"errors"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// convActions opens the conversation of convService with the viewer as login,
// the thread allowing what edit makes of it, and the cursor on the row of the
// item with ID at.
func convActions(t *testing.T, svc *fakeService, login, at string, edit func(*core.Thread)) (*host, *panesModal) {
	t.Helper()
	page := convThreads
	page.Threads = slices.Clone(convThreads.Threads)
	if edit != nil {
		edit(&page.Threads[0])
	}
	svc.threadPages = map[string]core.ThreadPage{"": page}
	// What the service keeps is what it serves, so a change of the service's
	// cache shows when the conversation reads it again.
	svc.threadCache = svc.threadPages
	svc.timeline.cache = svc.timeline.pages
	h, m := convModal(t, svc)
	m.conv.viewer = login
	// The rows are built once the pane has a size, which drawing it gives.
	m.View()
	if at != "" && !m.conv.model.Reveal(at) {
		t.Fatalf("the conversation has no row for %q", at)
	}
	return h, m
}

// asked returns the question the modal asks, or "".
func asked(m *panesModal) string {
	if len(m.overlays) == 0 {
		return ""
	}
	if o, ok := m.top().(*confirmOverlay); ok {
		return o.c.Question
	}
	return ""
}

// toasts returns the texts of the toasts among msgs.
func toasts(msgs []tea.Msg) []string {
	var out []string
	for _, msg := range msgs {
		if n, ok := msg.(ui.NotifyMsg); ok {
			out = append(out, n.Text)
		}
	}
	return out
}

func canResolve(t *core.Thread)   { t.ViewerCanResolve = true }
func canUnresolve(t *core.Thread) { t.Resolved, t.ViewerCanUnresolve = true, true }

func TestConvCommentOpensTheComposer(t *testing.T) {
	t.Parallel()
	svc := convService()
	_, m := convActions(t, svc, "me", "", nil)
	m.Update(keyMsg("c"))
	o, ok := m.top().(*composerOverlay)
	if !ok {
		t.Fatalf("c opened %T, want the composer", m.top())
	}
	if v := o.comp.Value(); v != "" {
		t.Fatalf("the composer starts with %q, want it empty", v)
	}
	typeText(m, "hello")
	submit(m)
	got := svc.changes()
	if len(got) != 1 || !strings.Contains(got[0], "hello") {
		t.Fatalf("changes = %q, want the comment", got)
	}
}

func TestConvResolveAsksAndSends(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, at, question, op, toast string
		edit                          func(*core.Thread)
	}{
		{"thread", "T1-a", "Resolve thread a/b.go:7?", "resolve thread T1 true", "thread resolved", canResolve},
		{"reply", "T1-b", "Resolve thread a/b.go:7?", "resolve thread T1 true", "thread resolved", canResolve},
		{"reopen", "T1-a", "Reopen thread a/b.go:7?", "resolve thread T1 false", "thread reopened", canUnresolve},
		{"file thread", "T1-a", "Resolve thread a/b.go?", "resolve thread T1 true", "thread resolved", func(th *core.Thread) {
			th.File, th.Line, th.ViewerCanResolve = true, 0, true
		}},
		{"outdated thread", "T1-a", "Resolve thread a/b.go:5?", "resolve thread T1 true", "thread resolved", func(th *core.Thread) {
			th.Line, th.OriginalLine, th.ViewerCanResolve = 0, 5, true
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			svc := convService()
			h, m := convActions(t, svc, "dee", tc.at, tc.edit)
			if msgs := press(t, h, "T"); len(toasts(msgs)) != 0 {
				t.Fatalf("T toasted %v, want a question", toasts(msgs))
			}
			if got := asked(m); got != tc.question {
				t.Fatalf("T asks %q, want %q", got, tc.question)
			}
			if got := svc.changes(); len(got) != 0 {
				t.Fatalf("a change was made before the answer: %v", got)
			}
			msgs := press(t, h, "y")
			if got := svc.changes(); !slices.Equal(got, []string{tc.op}) {
				t.Errorf("changes %v, want %q", got, tc.op)
			}
			if got := toasts(msgs); !slices.Equal(got, []string{tc.toast}) {
				t.Errorf("toasts %v, want %q", got, tc.toast)
			}
		})
	}
}

func TestConvResolveFailureRollsBack(t *testing.T) {
	t.Parallel()
	svc := convService()
	svc.sendErr = errors.New("boom")
	h, m := convActions(t, svc, "dee", "T1-a", canResolve)
	press(t, h, "T")
	msgs := press(t, h, "y")
	if got := toasts(msgs); len(got) != 0 {
		t.Errorf("a failed change toasted %v as done", got)
	}
	var failed bool
	for _, msg := range msgs {
		if d, ok := msg.(ui.DoneMsg); ok && d.Err != nil {
			failed = true
		}
	}
	if !failed {
		t.Errorf("no DoneMsg carried the error: %v", msgs)
	}
	if asked(m) != "" {
		t.Error("the question stays open")
	}
}

func TestConvResolveRefusals(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, at, toast string
		edit            func(*core.Thread)
	}{
		{"can't resolve", "T1-a", "you can't resolve this thread", nil},
		{"can't reopen", "T1-a", "you can't reopen this thread", func(th *core.Thread) { th.Resolved = true }},
		{"not a thread", "C1", "select a thread (or a reply in it) to resolve", canResolve},
		{"a review", "R1", "select a thread (or a reply in it) to resolve", canResolve},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			svc := convService()
			h, m := convActions(t, svc, "dee", tc.at, tc.edit)
			msgs := press(t, h, "T")
			if got := toasts(msgs); !slices.Equal(got, []string{tc.toast}) {
				t.Errorf("toasts %v, want %q", got, tc.toast)
			}
			if asked(m) != "" || len(svc.changes()) != 0 {
				t.Errorf("asked %q and changed %v, want neither", asked(m), svc.changes())
			}
		})
	}
}

func TestConvDelete(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, login, at string
		question, op    string
		toast           string
	}{
		{name: "own comment", login: "ann", at: "C1", question: "Delete your comment?", op: "delete comment C1"},
		{name: "own thread comment with replies", login: "dee", at: "T1-a", question: "Delete your comment? Its replies stay.", op: "delete review comment T1 T1-a"},
		{name: "own reply", login: "bob", at: "T1-b", question: "Delete your reply?", op: "delete review comment T1 T1-b"},
		{name: "someone else's comment", login: "bob", at: "C1", toast: "you can only delete your own comments"},
		{name: "someone else's reply", login: "dee", at: "T1-b", toast: "you can only delete your own comments"},
		{name: "a review", login: "bob", at: "R1", toast: "a submitted review can't be deleted"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			svc := convService()
			h, m := convActions(t, svc, tc.login, tc.at, nil)
			msgs := press(t, h, "D")
			if tc.toast != "" {
				if got := toasts(msgs); !slices.Equal(got, []string{tc.toast}) {
					t.Errorf("toasts %v, want %q", got, tc.toast)
				}
				if asked(m) != "" || len(svc.changes()) != 0 {
					t.Errorf("asked %q and changed %v, want neither", asked(m), svc.changes())
				}
				return
			}
			if got := asked(m); got != tc.question {
				t.Fatalf("D asks %q, want %q", got, tc.question)
			}
			if got := svc.changes(); len(got) != 0 {
				t.Fatalf("a change was made before the answer: %v", got)
			}
			msgs = press(t, h, "y")
			if got := svc.changes(); !slices.Equal(got, []string{tc.op}) {
				t.Errorf("changes %v, want %q", got, tc.op)
			}
			if got := toasts(msgs); !slices.Equal(got, []string{"comment deleted"}) {
				t.Errorf("toasts %v, want comment deleted", got)
			}
		})
	}
}

func TestConvDeleteWithoutReplies(t *testing.T) {
	t.Parallel()
	h, m := convActions(t, convService(), "dee", "T1-a", func(th *core.Thread) {
		th.Comments, th.TotalComments = th.Comments[:1], 1
	})
	press(t, h, "D")
	if got, want := asked(m), "Delete your comment?"; got != want {
		t.Errorf("D asks %q, want %q", got, want)
	}
}

func TestConvDeleteFailureRollsBack(t *testing.T) {
	t.Parallel()
	svc := convService()
	svc.sendErr = errors.New("boom")
	h, _ := convActions(t, svc, "ann", "C1", nil)
	press(t, h, "D")
	msgs := press(t, h, "y")
	if got := toasts(msgs); len(got) != 0 {
		t.Errorf("a failed delete toasted %v as done", got)
	}
	var failed bool
	for _, msg := range msgs {
		if d, ok := msg.(ui.DoneMsg); ok && d.Err != nil {
			failed = true
		}
	}
	if !failed {
		t.Errorf("no DoneMsg carried the error: %v", msgs)
	}
}
