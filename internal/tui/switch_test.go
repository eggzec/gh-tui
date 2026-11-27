package tui

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/target"
)

// switchApp is an app signed in as me on github.com whose switcher fails
// with err and records the targets it was asked about.
func switchApp(t *testing.T, err error) (*Model, *[]target.Target) {
	t.Helper()
	var asked []target.Target
	m, _ := newTestApp(t, WithLogin("me"), WithHost("github.com"),
		WithSwitcher(func(_ context.Context, tg target.Target) (target.Target, error) {
			asked = append(asked, tg)
			// gh stores logins in the case the account was made with.
			tg.User = strings.ToUpper(tg.User[:1]) + tg.User[1:]
			return tg, err
		}))
	return m, &asked
}

func TestSwitchBadSyntaxToastsUsage(t *testing.T) {
	t.Parallel()
	m, asked := switchApp(t, nil)
	runCommand(t, m, "switch nonsense")
	if !hasToast(m, "usage") || len(*asked) != 0 {
		t.Fatalf("toast = %q, asked %v", toasted(m), *asked)
	}
}

func TestSwitchSameIdentity(t *testing.T) {
	t.Parallel()
	m, asked := switchApp(t, nil)
	runCommand(t, m, "switch me@github.com")
	if !hasToast(m, "Already signed in as me on github.com") || len(*asked) != 0 {
		t.Fatalf("toast = %q, asked %v", toasted(m), *asked)
	}
}

func TestSwitchErrorToastsAndStays(t *testing.T) {
	t.Parallel()
	m, _ := switchApp(t, errors.New("no token for you on github.com"))
	runCommand(t, m, "switch user you")
	if !hasToast(m, "no token for you on github.com") {
		t.Fatalf("toast = %q", toasted(m))
	}
	if _, ok := m.Restart(); ok {
		t.Fatal("an unusable account restarted the app")
	}
}

func TestSwitchConfirmAccept(t *testing.T) {
	t.Parallel()
	m, asked := switchApp(t, nil)
	runCommand(t, m, "switch user you")
	if len(*asked) != 1 || (*asked)[0] != (target.Target{User: "you", Host: "github.com"}) {
		t.Fatalf("asked %v", *asked)
	}
	if _, ok := m.Restart(); ok {
		t.Fatal("restarted before the answer")
	}
	drive(m, m.key(press("y")))
	got, ok := m.Restart()
	if !ok || got != (target.Target{User: "You", Host: "github.com"}) {
		t.Fatalf("Restart = %v, %v", got, ok)
	}
}

// TestSwitchLogged checks that a switch logs its request, and why a refusal
// was one.
func TestSwitchLogged(t *testing.T) {
	// Not parallel: it replaces the default logger, which the whole process shares.
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })

	m, _ := switchApp(t, errors.New("gh is not logged in to github.com"))
	runCommand(t, m, "switch user you")
	out := buf.String()
	for _, want := range []string{`"msg":"switch requested"`, `"msg":"switch refused"`, `"level":"WARN"`, `gh is not logged in`} {
		if !strings.Contains(out, want) {
			t.Errorf("log lacks %s:\n%s", want, out)
		}
	}
}

// answerSwitch delivers the answer of the check that the last :switch
// started, as it would arrive once the check ended.
func answerSwitch(m *Model, seq int, tg target.Target) {
	drive(m, func() tea.Msg { return switchCheckedMsg{seq: seq, target: tg} })
}

func TestSwitchLateAnswerAfterKeyIsDropped(t *testing.T) {
	t.Parallel()
	m, _ := switchApp(t, nil)
	tg := target.Target{User: "you", Host: "github.com"}
	m.switchCommand("user you")
	seq := m.switchSeq
	drive(m, m.key(press("2")))
	answerSwitch(m, seq, tg)
	drive(m, m.key(press("y")))
	if _, ok := m.Restart(); ok {
		t.Fatal("a late answer asked, and a typed y restarted the app")
	}
}

func TestSwitchAnswerWhileKeysAreTakenOnlyToasts(t *testing.T) {
	t.Parallel()
	tg := target.Target{User: "you", Host: "github.com"}
	for name, open := range map[string]string{"line": ":", "help": "?"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			m, _ := switchApp(t, nil)
			m.switchCommand("user you")
			drive(m, m.key(press(open)))
			m.switchSeq = 1
			answerSwitch(m, 1, tg)
			if !hasToast(m, "you on github.com can be used; run :switch again to switch.") {
				t.Fatalf("toast = %q", toasted(m))
			}
			if m.modals.Top() != nil {
				t.Fatal("the confirm opened over what had the keys")
			}
			if name == "line" {
				typeKeys(m, "y")
			} else {
				drive(m, m.key(press("y")))
			}
			if _, ok := m.Restart(); ok {
				t.Fatal("a typed y restarted the app")
			}
		})
	}
}

func TestSwitchStaleSeqIsDropped(t *testing.T) {
	t.Parallel()
	m, _ := switchApp(t, nil)
	m.switchCommand("user you")
	first := m.switchSeq
	m.switchCommand("user other")
	answerSwitch(m, first, target.Target{User: "you", Host: "github.com"})
	if m.modals.Top() != nil {
		t.Fatal("the answer of a replaced check opened the confirm")
	}
}

func TestSwitchConfirmCancel(t *testing.T) {
	t.Parallel()
	m, _ := switchApp(t, nil)
	runCommand(t, m, "switch host ghe.example.com")
	drive(m, m.key(tea.KeyPressMsg{Code: tea.KeyEscape}))
	if _, ok := m.Restart(); ok {
		t.Fatal("esc restarted the app")
	}
}

func TestSwitchKeyOpensLine(t *testing.T) {
	t.Parallel()
	m, _ := switchApp(t, nil)
	drive(m, m.key(press("P")))
	if !m.line.Focused() || m.line.Value() != "switch " {
		t.Fatalf("focused %v, line %q", m.line.Focused(), m.line.Value())
	}
}

func TestSwitchWithoutSwitcher(t *testing.T) {
	t.Parallel()
	m, _ := newTestApp(t)
	drive(m, m.key(press("P")))
	if m.line.Focused() {
		t.Fatal("the key opened the line with nothing to switch with")
	}
}

func TestSwitchCompletion(t *testing.T) {
	t.Parallel()
	m, _ := switchApp(t, nil)
	texts := func(arg string) []string {
		cands := m.completeSwitch(arg, len(arg), len(arg), true)
		out := make([]string, 0, len(cands))
		for _, c := range cands {
			out = append(out, c.Text)
		}
		return out
	}
	for arg, want := range map[string]string{"": "user", "u": "user", "h": "host", "user ": "me", "user m": "me", "host ": "github.com"} {
		if got := texts(arg); len(got) == 0 || got[0] != want {
			t.Errorf("completions of %q = %v, want %q first", arg, got, want)
		}
	}
	if got := texts("user me"); len(got) != 0 {
		t.Errorf("completions of a finished login = %v", got)
	}
}
