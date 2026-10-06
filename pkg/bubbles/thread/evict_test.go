package thread

import (
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"
)

func TestEvictKeepsChunksNearScreen(t *testing.T) {
	tests := []struct {
		name string
		max  int
		want int // most chunks resident at the end
	}{
		{"default", DefaultMaxChunks, DefaultMaxChunks},
		{"three", 3, 3},
		{"one keeps what is on screen", 1, 2},
		{"unlimited", 0, 12},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := loaded(t, newSource(12, 10), nil, 60, 10, WithMaxChunks(tt.max))
			if m.MaxChunks() != tt.max {
				t.Fatalf("MaxChunks() = %d, want %d", m.MaxChunks(), tt.max)
			}
			m = toEnd(t, m)
			got := resident(m)
			if len(got) > tt.want {
				t.Fatalf("resident chunks %v, want at most %d", got, tt.want)
			}
			if !slices.Contains(got, 11) {
				t.Fatalf("resident chunks %v don't include the one on screen", got)
			}
		})
	}
}

func TestEvictedChunkKeepsCursorAndHeight(t *testing.T) {
	m := loaded(t, newSource(12, 10), nil, 60, 10, WithMaxChunks(3))
	m = toEnd(t, m)
	c := m.chunks[0]
	if c.loaded || c.items != nil || c.lines != nil {
		t.Fatal("chunk 0 is still in memory")
	}
	if c.cursor != "" || m.chunks[1].cursor != "1" || c.height == 0 {
		t.Fatalf("evicted chunk lost its cursor %q or height %d", c.cursor, c.height)
	}
}

func TestEvictionKeepsScrollPosition(t *testing.T) {
	m := loaded(t, newSource(12, 10), nil, 60, 10, WithMaxChunks(2))
	for range 400 {
		before := ansi.Strip(m.lines[m.YOffset()])
		total, top := m.TotalLines(), m.YOffset()
		m = press(t, m, "ctrl+d")
		if m.YOffset() == top {
			break
		}
		// A half page down moves by exactly half a page, even when chunks
		// were evicted or loaded behind it.
		if m.YOffset() != top+m.Height()/2 && m.TotalLines() == total && !m.AtBottom() {
			t.Fatalf("scrolled from %d to %d, after %q", top, m.YOffset(), before)
		}
	}
	if !m.done() {
		t.Fatal("didn't reach the end")
	}
}

func TestEvictedChunkFetchedAgainNearScreen(t *testing.T) {
	src := newSource(12, 10)
	r := &renders{}
	m := loaded(t, src, r, 60, 10, WithMaxChunks(3))
	m = toEnd(t, m)
	fetched, rendered := len(src.calls()), r.count()
	total := m.TotalLines()

	m = press(t, m, "g")
	if !strings.Contains(ansi.Strip(m.View()), "Cache drops entries early") {
		t.Fatal("not at the top")
	}
	// Scroll down to the first comments; chunk 0 comes back.
	for !m.chunks[0].loaded {
		m = press(t, m, "ctrl+d")
		if m.YOffset() > m.starts[1] {
			t.Fatal("scrolled past chunk 0 without fetching it")
		}
	}
	if got := src.calls()[fetched:]; !slices.Contains(got, "") {
		t.Fatalf("fetched %v, want chunk 0 again", got)
	}
	if r.count() <= rendered {
		t.Fatal("chunk 0 wasn't rendered again")
	}
	if m.TotalLines() != total {
		t.Fatalf("TotalLines() = %d, want %d: the same chunk came back at another height", m.TotalLines(), total)
	}
	if got := resident(m); len(got) > 3 || !slices.Contains(got, 0) {
		t.Fatalf("resident chunks %v", got)
	}
}

func TestEvictedChunkFailsAndRetries(t *testing.T) {
	src := newSource(12, 10)
	m := loaded(t, src, nil, 60, 10, WithMaxChunks(3))
	m = toEnd(t, m)
	src.fail[""] = 1
	m = press(t, m, "g")
	m = scrollUntil(t, m, func(m Model[comment]) bool { return m.chunks[0].err != nil })
	if all := ansi.Strip(strings.Join(m.lines, "\n")); !m.failed() || !strings.Contains(all, "· r to retry") {
		t.Fatalf("no error line:\n%s", all)
	}
	m = press(t, m, "r")
	if m.failed() || !m.chunks[0].loaded {
		t.Fatal("retry didn't load chunk 0")
	}
}

func TestFailedChunkOutOfSightRetriesOnItsOwn(t *testing.T) {
	src := newSource(12, 10)
	m := loaded(t, src, nil, 60, 10, WithMaxChunks(3))
	m = toEnd(t, m)
	src.fail[""] = 1
	m = press(t, m, "g")
	m = scrollUntil(t, m, func(m Model[comment]) bool { return m.chunks[0].err != nil })
	m = press(t, m, "G")
	if m.failed() {
		t.Fatal("a failed chunk out of sight still counts as failed")
	}
	m = press(t, m, "g")
	_ = scrollUntil(t, m, func(m Model[comment]) bool { return m.chunks[0].loaded })
}

func TestResizeWithEvictedChunks(t *testing.T) {
	r := &renders{}
	m := loaded(t, newSource(12, 10), r, 80, 10, WithMaxChunks(3))
	m = toEnd(t, m)
	// Keep the first comment of the last chunk at the top while the width
	// changes.
	m.vp.SetYOffset(m.starts[11])
	rendered := r.count()
	m.SetSize(40, 10)
	if got, want := r.count()-rendered, 10*len(resident(m)); got != want {
		t.Fatalf("resize rendered %d comments, want %d: only the resident chunks", got, want)
	}
	if top := ansi.Strip(m.lines[m.YOffset()]); !strings.Contains(top, "@user110") {
		t.Fatalf("top line is %q, want @user110", strings.TrimSpace(top))
	}
	// The evicted chunks come back at the new width.
	m = press(t, m, "g")
	m = scrollUntil(t, m, func(m Model[comment]) bool { return m.chunks[0].loaded })
	requireFits(t, m.View(), 40, 10)
}

func TestViewEvictedLoading(t *testing.T) {
	m := loaded(t, newSource(12, 10), nil, 60, 12, WithMaxChunks(3))
	m = toEnd(t, m)
	if m.chunks[0].loaded {
		t.Fatal("chunk 0 wasn't evicted")
	}
	// Scroll onto chunk 0 without an Update, so it isn't fetched yet.
	m.vp.SetYOffset(m.starts[0] - 2)
	out := m.View()
	requireFits(t, out, 60, 12)
	golden.RequireEqual(t, out)
}

// scrollUntil presses ctrl+d from where m is until ok, or fails at the bottom.
func scrollUntil(t *testing.T, m Model[comment], ok func(Model[comment]) bool) Model[comment] {
	t.Helper()
	for !ok(m) {
		if m.AtBottom() {
			t.Fatal("reached the bottom")
		}
		m = press(t, m, "ctrl+d")
	}
	return m
}
