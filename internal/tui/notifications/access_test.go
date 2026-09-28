package notifications

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/internal/tui/ui/uitest"
	"github.com/eggzec/gh-tui/pkg/bubbles/toast"
)

// tokenSection returns a section whose token is what tok knows, in a
// dark or light theme, read from svc, which refuses to list while the
// token may not, as the service does.
func tokenSection(tb testing.TB, svc *fakeService, tok *uitest.Checker, dark bool, width, height int) *Section {
	tb.Helper()
	v := ui.NewVoice(config.Default().Keys, "")
	v.Token = uitest.Token(tok)
	svc.listErr = tok.Check(core.NeedNotifications)
	s := New(tb.Context(), svc, config.Default().Keys, WithNow(func() time.Time { return now }), WithVoice(v))
	p, err := config.Default().Palette(dark)
	if err != nil {
		tb.Fatal(err)
	}
	s.SetTheme(ui.NewTheme(p, dark))
	s.SetSize(width, height)
	s.Focus()
	run(tb, s, s.Init())
	return s
}

func TestViewUnreadable(t *testing.T) {
	tests := []struct {
		name   string
		access core.Access
		width  int
	}{
		{"no scope", uitest.Classic("gist", "read:org"), 80},
		{"no scope at 50 columns", uitest.Classic("gist", "read:org"), 50},
		{"fine-grained", core.Access{Kind: core.TokenFineGrained}, 80},
	}
	for _, tt := range tests {
		for _, dark := range []bool{true, false} {
			name := tt.name + "/light"
			if dark {
				name = tt.name + "/dark"
			}
			t.Run(name, func(t *testing.T) {
				s := tokenSection(t, newFake(inbox()...), &uitest.Checker{A: tt.access}, dark, tt.width, 6)
				view := s.View()
				for i, l := range strings.Split(view, "\n") {
					if w := ansi.StringWidth(l); w != tt.width {
						t.Errorf("line %d is %d cells wide, want %d", i, w, tt.width)
					}
				}
				if strings.Contains(ansi.Strip(view), errMark) {
					t.Errorf("the empty state is marked as an error:\n%s", ansi.Strip(view))
				}
				golden.RequireEqual(t, view)
			})
		}
	}
}

func TestUnreadableWords(t *testing.T) {
	tests := []struct {
		access core.Access
		want   string
	}{
		{uitest.Classic("gist"), "The token lacks the notifications scope · :auth to grant it"},
		{core.Access{Kind: core.TokenFineGrained}, "This needs a classic token, not a fine-grained one · :auth to see how"},
		{core.Access{Kind: core.TokenApp}, "This needs a classic token, not a GitHub App's · :auth to see how"},
	}
	for _, tt := range tests {
		s := tokenSection(t, newFake(inbox()...), &uitest.Checker{A: tt.access}, true, 120, 6)
		if got := ansi.Strip(s.View()); !strings.Contains(got, tt.want) {
			t.Errorf("%v shows\n%s\nwant %q", tt.access.Kind, got, tt.want)
		}
	}
}

func TestMarksNeedTheToken(t *testing.T) {
	tok := &uitest.Checker{A: core.Access{Kind: core.TokenFineGrained}}
	svc := newFake(inbox()...)
	s := tokenSection(t, svc, tok, true, 120, 10)
	for _, desc := range uitest.Enabled(s.KeyLayers()) {
		if strings.HasPrefix(desc, "read") || strings.HasPrefix(desc, "done") || strings.Contains(desc, "all read") {
			t.Errorf("help offers %q while the token may not mark", desc)
		}
	}
	why := "Marking notifications needs a classic token, not a fine-grained one · :auth to see how"
	for _, k := range []string{"m", "d", "M"} {
		msgs := press(t, s, k)
		if question(s) != "" || !slices.Contains(msgs, any(ui.NotifyMsg{Level: toast.Info, Text: why})) {
			t.Errorf("%s asked %q and showed %v, want the toast %q", k, question(s), msgs, why)
		}
	}
	if len(svc.reads) != 0 || len(svc.dones) != 0 || svc.allRead != 0 {
		t.Errorf("marks sent: %v %v %d, want none", svc.reads, svc.dones, svc.allRead)
	}
}

func TestAccessReadsWhatWasRefused(t *testing.T) {
	tok := &uitest.Checker{A: uitest.Classic("gist")}
	svc := newFake(inbox()...)
	s := tokenSection(t, svc, tok, true, 120, 10)
	// The user granted the scope.
	tok.A = uitest.Classic("gist", "notifications")
	svc.listErr = nil
	before := svc.listCount()
	run(t, s, s.Update(ui.AccessMsg{Access: tok.A}))
	if svc.listCount() == before {
		t.Fatal("the section didn't read the notifications again")
	}
	if got := ansi.Strip(s.View()); strings.Contains(got, "lacks") || len(rows(s)) == 0 {
		t.Errorf("after the grant it shows\n%s\nwant the threads", got)
	}
	// Once read, another change of the token reads nothing again.
	before = svc.listCount()
	run(t, s, s.Update(ui.AccessMsg{Access: uitest.Classic("notifications", "repo")}))
	if got := svc.listCount(); got != before {
		t.Errorf("reads after an unrelated change = %d, want none", got-before)
	}
}

func TestAccessStillRefusedReadsNothing(t *testing.T) {
	tok := &uitest.Checker{A: uitest.Classic("gist")}
	svc := newFake(inbox()...)
	s := tokenSection(t, svc, tok, true, 120, 10)
	tok.A = uitest.Classic("gist", "read:org")
	run(t, s, s.Update(ui.AccessMsg{Access: tok.A}))
	if got := svc.listCount(); got != 0 {
		t.Errorf("the section sent %d requests while the token still may not read notifications", got)
	}
	if got := ansi.Strip(s.View()); !strings.Contains(got, "lacks the notifications scope") {
		t.Errorf("it shows\n%s\nwant why it is empty", got)
	}
}
