package tui

import (
	"context"
	"os/exec"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/obs"
	"github.com/eggzec/gh-tui/internal/service/access"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/toast"
)

// Access is what the app knows of the token, and how it grants it more,
// as the access service does.
type Access interface {
	ui.Checker
	// Changes receives what the token may do each time that changes.
	Changes() <-chan core.Access
	// Refresh says what the user can do so that the token may do what
	// needs each of needs.
	Refresh(needs ...core.Need) access.Plan
	// Reload reads the token again, such as after gh refreshed it, and
	// asks GitHub what it may do.
	Reload(ctx context.Context) (core.Access, error)
}

// WithAccess sets what the app knows of the token: the app tells the user
// once what the token can't do, the sections follow what it may do, and
// the auth command shows it and grants it more.
func WithAccess(a Access) Option {
	return func(m *Model) {
		m.access = a
		m.token = ui.NewToken(a, m.cfg.Keys)
		m.accessChanges = a.Changes()
	}
}

// runArgs runs a program in the terminal, which the app gives up until it
// ends, and reports how it ended to done.
type runArgs func(argv []string, done func(error) tea.Msg) tea.Cmd

// execArgs runs argv, a program and its arguments, never through a shell.
func execArgs(argv []string, done func(error) tea.Msg) tea.Cmd {
	return tea.ExecProcess(exec.Command(argv[0], argv[1:]...), done) //nolint:gosec // The program is gh, as the access service found it.
}

// capability is something the app does that the token may not be allowed.
type capability struct {
	name string
	need core.Need
}

// capabilities are what the app does that needs more of the token than a
// read of what is public, in the order the auth command lists them.
var capabilities = []capability{
	{"Read and mark notifications", core.NeedNotifications},
	{"Change public repositories", core.NeedWrite(core.RepoCaps{Known: true})},
	{"Change private repositories", core.NeedWrite(core.RepoCaps{Known: true, Private: true})},
	{"Re-run and cancel workflow runs", core.NeedRuns},
	{"Merge changes to workflows", core.NeedWorkflow},
}

// needs returns the needs of the capabilities.
func needs() []core.Need {
	out := make([]core.Need, len(capabilities))
	for i, c := range capabilities {
		out[i] = c.need
	}
	return out
}

// accessChangedMsg reports what the token may do, from the access
// service, each time that changes.
type accessChangedMsg struct {
	access core.Access
}

// authReloadedMsg reports the token read again, and what it may do.
// before is what it could do before gh ran, or nil when the auth command
// only checked it.
type authReloadedMsg struct {
	access core.Access
	err    error
	before *core.Access
}

// authRanMsg reports that the command that grants the token more ended,
// and how, with what the token could do before.
type authRanMsg struct {
	err    error
	before core.Access
}

// listenAccess waits for the next change of what the token may do.
func (m *Model) listenAccess() tea.Cmd {
	if m.accessChanges == nil {
		return nil
	}
	ch, ctx := m.accessChanges, m.ctx
	return func() tea.Msg {
		select {
		case a := <-ch:
			return accessChangedMsg{access: a}
		case <-ctx.Done():
			return nil
		}
	}
}

// startAccess tells the user what the token can't do, if GitHub said
// already, and listens for what it says later.
func (m *Model) startAccess() tea.Cmd {
	if m.access == nil {
		return nil
	}
	return tea.Batch(m.noticeOnce(m.access.Access()), m.listenAccess())
}

// updateAccess handles what the access service and the auth command
// report.
func (m *Model) updateAccess(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case accessChangedMsg:
		return tea.Batch(m.noticeOnce(msg.access), m.broadcast(ui.AccessMsg{Access: msg.access}), m.listenAccess())
	case authRanMsg:
		if msg.err != nil {
			return m.toast.Push(toast.Error, "gh auth refresh didn't finish; nothing changed.")
		}
		return m.reloadToken(&msg.before)
	case authReloadedMsg:
		m.noticed = true
		if a, ok := m.modal.(*authModal); ok {
			a.checked(msg.access, m.access.Refresh(needs()...), msg.err)
			// It may need more lines now.
			a.SetSize(m.modalSize())
		}
		// The sections read again what the token was refused, or failed
		// to read, since it may be a new token.
		cmd := m.broadcast(ui.AccessMsg{Access: msg.access})
		if msg.before == nil {
			return cmd
		}
		return tea.Batch(cmd, m.refreshed(msg))
	}
	return nil
}

