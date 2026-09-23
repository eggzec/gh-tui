package thread

import (
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestReloadFetchesLoadedChunks(t *testing.T) {
	src := newSource(12, 10)
	m := loaded(t, src, nil, 60, 10, WithMaxChunks(3))
	m = toEnd(t, m)
	res := resident(m)
	want := make([]string, 0, len(res))
	for _, i := range res {
		want = append(want, m.chunks[i].cursor)
	}
	before := len(src.calls())
	m = drain(t, m, m.Reload())
	got := src.calls()[before:]
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("reload fetched %v, want the resident chunks %v", got, want)
	}
}

func TestReloadShowsChanges(t *testing.T) {
	tests := []struct {
		name   string
		change func([][]comment) [][]comment
		want   string
	}{
		{
			name: "edited comment",
			change: func(c [][]comment) [][]comment {
				c[0][1].body = "Edited after the fact."
				return c
			},
			want: "Edited after the fact.",
		},
		{
			name: "new comment in the last chunk",
			change: func(c [][]comment) [][]comment {
				c[0] = append(c[0], comment{author: "newcomer", body: "Just posted."})
				return c
			},
			want: "@newcomer",
		},
		{
			name: "new comment on a new page",
			change: func(c [][]comment) [][]comment {
				return append(c, []comment{{author: "newcomer", body: "Just posted."}})
			},
			want: "@newcomer",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := newSource(1, 3)
			m := loaded(t, src, nil, 60, 30)
			src.update(tt.change)
			m = drain(t, m, m.Reload())
			if out := ansi.Strip(m.View()); !strings.Contains(out, tt.want) {
				t.Fatalf("view doesn't show %q:\n%s", tt.want, out)
			}
		})
	}
}

func TestReloadKeepsReadingPosition(t *testing.T) {
	src := newSource(2, 20)
	m := loaded(t, src, nil, 60, 10)
	m = toEnd(t, m)
	m.vp.SetYOffset(m.starts[1] + m.chunks[1].starts[4])
	// Comments above the screen grow.
	src.update(func(c [][]comment) [][]comment {
		for i := range c[0] {
			c[0][i].body = strings.Repeat("Much longer now. ", 12)
		}
		return c
	})
	m = drain(t, m, m.Reload())
	if top := ansi.Strip(m.lines[m.YOffset()]); !strings.Contains(top, "@user24") {
		t.Fatalf("top line is %q, want @user24", strings.TrimSpace(top))
	}
}

func TestReloadFailureKeepsCommentsAndRetries(t *testing.T) {
	src := newSource(1, 3)
	m := loaded(t, src, nil, 60, 30)
	src.fail[""] = 1
	m = drain(t, m, m.Reload())
	out := ansi.Strip(m.View())
	if !m.failed() || !strings.Contains(out, "@user0") || !strings.Contains(out, "Press r to retry") {
		t.Fatalf("want the old comments and an error line:\n%s", out)
	}
	m = press(t, m, "r")
	if m.failed() || strings.Contains(ansi.Strip(m.View()), "retry") {
		t.Fatalf("retry didn't clear the error:\n%s", ansi.Strip(m.View()))
	}
}

func TestReloadRetriesFailedTail(t *testing.T) {
	src := newSource(1, 3)
	src.fail[""] = 1
	m := loaded(t, src, nil, 60, 30)
	if !m.failed() {
		t.Fatal("first fetch didn't fail")
	}
	m = drain(t, m, m.Reload())
	if m.failed() || !strings.Contains(ansi.Strip(m.View()), "@user0") {
		t.Fatal("reload didn't retry the failed chunk")
	}
}

func TestReloadBeforeDocument(t *testing.T) {
	m := newTest(newSource(1, 3), nil, 60, 30)
	if cmd := m.Reload(); cmd != nil {
		t.Fatal("Reload() before SetDocument returned a command")
	}
}

func TestResetStartsOver(t *testing.T) {
	src := newSource(3, 20)
	m := loaded(t, src, nil, 60, 10)
	m = press(t, m, "G")
	// A fetch is in flight when the document changes.
	var inflight tea.Cmd
	m, inflight = m.Update(keyMsg("G"))
	if inflight == nil {
		t.Fatal("no fetch in flight")
	}

	if cmd := m.Reset(); cmd == nil {
		t.Fatal("Reset() = nil, want the spinner tick")
	}
	src.mu.Lock()
	ctx := src.ctxs[len(src.ctxs)-1]
	src.mu.Unlock()
	if ctx.Err() == nil {
		t.Fatal("Reset didn't cancel the fetches in flight")
	}
	if out := ansi.Strip(m.View()); !strings.Contains(out, "Loading…") || m.YOffset() != 0 {
		t.Fatalf("after Reset:\n%s", out)
	}
	// The stale result arrives after the reset and is dropped.
	m = drain(t, m, inflight)
	if len(m.chunks) != 0 {
		t.Fatalf("applied %d stale chunks", len(m.chunks))
	}

	before := len(src.calls())
	m = drain(t, m, m.SetDocument("\x1b[1mAnother issue\x1b[m", "Different body."))
	if got := src.calls()[before:]; !slices.Equal(got, []string{""}) {
		t.Fatalf("fetched %v, want the first chunk", got)
	}
	out := ansi.Strip(m.View())
	if !strings.Contains(out, "Another issue") || !strings.Contains(out, "@user0") || strings.Contains(out, "Cache drops") {
		t.Fatalf("after SetDocument:\n%s", out)
	}
}
