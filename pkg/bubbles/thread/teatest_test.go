package thread

import (
	"bytes"
	"slices"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/exp/teatest/v2"
)

// root hosts a thread as the program root, the way a parent view would. It
// signals on end once every comment is loaded and the bottom is on screen,
// since the renderer only writes changed cells and the output can't show that
// reliably.
type root struct {
	thread Model[comment]
	start  tea.Cmd
	end    chan struct{}
}

func (r root) Init() tea.Cmd { return tea.Batch(r.thread.Init(), r.start) }

func (r root) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if k, ok := msg.(tea.KeyPressMsg); ok && k.String() == "q" {
		return r, tea.Quit
	}
	var cmd tea.Cmd
	r.thread, cmd = r.thread.Update(msg)
	if r.thread.done() && r.thread.AtBottom() {
		select {
		case r.end <- struct{}{}:
		default:
		}
	}
	return r, cmd
}

func (r root) View() tea.View { return tea.NewView(r.thread.View()) }

func TestProgramScrollsAndLoadsComments(t *testing.T) {
	src := newSource(3, 20)
	m := newTest(src, nil, 80, 24)
	start := m.SetDocument(testHeader, testBody)
	end := make(chan struct{}, 1)
	tm := teatest.NewTestModel(t, root{thread: m, start: start, end: end},
		teatest.WithInitialTermSize(80, 24))

	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return bytes.Contains(b, []byte("Cache drops entries early")) && bytes.Contains(b, []byte("@user0"))
	}, teatest.WithDuration(3*time.Second))

	// Each G jumps to the end of what is loaded, which loads the next chunk,
	// so keep pressing it until the end.
	timeout := time.After(5 * time.Second)
	for waiting := true; waiting; {
		tm.Type("G")
		select {
		case <-end:
			waiting = false
		case <-time.After(50 * time.Millisecond):
		case <-timeout:
			t.Fatal("never reached the last comment")
		}
	}
	tm.Type("q")

	final := tm.FinalModel(t, teatest.WithFinalTimeout(3*time.Second)).(root).thread
	if got, want := src.calls(), []string{"", "1", "2"}; !slices.Equal(got, want) {
		t.Fatalf("fetched %v, want %v", got, want)
	}
	if !final.AtBottom() || len(final.chunks) != 3 {
		t.Fatalf("AtBottom() = %v with %d chunks, want the bottom of 3", final.AtBottom(), len(final.chunks))
	}
}
