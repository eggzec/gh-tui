package pulls

import (
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/panes"
)

const convHead = "abc123"

var (
	convAt       = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	convTimeline = []core.TimelineItem{
		{Kind: core.TimelineReview, ID: "R1", Actor: core.User{Login: "bob"}, At: convAt, Review: core.ReviewStateCommented},
		{Kind: core.TimelineComment, ID: "C1", Actor: core.User{Login: "ann"}, At: convAt, Body: "Looks good."},
	}
	convThreads = core.ThreadPage{Total: 1, Threads: []core.Thread{{
		ID: "T1", ReviewID: "R1", Path: "a/b.go", Line: 7, TotalComments: 2,
		Comments: []core.ThreadComment{
			{ID: "T1-a", Author: core.User{Login: "dee"}, At: convAt, Body: "Why?"},
			{ID: "T1-b", Author: core.User{Login: "bob"}, At: convAt, Body: "Because."},
		},
	}}}
)

// convService serves details that have a head, and a timeline and a thread.
func convService() *fakeService {
	svc := newFakeService()
	svc.tweaks = map[int]func(*core.PullRequestDetail){}
	for i := range svc.pulls {
		svc.tweaks[svc.pulls[i].Number] = func(d *core.PullRequestDetail) { d.HeadSHA = convHead }
	}
	svc.timeline.pages = map[string]core.Page[core.TimelineItem]{"": {Items: convTimeline}}
	svc.threadPages = map[string]core.ThreadPage{"": convThreads}
	return svc
}

// convModal opens the first pull request over svc, 160 columns wide, with the
// Conversation focused, and runs what opening it starts.
func convModal(t *testing.T, svc *fakeService) (*host, *panesModal) {
	t.Helper()
	h := started(t, svc, 160, 30)
	press(t, h, "enter")
	m := h.modal()
	if m == nil {
		t.Fatal("enter didn't open the modal")
	}
	return h, m
}

func TestConvPaneLoadsAfterDetail(t *testing.T) {
	t.Parallel()
	svc := convService()
	svc.getErr = errors.New("boom")
	_, m := convModal(t, svc)
	if m.conv.started || len(svc.timelineReads()) != 0 || len(svc.threadReadsSoFar()) != 0 {
		t.Errorf("without a detail, started is %v after %d timeline and %d thread reads, want none",
			m.conv.started, len(svc.timelineReads()), len(svc.threadReadsSoFar()))
	}

	svc = convService()
	_, m = convModal(t, svc)
	tl, th := svc.timelineReads(), svc.threadReadsSoFar()
	if len(tl) != 1 || len(th) != 1 {
		t.Fatalf("with a detail, %d timeline and %d thread reads, want one each", len(tl), len(th))
	}
	if tl[0].Number != m.number || !tl[0].Repo.Same(m.repo) {
		t.Errorf("timeline read %+v, want %v #%d", tl[0], m.repo, m.number)
	}
	if th[0].Head != convHead || th[0].Number != m.number {
		t.Errorf("threads read %+v, want head %q of #%d", th[0], convHead, m.number)
	}
}

func TestConvPaneCachedFirst(t *testing.T) {
	t.Parallel()
	svc := convService()
	svc.getErr = errors.New("boom")
	svc.timeline.cache = svc.timeline.pages
	svc.threadCache = svc.threadPages
	h, m := convModal(t, svc)
	if strings.Contains(modalScreen(t, h), "Looks good.") {
		t.Fatal("the conversation shows before the pull request has a head")
	}

	// The detail arrives, and the timeline read it starts doesn't return: the
	// cached pages, which cost no I/O, are what the pane shows.
	svc.timeline.gate = make(chan struct{})
	defer close(svc.timeline.gate)
	batch, ok := m.receive(panesDetailMsg{id: m.id, detail: svc.detail(m.number)})().(tea.BatchMsg)
	if !ok {
		t.Fatal("the detail started no batch of reads")
	}
	out := make(chan tea.Msg, 16)
	var run func(c tea.Cmd)
	run = func(c tea.Cmd) {
		if c == nil {
			return
		}
		go func() {
			// The reads of the conversation and of the commits come as batches.
			msg := c()
			if b, ok := msg.(tea.BatchMsg); ok {
				for _, c := range b {
					run(c)
				}
				return
			}
			out <- msg
		}()
	}
	for _, c := range batch {
		run(c)
	}
	deadline := time.After(300 * time.Millisecond)
	for done := false; !done; {
		select {
		case msg := <-out:
			m.Update(msg)
		case <-deadline:
			done = true
		}
	}
	if got := modalScreen(t, h); !strings.Contains(got, "Looks good.") {
		t.Errorf("the cached conversation isn't shown before a read returns:\n%s", got)
	}
}

func TestConvPaneKeysMoveCursor(t *testing.T) {
	t.Parallel()
	_, m := convModal(t, convService())
	if m.focus != panes.Conversation {
		t.Fatalf("the modal focuses %v, want Conversation", m.focus)
	}
	before, v := ansi.Strip(m.View()), m.conv.version
	m.Update(keyMsg("down"))
	if after := ansi.Strip(m.View()); after == before || m.conv.version == v {
		t.Errorf("down didn't change the frame (version %d, was %d)", m.conv.version, v)
	}

	m.focus = panes.Description
	m.syncConvFocus()
	v = m.conv.version
	m.Update(keyMsg("down"))
	if m.conv.version != v {
		t.Error("down reached the conversation with the Description focused")
	}
}

