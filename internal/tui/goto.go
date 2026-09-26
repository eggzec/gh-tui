package tui

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"unicode"
	"unicode/utf8"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/obs"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/toast"
)

// Repos reads repositories, so that goto opens only one that exists.
type Repos interface {
	// CachedGet returns the repository if it is in memory, without I/O.
	CachedGet(ref core.RepoRef) (core.Repo, bool)
	// Get reads the repository. One that doesn't exist, or that the
	// viewer may not see, fails with an error matching core.ErrNotFound.
	Get(ctx context.Context, ref core.RepoRef) (core.Repo, error)
}

// WithRepos sets what reads the repositories that goto opens. Without it
// goto opens any repository named, without checking that it exists.
func WithRepos(r Repos) Option {
	return func(m *Model) { m.repos = r }
}

// WithHost sets the user's GitHub host, such as github.com or an
// Enterprise host with its port, whose links goto opens. It defaults to
// github.com.
func WithHost(host string) Option {
	return func(m *Model) { m.host = host }
}

// WithUnreachable sets the function that tells whether an error means
// GitHub couldn't be reached, so that goto can say so.
func WithUnreachable(f func(ctx context.Context, err error) bool) Option {
	return func(m *Model) { m.unreachable = f }
}

// going is a goto waiting for GitHub, shown in the footer until it ends.
type going struct {
	seq    int
	target core.Target
	cancel context.CancelFunc
}

// gotoRepoMsg reports whether the repository of goto seq exists.
type gotoRepoMsg struct {
	seq  int
	ref  core.RepoRef
	repo core.Repo
	err  error
}

// gotoCommand opens what arg names: a repository on its screen.
func (m *Model) gotoCommand(arg string) tea.Cmd {
	t, err := core.ParseTarget(arg, m.host)
	if err != nil {
		return m.toast.Push(toast.Error, sentence(err.Error()))
	}
	if !t.HasNumber() {
		return m.gotoRepo(t.Repo)
	}
	return m.toast.Push(toast.Error, "goto doesn't open numbers yet.")
}

// gotoRepo shows the repository screen of ref, once it is known to exist.
func (m *Model) gotoRepo(ref core.RepoRef) tea.Cmd {
	if m.repos == nil {
		logGoto(m.ctx, core.Target{Repo: ref}, "unchecked")
		return m.selectRepo(ui.RepoMsg{Repo: ref})
	}
	if r, ok := m.repos.CachedGet(ref); ok {
		logGoto(m.ctx, core.Target{Repo: ref}, "memory")
		return m.selectRepo(ui.RepoMsg{Repo: canonical(r, ref)})
	}
	ctx, seq, spin := m.startGoto(core.Target{Repo: ref})
	repos := m.repos
	return tea.Batch(spin, func() tea.Msg {
		ctx, end := obs.Begin(ctx, "goto.repo")
		logGoto(ctx, core.Target{Repo: ref}, "github")
		r, err := repos.Get(ctx, ref)
		end(err, "span", "tui", "repo", ref.String())
		return gotoRepoMsg{seq: seq, ref: ref, repo: r, err: err}
	})
}

// gotRepo shows the repository that goto asked for, or says why not.
func (m *Model) gotRepo(msg gotoRepoMsg) tea.Cmd {
	g := m.endGoto(msg.seq)
	if g == nil {
		return nil
	}
	if msg.err != nil {
		return m.gotoFailed(g.target, msg.err)
	}
	return m.selectRepo(ui.RepoMsg{Repo: canonical(msg.repo, msg.ref)})
}

// canonical returns the name of r as GitHub spells it, or ref if r lacks
// it.
func canonical(r core.Repo, ref core.RepoRef) core.RepoRef {
	if r.Ref.Owner == "" || r.Ref.Name == "" {
		return ref
	}
	return r.Ref
}

// gotoFailed tells why goto couldn't open t.
func (m *Model) gotoFailed(t core.Target, err error) tea.Cmd {
	var text string
	switch {
	case errors.Is(err, context.Canceled):
		return nil
	case m.unreachable != nil && m.unreachable(m.ctx, err):
		text = fmt.Sprintf("Can't reach GitHub to open %s.", t)
	case errors.Is(err, core.ErrNotFound):
		text = "Repository not found: " + t.Repo.String() + "."
	default:
		text = sentence(fmt.Sprintf("Couldn't open %s: %v", t, err))
	}
	return m.toast.Push(toast.Error, text)
}

// logGoto logs that goto opens t, and where it learned what t is.
func logGoto(ctx context.Context, t core.Target, found string) {
	slog.InfoContext(ctx, "goto", "span", "tui", "target", t.String(), "found", found)
}

// sentence makes s, such as an error, a sentence for a toast: it starts
// with a capital and ends with a full stop.
func sentence(s string) string {
	r, n := utf8.DecodeRuneInString(s)
	s = string(unicode.ToUpper(r)) + s[n:]
	if !strings.HasSuffix(s, ".") {
		s += "."
	}
	return s
}

// startGoto starts waiting for GitHub on t, in place of any goto waiting
// already, and returns the context to wait with, the goto's sequence
// number and the command that spins the spinner in the footer.
func (m *Model) startGoto(t core.Target) (context.Context, int, tea.Cmd) {
	m.cancelGoto()
	ctx, cancel := context.WithCancel(m.ctx)
	m.gotoSeq++
	m.going = &going{seq: m.gotoSeq, target: t, cancel: cancel}
	m.layout()
	return ctx, m.gotoSeq, m.spin.Tick
}

// endGoto ends goto seq and returns it, or nil if it isn't the one
// waiting, since another replaced it or the user went elsewhere.
func (m *Model) endGoto(seq int) *going {
	g := m.going
	if g == nil || g.seq != seq {
		return nil
	}
	g.cancel()
	m.going = nil
	m.layout()
	return g
}

// cancelGoto stops the goto waiting for GitHub, if any: the user ran
// another command or went elsewhere.
func (m *Model) cancelGoto() {
	if m.going == nil {
		return
	}
	m.going.cancel()
	m.going = nil
	m.layout()
}

// newSpinner returns the spinner of a goto waiting in the footer.
func newSpinner() spinner.Model {
	return spinner.New(spinner.WithSpinner(spinner.MiniDot))
}

// goingView is the footer while a goto waits for GitHub.
func (m *Model) goingView() string {
	return m.spin.View() + " " + m.theme.Muted.Render("Opening "+m.going.target.String()+"…")
}
