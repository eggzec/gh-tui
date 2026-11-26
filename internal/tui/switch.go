package tui

import (
	"context"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/target"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/cmdline"
	"github.com/eggzec/gh-tui/pkg/bubbles/confirm"
	"github.com/eggzec/gh-tui/pkg/bubbles/toast"
)

// Switcher checks that the program can run as t, whose login and host are
// both known, and says why it can't. The app calls it in a command, so it
// may wait on the network; it never reads or changes what the app shows.
type Switcher func(ctx context.Context, t target.Target) error

// WithSwitcher sets what checks an account before the switch command starts
// the program again as it. Without it, the command and its key say there is
// nothing to switch to.
func WithSwitcher(s Switcher) Option {
	return func(m *Model) { m.switcher = s }
}

// switchPrefix is what the switch key types into the command line.
const switchPrefix = "switch "

// switchCheckedMsg reports that the check of a switch to target ended.
type switchCheckedMsg struct {
	seq    int
	target target.Target
	err    error
}

// canSwitch reports whether the switch command and its key do something.
func (m *Model) canSwitch() bool { return m.switcher != nil }

// openSwitch opens the command line with the switch command typed, for the
// user to say which account.
func (m *Model) openSwitch() tea.Cmd {
	return m.openLineWith(switchPrefix)
}

// switchCommand checks the account that arg names, then asks whether to
// start the program again as it. A name that is the current account's, or
// that no account has, is told in a toast and nothing restarts.
func (m *Model) switchCommand(arg string) tea.Cmd {
	if !m.canSwitch() {
		return m.toast.Push(toast.Warning, "There is no other account to switch to here.")
	}
	t, err := target.Parse(arg, m.login, target.NormalizeHost(m.host))
	if err != nil {
		return m.toast.Push(toast.Warning, err.Error())
	}
	if t.User == "" {
		return m.toast.Push(toast.Warning, "The account's login isn't known yet; give both, as user@host.")
	}
	if strings.EqualFold(t.User, m.login) && t.Host == target.NormalizeHost(m.host) {
		return m.toast.Push(toast.Info, "Already signed in as "+t.User+" on "+t.Host+".")
	}
	m.switchSeq++
	seq, check, ctx := m.switchSeq, m.switcher, m.ctx
	run := func() tea.Msg {
		return switchCheckedMsg{seq: seq, target: t, err: check(ctx, t)}
	}
	return tea.Batch(m.toast.Push(toast.Info, "Checking "+t.String()+m.icons.Ellipsis), run)
}

// switchChecked tells why the account can't be used, or else asks whether
// to switch to it. The answer of a check that a later command replaced, or
// that the user went on from with a key press, is dropped; one that comes
// while something else has the keys only says that the account can be used,
// since a typed y must never answer a question nobody saw.
func (m *Model) switchChecked(msg switchCheckedMsg) tea.Cmd {
	if msg.seq != m.switchSeq {
		return nil
	}
	m.toast.DismissLevel(toast.Info)
	if msg.err != nil {
		return m.toast.Push(toast.Error, msg.err.Error())
	}
	t := msg.target
	if m.modals.Top() != nil || m.line.Focused() || m.helpOpen() {
		return m.toast.Push(toast.Info, t.User+" on "+t.Host+" can be used; run :switch again to switch.")
	}
	return ui.OpenConfirm(confirm.Ask{
		Question: "Switch to " + t.User + " on " + t.Host + "? History, filters and unsaved drafts reset.",
		Run:      func() tea.Cmd { return func() tea.Msg { return RestartMsg{Target: t} } },
	}, m.keys.confirm, m.icons)
}

// completeSwitch completes the argument of switch: the words user and host,
// then the login and the host of the session after each. It reads memory
// only, so other accounts of the host are not offered.
func (m *Model) completeSwitch(arg string, cursor, _ int, _ bool) []cmdline.Candidate {
	word := strings.TrimLeft(arg, " ")
	first, rest, spaced := strings.Cut(word, " ")
	if !spaced {
		var out []cmdline.Candidate
		for _, w := range []string{"user", "host"} {
			if w != first && strings.HasPrefix(w, first) {
				out = append(out, cmdline.Candidate{Text: w, Label: w, Detail: "switch to a login on this host or a host for this login", Start: cursor - len(first), End: cursor})
			}
		}
		return out
	}
	known, detail := m.login, "the current login"
	if first == "host" {
		known, detail = target.NormalizeHost(m.host), "the current host"
	} else if first != "user" {
		return nil
	}
	if known == "" || strings.Contains(rest, " ") || strings.EqualFold(known, rest) ||
		!strings.HasPrefix(strings.ToLower(known), strings.ToLower(rest)) {
		return nil
	}
	return []cmdline.Candidate{{Text: known, Label: known, Detail: detail, Start: cursor - len(rest), End: cursor}}
}
