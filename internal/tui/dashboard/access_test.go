package dashboard

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/internal/tui/ui/uitest"
)

// tokenVoice returns the voice of the default keys with the token tok
// knows of.
func tokenVoice(tok *uitest.Checker) ui.Voice {
	v := ui.NewVoice(config.Default().Keys, "")
	v.Token = uitest.Token(tok)
	return v
}

func TestViewInboxUnreadable(t *testing.T) {
	for _, dark := range []bool{false, true} {
		t.Run(fmt.Sprintf("dark=%t", dark), func(t *testing.T) {
			tok := &uitest.Checker{A: uitest.Classic("gist", "read:org")}
			in := &fakeInbox{threads: inboxThreads()}
			s := newSection(t, newFake(), in, 80, 22, WithIcons(ui.NewIcons(config.IconsUnicode)), WithVoice(tokenVoice(tok)))
			p, err := config.Default().Palette(dark)
			if err != nil {
				t.Fatal(err)
			}
			s.SetTheme(ui.NewTheme(p, dark))
			press(t, s, "5")
			if in.lists != 0 {
				t.Errorf("the inbox was read %d times, want none while the token may not", in.lists)
			}
			view := s.View()
			if got := ansi.Strip(view); !strings.Contains(got, "The token lacks the notifications scope") {
				t.Errorf("the dashboard shows\n%s\nwithout saying why the inbox is empty", got)
			}
			golden.RequireEqual(t, view)
		})
	}
}

func TestAccessReadsTheInbox(t *testing.T) {
	tok := &uitest.Checker{A: core.Access{Kind: core.TokenFineGrained}}
	in := &fakeInbox{threads: inboxThreads()}
	s := newSection(t, newFake(), in, 140, 38, WithVoice(tokenVoice(tok)))
	if got := ansi.Strip(s.View()); !strings.Contains(got, "This needs a classic token") {
		t.Errorf("the dashboard shows\n%s\nwithout saying why the inbox is empty", got)
	}
	tok.A = uitest.Classic("repo")
	run(t, s, s.Update(ui.AccessMsg{Access: tok.A}))
	if in.lists != 1 {
		t.Fatalf("the inbox was read %d times after the token may, want once", in.lists)
	}
	if got := ansi.Strip(s.View()); !strings.Contains(got, "unread") {
		t.Errorf("the dashboard shows\n%s\nwithout the unread threads", got)
	}
	// Read, the inbox isn't read again for another change.
	run(t, s, s.Update(ui.AccessMsg{Access: uitest.Classic("repo", "workflow")}))
	if in.lists != 1 {
		t.Errorf("the inbox was read %d times, want once", in.lists)
	}
}

func TestAccessStillRefusedReadsNoInbox(t *testing.T) {
	tok := &uitest.Checker{A: uitest.Classic("gist")}
	in := &fakeInbox{threads: inboxThreads()}
	s := newSection(t, newFake(), in, 140, 38, WithVoice(tokenVoice(tok)))
	tok.A = uitest.Classic("gist", "read:org")
	run(t, s, s.Update(ui.AccessMsg{Access: tok.A}))
	if in.lists != 0 {
		t.Errorf("the inbox was read %d times while the token still may not", in.lists)
	}
}