func TestConvPaneModalKeysWin(t *testing.T) {
	t.Parallel()
	_, m := convModal(t, convService())

	m.press(keyMsg("3"))
	if m.focus != panes.Merge {
		t.Errorf("3 focused %v, want Merge", m.focus)
	}
	m.focus = panes.Conversation
	m.press(keyMsg("tab"))
	if m.focus == panes.Conversation {
		t.Error("tab stayed on the conversation")
	}
	m.focus = panes.Conversation
	m.press(keyMsg("z"))
	if !m.zoom.Zoomed() {
		t.Error("z didn't zoom the conversation")
	}
}

func TestConvPaneInfo(t *testing.T) {
	t.Parallel()
	_, m := convModal(t, convService())
	n, total := m.conv.model.Shown()
	if total == 0 {
		t.Fatal("the conversation has no items")
	}
	info := m.convInfo()
	want := strconv.Itoa(n) + " of " + strconv.Itoa(total) + " shown" + m.icons.Separator + "filter: all"
	if len(info) == 0 || info[0] != want {
		t.Errorf("info is %q, want %q first", info, want)
	}
}

func TestConvPaneOnlineReload(t *testing.T) {
	t.Parallel()
	svc := convService()
	svc.timeline.pages = map[string]core.Page[core.TimelineItem]{"": {Items: convTimeline, Offline: true}}
	h, m := convModal(t, svc)
	if n := len(svc.timelineReads()); n != 1 {
		t.Fatalf("%d timeline reads, want 1", n)
	}
	if !m.conv.model.Status().Offline {
		t.Fatal("a page served offline left the conversation online")
	}

	drain(t, h, h.Update(ui.OnlineMsg{}))
	if n := len(svc.timelineReads()); n != 2 {
		t.Errorf("%d timeline reads after coming online, want 2", n)
	}
}

// TestConvPaneKeepsPagesWhileOverlayOpen checks that the pages that arrive
// while a question is open reach the conversation, which shows them once the
// question is answered.
func TestConvPaneKeepsPagesWhileOverlayOpen(t *testing.T) {
	t.Parallel()
	svc := convService()
	svc.getErr = errors.New("boom")
	h, m := convModal(t, svc)
	m.ask(ui.Confirm{Question: "Close #1?", Run: func() tea.Cmd { return nil }})
	if m.top() == nil {
		t.Fatal("the question didn't open")
	}

	drain(t, h, m.receive(panesDetailMsg{id: m.id, detail: svc.detail(m.number)}))
	m.pop()
	if got := modalScreen(t, h); !strings.Contains(got, "Looks good.") {
		t.Errorf("the pages that came while the question was open are lost:\n%s", got)
	}
}

// TestConvPaneFilterFromConfig checks that ui.conversation.filter sets what
// the pane shows when it opens, from the option the app gives and from the
// settings that :set changes.
func TestConvPaneFilterFromConfig(t *testing.T) {
	t.Parallel()
	cfg := config.Default()
	cfg.UI.Conversation.Filter = []string{"reviews"}
	for _, tc := range []struct {
		name string
		open func(t *testing.T) *host
	}{
		{"option", func(t *testing.T) *host {
			t.Helper()
			return started(t, convService(), 160, 30, WithConversation(cfg))
		}},
		{"settings", func(t *testing.T) *host {
			t.Helper()
			h := started(t, convService(), 160, 30)
			h.Update(ui.SettingsMsg{Config: cfg})
			return h
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := tc.open(t)
			press(t, h, "enter")
			m := h.modal()
			if m == nil {
				t.Fatal("enter didn't open the modal")
			}
			n, total := m.conv.model.Shown()
			if n == 0 || n >= total {
				t.Errorf("the filter shows %d of %d, want some but not all", n, total)
			}
			if name := m.conv.model.FilterName(); name == "" || name == "all" {
				t.Errorf("the filter is named %q, want the configured one", name)
			}
			if info := m.convInfo(); len(info) == 0 || strings.Contains(info[0], "filter: all") {
				t.Errorf("info is %q, want the configured filter", info)
			}
		})
	}
}

func TestConvPaneNewHeadRebuilds(t *testing.T) {
	t.Parallel()
	svc := convService()
	h, m := convModal(t, svc)
	d := svc.detail(m.number)

	drain(t, h, m.receive(panesDetailMsg{id: m.id, detail: d}))
	if n := len(svc.threadReadsSoFar()); n != 1 {
		t.Errorf("the same head read the threads again: %d reads, want 1", n)
	}

	d.HeadSHA = "def456"
	drain(t, h, m.receive(panesDetailMsg{id: m.id, detail: d}))
	reads := svc.threadReadsSoFar()
	if len(reads) != 2 || reads[1].Head != "def456" {
		t.Errorf("threads reads after a push are %+v, want a second one at def456", reads)
	}
	if m.conv.head != "def456" {
		t.Errorf("the conversation is at %q, want def456", m.conv.head)
	}
}