// refreshed tells the user what gh's refresh of the token changed.
func (m *Model) refreshed(msg authReloadedMsg) tea.Cmd {
	if msg.err != nil {
		if text := ui.SayToast(core.Explain("use the new token", msg.err), m.voice, m.fitsToast); text != "" {
			return m.toast.Push(toast.Error, text)
		}
		return nil
	}
	if text := m.notice(msg.access); text != "" {
		return m.toast.Push(toast.Info, text)
	}
	var gained []string
	for _, s := range msg.access.Scopes {
		if !slices.Contains(msg.before.Scopes, s) {
			gained = append(gained, s)
		}
	}
	switch len(gained) {
	case 0:
		return m.toast.Push(toast.Info, "The token didn't change.")
	case 1:
		return m.toast.Push(toast.Info, "The token has the "+gained[0]+" scope now.")
	}
	last := len(gained) - 1
	return m.toast.Push(toast.Info, "The token has the "+strings.Join(gained[:last], ", ")+" and "+gained[last]+" scopes now.")
}

// noticeOnce tells the user what the token can't do, once a session, as
// soon as GitHub said what it may do.
func (m *Model) noticeOnce(a core.Access) tea.Cmd {
	if m.noticed || !decided(a) {
		return nil
	}
	m.noticed = true
	text := m.notice(a)
	if text == "" {
		return nil
	}
	return m.toast.Push(toast.Info, text)
}

// decided reports whether a says what the token may do: its kind is known,
// and so are the scopes of a classic token.
func decided(a core.Access) bool {
	return a.Kind != core.TokenUnknown && (a.Kind != core.TokenClassic || a.Known)
}

// notice returns what the user should know of what the token a may do
// can't do, the most that matters only, or "". Merging changes to
// workflows is left to the merge, since gh never asks for its scope.
func (m *Model) notice(a core.Access) string {
	var text string
	switch {
	case m.token.Check(core.NeedNotifications) != nil:
		text = "Notifications need a classic token with the notifications scope"
	case m.token.Check(core.NeedWrite(core.RepoCaps{Known: true})) != nil:
		text = "This token can't change anything on GitHub"
	case m.token.Check(core.NeedRuns) != nil:
		text = "Private repositories, re-runs and cancelling need the repo scope"
	case len(a.SSO) > 0:
		text = "Some organizations need SSO authorization"
	default:
		return ""
	}
	if h := m.token.Hint(); h != "" {
		return text + m.icons.Separator + h
	}
	return text + "."
}

// authCommand shows what the token may do and what it lacks, and offers
// to grant it more. It reads the token again first, since the user may
// have changed it since.
func (m *Model) authCommand(string) tea.Cmd {
	if m.access == nil {
		return m.toast.Push(toast.Error, "gh-tui doesn't know the token here.")
	}
	a := m.access.Access()
	mod := newAuthModal(m.cfg.Keys, m.account(), a, m.access.Refresh(needs()...), m.token, m.run)
	mod.checksOff = !m.cfg.Auth.Check
	mod.icons = ui.NewIcons(m.cfg.UI.Icons)
	m.openModal(mod)
	return m.reloadToken(nil)
}

// reloadToken reads the token again and asks GitHub what it may do.
// before is what it could do before gh refreshed it, or nil.
func (m *Model) reloadToken(before *core.Access) tea.Cmd {
	acc, ctx := m.access, m.ctx
	return func() tea.Msg {
		ctx, end := obs.Begin(ctx, "auth.reload")
		a, err := acc.Reload(ctx)
		end(err, "span", "tui")
		return authReloadedMsg{access: a, err: err, before: before}
	}
}

// run runs plan's command in the terminal, and then reads the token
// again, which could do before.
func (m *Model) run(argv []string, before core.Access) tea.Cmd {
	do := m.exec
	if do == nil {
		do = execArgs
	}
	return do(argv, func(err error) tea.Msg { return authRanMsg{err: err, before: before} })
}

// withExec sets how the app runs a program in the terminal, for tests.
func withExec(run runArgs) Option {
	return func(m *Model) { m.exec = run }
}
