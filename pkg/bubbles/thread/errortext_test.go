package thread

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/key"
	"github.com/charmbracelet/x/ansi"
)

// failedTail returns a thread whose first chunk of comments failed to
// load, with opts.
func failedTail(t *testing.T, width int, opts ...Option) Model[comment] {
	t.Helper()
	src := newSource(1, 3)
	src.fail[""] = 1
	return loaded(t, src, nil, width, 24, opts...)
}

func TestErrorText(t *testing.T) {
	rebound := DefaultKeyMap()
	rebound.Retry = key.NewBinding(key.WithKeys("R"), key.WithHelp("R", "reload"))
	offline := func(error) (string, string) { return "Can't reach GitHub", "r to retry" }
	tests := []struct {
		name string
		opts []Option
		want string
	}{
		{"default", nil, "✗ Couldn't load comments: connection reset · r to retry"},
		{"default names the retry key", []Option{WithKeyMap(rebound)}, "✗ Couldn't load comments: connection reset · R to retry"},
		{"custom", []Option{WithErrorText(offline)}, "✗ Can't reach GitHub · r to retry"},
		{"custom without a hint", []Option{WithErrorText(func(error) (string, string) { return "o/r#5 doesn't exist.", "" })}, "✗ o/r#5 doesn't exist."},
		{"empty names only the retry key", []Option{WithErrorText(func(error) (string, string) { return "", "" })}, "r to retry"},
		{"text with a dot", []Option{WithErrorText(func(error) (string, string) { return "GitHub says a · b", "" })}, "✗ GitHub says a · b"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := failedTail(t, 80, tt.opts...)
			if got := strings.TrimSpace(ansi.Strip(m.status)); got != tt.want {
				t.Errorf("status = %q, want %q", got, tt.want)
			}
			if v := ansi.Strip(m.View()); !strings.Contains(v, tt.want) || strings.HasPrefix(tt.want, "r") && strings.Contains(v, "✗") {
				t.Errorf("View() = %q, want the line %q", v, tt.want)
			}
		})
	}
}

// A chunk that fails to load again after it was dropped says so in its
// place, in the words given.
func TestErrorTextOfEvictedChunk(t *testing.T) {
	say := func(error) (string, string) { return "Can't reach GitHub", "r to retry" }
	src := newSource(12, 10)
	m := toEnd(t, loaded(t, src, nil, 60, 10, WithMaxChunks(3), WithErrorText(say)))
	src.fail[""] = 1
	m = press(t, m, "g")
	m = scrollUntil(t, m, func(m Model[comment]) bool { return m.chunks[0].err != nil })
	if all := ansi.Strip(strings.Join(m.lines, "\n")); !strings.Contains(all, "✗ Can't reach GitHub · r to retry") {
		t.Errorf("lines = %q, want the chunk's error in its place", all)
	}
}

// The hint of a failed fetch stays whole at any width that can hold it,
// and the text gives way to it.
func TestErrorKeepsTheHintWhole(t *testing.T) {
	const hint = "r to retry"
	say := func(error) (string, string) {
		return "GitHub says the token can't read this organization's repositories until SSO allows it", hint
	}
	for width := 1; width <= 120; width++ {
		m := failedTail(t, width, WithErrorText(say))
		requireFits(t, m.View(), width, 24)
		if w := ansi.StringWidth(m.status); w != width {
			t.Fatalf("at %d the status is %d wide", width, w)
		}
		status := ansi.Strip(m.status)
		if width >= len(statusIndent)+len(hint) && !strings.Contains(status, hint) {
			t.Errorf("at %d the hint is cut: %q", width, status)
		}
		if width > 25 && !strings.Contains(status, "✗ GitHub") {
			t.Errorf("at %d the text is lost: %q", width, status)
		}
	}
}

// The words of a failed fetch are asked for once, as it fails, not on
// every render or scroll.
func TestErrorTextWordedOnce(t *testing.T) {
	calls := 0
	m := failedTail(t, 60, WithErrorText(func(error) (string, string) {
		calls++
		return "Can't reach GitHub", "r to retry"
	}))
	before := calls
	for range 3 {
		_ = m.View()
		m = press(t, m, "j")
	}
	m.SetSize(70, 20)
	if before != 1 || calls != before {
		t.Errorf("asked for the words %d times as the fetch failed and %d more after, want once and none", before, calls-before)
	}
}

func TestSetErrorText(t *testing.T) {
	m := failedTail(t, 60)
	m.SetErrorText(func(error) (string, string) { return "Something went wrong", "r to retry" })
	if v := ansi.Strip(m.View()); !strings.Contains(v, "✗ Something went wrong · r to retry") {
		t.Errorf("View() = %q, want the new error text", v)
	}
	// A failure not worth telling, such as a canceled fetch, leaves the
	// comments to read again rather than seeming to load.
	m.SetErrorText(func(error) (string, string) { return "", "" })
	if v := ansi.Strip(m.View()); strings.Contains(v, "✗") || !strings.Contains(v, "r to retry") {
		t.Errorf("View() = %q, want only the retry key", v)
	}
}

// TestRetry checks that Retry fetches again what failed, as the retry key
// does, that Err tells what failed, and that neither costs anything when
// nothing did.
func TestRetry(t *testing.T) {
	src := newSource(1, 3)
	src.fail[""] = 1
	m := loaded(t, src, nil, 80, 24)
	if m.Err() == nil {
		t.Fatal("Err() = nil after the fetch failed")
	}
	m = drain(t, m, m.Retry())
	if m.Err() != nil {
		t.Fatalf("after Retry: Err() = %v", m.Err())
	}
	if got := len(src.calls()); got != 2 {
		t.Errorf("fetched %d times, want 2", got)
	}
	if cmd := m.Retry(); cmd != nil {
		t.Error("Retry with nothing failed returned a command")
	}
}

// A retry key that is turned off is named nowhere, not even beside a
// failure not worth telling.
func TestErrorTextWithoutRetry(t *testing.T) {
	off := DefaultKeyMap()
	off.Retry.SetEnabled(false)
	for _, opts := range [][]Option{
		{WithKeyMap(off)},
		{WithKeyMap(off), WithErrorText(func(error) (string, string) { return "", "" })},
	} {
		m := failedTail(t, 60, opts...)
		if v := ansi.Strip(m.View()); strings.Contains(v, "to retry") {
			t.Errorf("View() = %q, want no retry key", v)
		}
	}
}
