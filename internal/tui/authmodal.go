package tui

import (
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/access"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/keyhelp"
	"github.com/eggzec/gh-tui/pkg/termtext"
)

// authWidth is the widest the auth modal grows.
const authWidth = 76

// authModal shows what the token may do and what it lacks, and asks to
// run what grants it more, if something can.
type authModal struct {
	account string
	access  core.Access
	plan    access.Plan
	token   *ui.Token
	// checking is set until the token was read again, and err is why
	// that failed, or nil.
	checking bool
	err      error
	// run runs the plan's command, with what the token could do before.
	run func(argv []string, before core.Access) tea.Cmd
	// gh is the gh on the PATH, which the question names by its name
	// alone, or "". checksOff says that auth.check is off, so the app
	// tries what the token seems to lack too.
	gh        string
	checksOff bool

	close   key.Binding
	confirm ui.ConfirmKeys
	voice   ui.Voice
	// icons mark what the token may do, and why it couldn't be read, in
	// errs.
	icons ui.Icons
	errs  ui.ErrorStyles
	st    authStyles
	cst   ui.ConfirmStyles
	links termtext.Links

	width, height int
	view          string
}

// authStyles style the lines of the auth modal.
type authStyles struct {
	text, muted, subtle, yes, no lipgloss.Style
}

func newAuthModal(keys config.Keymap, account string, a core.Access, p access.Plan, tok *ui.Token, run func([]string, core.Access) tea.Cmd) *authModal {
	v := ui.NewVoice(keys, "")
	v.Token = tok
	// Nothing the modal tells of has a page to open.
	v.Open.SetEnabled(false)
	m := &authModal{
		account:  account,
		access:   a,
		plan:     p,
		token:    tok,
		checking: true,
		run:      run,
		gh:       lookGH(),
		close:    ui.Binding(keys, config.ActionDismiss, "close"),
		confirm:  ui.DefaultConfirmKeys(),
		voice:    v,
		icons:    ui.NewIcons(config.Default().UI.Icons),
	}
	m.voice.Icons = &m.icons
	return m
}

// checked takes what the token may do, read again, and what grants it
// more, or why reading it failed.
func (m *authModal) checked(a core.Access, p access.Plan, err error) {
	m.checking, m.access, m.plan, m.err = false, a, p, err
	m.render()
}

// asks reports whether the modal asks to run the plan's command.
func (m *authModal) asks() bool {
	return !m.checking && len(m.plan.Cmd) > 0
}

// Title implements ui.Modal.
func (m *authModal) Title() string { return "Token" }

// Update implements ui.Modal: the keys that answer the question, or the
// one that closes the modal.
func (m *authModal) Update(msg tea.Msg) tea.Cmd {
	k, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return nil
	}
	if m.asks() {
		c := ui.Confirm{Run: func() tea.Cmd { return m.run(m.plan.Cmd, m.access) }}
		cmd, done := m.confirm.Answer(c, k)
		if !done {
			return nil
		}
		// The terminal is gh's once the modal is gone.
		return tea.Sequence(ui.CloseModal(m), cmd)
	}
	if key.Matches(k, m.close) {
		return ui.CloseModal(m)
	}
	return nil
}

// View implements ui.Modal.
func (m *authModal) View() string { return m.view }

// SetSize implements ui.Modal.
func (m *authModal) SetSize(width, height int) {
	m.width, m.height = max(width, 0), max(height, 0)
	m.render()
}

// Fit implements ui.Fitter: as wide as authWidth or the room, and as high
// as its lines.
func (m *authModal) Fit(maxWidth, maxHeight int) (width, height int) {
	w := min(maxWidth, authWidth)
	return w, min(maxHeight, len(m.lines(w)))
}

// SetTheme implements ui.Modal.
func (m *authModal) SetTheme(t ui.Theme) {
	m.voice.Icons = &m.icons
	m.st = authStyles{text: t.Text, muted: t.Muted, subtle: t.Subtle, yes: t.Success, no: t.Error}
	m.errs = t.Errors(m.icons)
	m.cst = t.Confirm(m.icons)
	m.render()
}

// KeyLayers implements ui.Keyed: the keys that answer the question while
// it is asked, which take every key, or else the one that closes it.
func (m *authModal) KeyLayers() []keyhelp.Layer {
	if m.asks() {
		return []keyhelp.Layer{m.confirm.Layer()}
	}
	return []keyhelp.Layer{{Source: "token", Bindings: []key.Binding{m.close}, Short: []key.Binding{m.close}}}
}

func (m *authModal) render() {
	if m.width <= 0 || m.height <= 0 {
		m.view = ""
		return
	}
	m.view = strings.Join(ui.FitLines(m.lines(m.width), m.width, m.height), "\n")
}

