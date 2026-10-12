package pulls

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// resolvable returns a review thread that the viewer may resolve.
func resolvable(id, path string, line int) core.Thread {
	th := review(id, path, line, "hubot", "Swallowing err hides a full disk.")
	th.ViewerCanResolve = true
	return th
}

// resolving returns the modal of #142 on its Conversation tab, whose pull
// request has the threads ths, and says it has that many unresolved.
func resolving(t *testing.T, svc *fakeService, ths ...core.Thread) (*host, *detailModal) {
	t.Helper()
	svc.threadPages = map[string]core.ThreadPage{"": {Threads: ths, Total: len(ths)}}
	svc.summaries[142] = core.ThreadSummary{Total: len(ths), Unresolved: len(ths)}
	return threaded(t, svc, 116, 40, config.IconsUnicode)
}

func TestResolveAsksThenSends(t *testing.T) {
	t.Parallel()
	svc := newFakeService()
	h, m := resolving(t, svc, resolvable("t1", "a.go", 1), resolvable("t2", "b.go", 2))
	if m.detail.Threads.Unresolved != 2 {
		t.Fatalf("the detail counts %d unresolved threads, want 2", m.detail.Threads.Unresolved)
	}
	press(t, h, "T")
	if m.ask == nil || m.ask.Question != "Resolve this thread?" {
		t.Fatalf("T asked %+v, want the question to resolve", m.ask)
	}
	if got := svc.changes(); len(got) != 0 {
		t.Fatalf("T alone sent %v, want nothing before the answer", got)
	}
	press(t, h, "n")
	if m.ask != nil || len(svc.changes()) != 0 {
		t.Errorf("n left the question %+v and the changes %v, want neither", m.ask, svc.changes())
	}

	press(t, h, "T")
	cmd := h.Update(keyMsg("y"))
	if !m.th.pending["t1"] {
		t.Error("the thread is not marked as under way while the request is in flight")
	}
	if got := ansi.Strip(m.View()); !strings.Contains(got, m.icons.Ellipsis+" a.go") {
		t.Errorf("the thread under way shows no mark:\n%s", got)
	}
	drain(t, h, cmd)
	if got := svc.sent(); !slices.Equal(got, []string{"resolve t1"}) {
		t.Errorf("sent %v, want the resolve of t1", got)
	}
	if m.th.pending["t1"] {
		t.Error("the thread is still marked as under way after the answer")
	}
	if len(m.th.list) != 1 || m.th.list[0].ID != "t2" {
		t.Errorf("the unresolved threads are %v, want only t2", m.th.list)
	}
	if m.detail.Threads.Unresolved != 1 {
		t.Errorf("the overview counts %d unresolved threads, want 1", m.detail.Threads.Unresolved)
	}
}

func TestResolveRefused(t *testing.T) {
	t.Parallel()
	svc := newFakeService()
	h, m := resolving(t, svc, resolvable("t1", "a.go", 1), resolvable("t2", "b.go", 2))
	svc.sendErr = errors.New("boom")
	press(t, h, "T")
	var failed bool
	for _, msg := range press(t, h, "y") {
		if _, ok := msg.(ui.FailMsg); ok {
			failed = true
		}
	}
	if !failed {
		t.Error("a refused change sent no failure to show")
	}
	if len(m.th.list) != 2 || m.th.list[0].Resolved || m.th.pending["t1"] {
		t.Errorf("the threads are %v (under way %v), want both unresolved and none under way", m.th.list, m.th.pending)
	}
	if m.detail.Threads.Unresolved != 2 {
		t.Errorf("the overview counts %d unresolved threads, want the cache rolled back to 2", m.detail.Threads.Unresolved)
	}
}

func TestResolveNeedsTheViewersRight(t *testing.T) {
	t.Parallel()
	svc := newFakeService()
	locked := review("t1", "a.go", 1, "hubot", "Swallowing err.")
	h, m := resolving(t, svc, locked)
	msgs := press(t, h, "T")
	if m.ask != nil {
		t.Errorf("T asked %+v of a thread the viewer may not resolve", m.ask)
	}
	var said bool
	for _, msg := range msgs {
		if n, ok := msg.(ui.NotifyMsg); ok && strings.Contains(n.Text, "can't resolve") {
			said = true
		}
	}
	if !said {
		t.Errorf("T on a thread the viewer may not resolve told nothing: %v", msgs)
	}
}

func TestResolveInTheDiffFoldsAndUnfolds(t *testing.T) {
	t.Parallel()
	svc := newFakeService()
	open := resolvable("t1", "internal/cache/disk.go", 45)
	svc.threadPages = map[string]core.ThreadPage{"": {Threads: []core.Thread{open}, Total: 1}}
	svc.summaries[142] = core.ThreadSummary{Total: 1, Unresolved: 1}
	h, m := threaded(t, svc, 116, 60, config.IconsUnicode)
	drain(t, h, m.switchTo(filesTab))
	m.files.diff.SeekBlock("t1")
	press(t, h, "enter")
	if m.files.diff.BlockFolded("t1") {
		t.Fatal("enter left the thread folded")
	}
	press(t, h, "T")
	if m.ask == nil || m.ask.Question != "Resolve this thread?" {
		t.Fatalf("T on the thread asked %+v, want the question to resolve", m.ask)
	}
	press(t, h, "y")
	if !m.files.diff.BlockFolded("t1") {
		t.Error("resolving the thread left it unfolded")
	}
	if got := m.files.counts["internal/cache/disk.go"]; got.open != 0 {
		t.Errorf("the file counts %d open threads, want 0", got.open)
	}

	m.files.diff.SeekBlock("t1")
	press(t, h, "T")
	if m.ask == nil || m.ask.Question != "Unresolve this thread?" {
		t.Fatalf("T on a resolved thread asked %+v, want the question to unresolve", m.ask)
	}
	press(t, h, "y")
	if m.files.diff.BlockFolded("t1") {
		t.Error("unresolving the thread left it folded")
	}
	if got := svc.sent(); !slices.Equal(got, []string{"resolve t1", "unresolve t1"}) {
		t.Errorf("sent %v, want the resolve and the unresolve of t1", got)
	}
}