// lines renders the modal at width w: the account and the kind of token,
// its scopes, what it may do, and what grants it more.
func (m *authModal) lines(w int) []string {
	st, a, ic := &m.st, m.access, m.icons
	head := ui.OneLine(m.account) + ic.Separator + kindName(a)
	lines := []string{st.text.Render(termtext.Truncate(head, w, ic.Ellipsis))}
	if a.Kind == core.TokenClassic && a.Known {
		scopes := "no scopes"
		if len(a.Scopes) > 0 {
			scopes = strings.Join(a.Scopes, ", ")
		}
		for _, l := range wrapPlain("Scopes: "+scopes, w) {
			lines = append(lines, st.muted.Render(l))
		}
	}
	lines = append(lines, "")
	for _, c := range capabilities {
		lines = append(lines, m.capLine(c, w))
	}
	if m.checksOff {
		for _, l := range wrapPlain("auth.check is off, so gh-tui tries these anyway, and GitHub decides.", w) {
			lines = append(lines, st.subtle.Render(l))
		}
	}
	if len(a.SSO) > 0 {
		lines = append(lines, termtext.Truncate(st.no.Render(m.icons.No)+" "+st.text.Render("Some organizations need SSO authorization"), w, ic.Ellipsis))
	}
	lines = append(lines, "")
	switch {
	case m.checking:
		lines = append(lines, st.subtle.Render("Checking the token"+ic.Ellipsis))
		return lines
	case m.err != nil:
		text, hint := ui.Say(core.Explain("check the token", m.err), m.voice)
		lines = append(lines, ui.ErrorLine(m.errs, text, hint, w)...)
	}
	for _, l := range wrapPlain(m.plan.Why, w) {
		lines = append(lines, st.muted.Render(l))
	}
	if u := m.plan.URL; u != "" {
		lines = append(lines, m.links.Link(u, st.text.Render(termtext.Truncate(u, w, ic.Ellipsis))))
	}
	if m.asks() {
		q := ui.Confirm{Question: "Run " + commandLine(m.plan.Cmd, m.gh) + "?"}
		lines = append(lines, q.Lines(m.cst, m.confirm, w, ui.ConfirmLines)...)
	}
	return lines
}

// capLine renders whether the token may do c: the yes mark when GitHub
// said it may, the no mark and what it needs when it may not, and ? while
// that isn't known.
func (m *authModal) capLine(c capability, w int) string {
	st := &m.st
	ok, known := m.access.Allows(c.need)
	mark, why := st.subtle.Render("?"), ""
	switch {
	case !known:
	case ok:
		mark = st.yes.Render(m.icons.Yes)
	case m.access.Missing(c.need) != "":
		mark, why = st.no.Render(m.icons.No), m.icons.Separator+"needs "+m.access.Missing(c.need)
	default:
		mark, why = st.no.Render(m.icons.No), m.icons.Separator+"needs a classic token"
	}
	return termtext.Truncate(mark+" "+st.text.Render(c.name)+st.subtle.Render(why), w, m.icons.Ellipsis)
}

// kindName names the kind of token of a.
func kindName(a core.Access) string {
	switch a.Kind {
	case core.TokenClassic:
		return "classic token"
	case core.TokenFineGrained:
		return "fine-grained token"
	case core.TokenApp:
		return "GitHub App token"
	default:
		return "token of an unknown kind"
	}
}

// commandLine shows argv as the user would type it in a shell, on one
// line: the program by its name when it is gh, the gh on the PATH, and
// else by its path, such as one GH_PATH names, so that the user sees what
// runs, and each word quoted where a shell would need it, such as a path
// with a space. Only the prompt quotes: argv runs without a shell.
func commandLine(argv []string, gh string) string {
	if len(argv) == 0 {
		return ""
	}
	words := slices.Clone(argv)
	if gh != "" && argv[0] == gh {
		words[0] = filepath.Base(gh)
	}
	for i, w := range words {
		words[i] = shellQuote(ui.OneLine(w), i == 0)
	}
	return strings.Join(words, " ")
}

// shellQuote returns w as a POSIX shell reads it as one word: as it is
// when it holds only characters no shell treats specially, and else in
// single quotes, in which a single quote ends the quotes, is escaped
// with a backslash, and opens them again. A first word with an = is
// quoted too, since a shell reads one such as A=b as an assignment, not
// as the program to run, and so is any word that starts with one, which
// zsh expands to the path of the program it names.
func shellQuote(w string, first bool) string {
	plain := w != "" && strings.Trim(w, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_@%+=:,./-") == ""
	special := (first && strings.Contains(w, "=")) || strings.HasPrefix(w, "=")
	if plain && !special {
		return w
	}
	return "'" + strings.ReplaceAll(w, "'", `'\''`) + "'"
}

// lookGH returns the gh on the PATH, or "".
var lookGH = func() string {
	p, err := exec.LookPath("gh")
	if err != nil {
		return ""
	}
	return p
}

// wrapPlain wraps s, the app's own words, to lines of w cells, at spaces.
func wrapPlain(s string, w int) []string {
	if s == "" {
		return nil
	}
	return strings.Split(ansi.Wordwrap(ui.OneLine(s), max(w, 1), ""), "\n")
}
