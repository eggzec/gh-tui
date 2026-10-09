package tui

import (
	"strconv"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/cmdline"
	"github.com/eggzec/gh-tui/pkg/bubbles/keyhelp"
	"github.com/eggzec/gh-tui/pkg/bubbles/pager"
	"github.com/eggzec/gh-tui/pkg/bubbles/toast"
)

// WithSource sets what the config file said, for the config command:
// where it is, and the line of each value it set. Without it the command
// names no file.
func WithSource(src config.Source) Option {
	return func(m *Model) { m.source = src }
}

// configDefaults is the word after config that shows default.yaml.
const configDefaults = "defaults"

// configCommand shows the session's config in a pager, each value that
// isn't the default with where it came from, or with "defaults",
// default.yaml as it is embedded. Neither writes anything.
func (m *Model) configCommand(arg string) tea.Cmd {
	switch strings.TrimSpace(arg) {
	case "":
		text, err := config.Layers{Source: m.source, Start: m.file, Session: m.cfg}.YAML()
		if err != nil {
			return m.toast.Push(toast.Error, "Can't show the config: "+ui.OneLine(err.Error())+".")
		}
		return m.openText("Config", "config.yaml", m.configHeader()+text+prefetchTable(m.cfg.Prefetch), false)
	case configDefaults:
		return m.openText("Default config", "default.yaml", config.DefaultFile(), false)
	}
	return m.toast.Push(toast.Warning, "Can't show config "+strconv.Quote(ui.OneLine(strings.TrimSpace(arg)))+". Use config, or config "+configDefaults+".")
}

// configHeader returns the comments that open the config command's YAML:
// the file, the host, the account and the profile, and how to read the
// comments.
func (m *Model) configHeader() string {
	lines := []string{
		"The config of this session: default.yaml, then the config file, then",
		"what :set changed.",
	}
	if p := m.source.Path(); p == "" {
		lines = append(lines, "file: none")
	} else {
		file := ui.ShortPath(p)
		if !m.source.Exists() {
			file += " (there is none)"
		}
		for _, l := range m.source.Header(file) {
			lines = append(lines, ui.OneLine(l))
		}
	}
	lines = append(lines,
		"A comment after a value says where it came from: the file and its",
		"line, with the hosts or profiles entry it is under, if any,",
		config.OriginStartup+" (which raised the log level at startup), or",
		"the session. A value without one is the default, which",
		":config "+configDefaults+" shows.",
	)
	var b strings.Builder
	for _, l := range lines {
		b.WriteString("# " + l + "\n")
	}
	b.WriteString("\n")
	return b.String()
}

// prefetchTable returns, as comments, what each page and kind of item
// reads ahead, each knob as it resolves through the three layers of
// prefetch and with the setting it came from, since the YAML shows only
// what each layer sets.
func prefetchTable(p config.PrefetchLayers) string {
	var b strings.Builder
	b.WriteString("\n# What each page and kind of item reads ahead, with the setting each\n# value comes from:\n")
	for _, l := range p.Table() {
		b.WriteString("#   " + l + "\n")
	}
	return b.String()
}

// completeConfig completes the one word the config command takes.
func completeConfig(_ *Model, arg string, cursor, end int, _ bool) []cmdline.Candidate {
	word := strings.TrimLeft(arg, " ")
	if strings.Contains(word, " ") || !strings.HasPrefix(configDefaults, word) {
		return nil
	}
	return []cmdline.Candidate{{Text: configDefaults, Detail: "show default.yaml", Start: cursor - len(word), End: end}}
}

// openText opens text, a file named name, which its extension
// highlights, in a pager in a modal titled title, which opens it in the
// editor too. wrap soft-wraps long lines, as prose wants, where a file
// whose lines line up scrolls sideways.
func (m *Model) openText(title, name, text string, wrap bool) tea.Cmd {
	back := ui.In(m.cfg.Keys, "text").Binding("global.back", "back")
	// Over a modal, the text replaces it, and back returns to it.
	over := m.modal
	back.SetEnabled(over != nil)
	t := &textModal{title: title, back: back, returns: over != nil, pager: pager.New(pager.WithKeyMap(pager.NewKeyMap(ui.In(m.cfg.Keys, "text"))), pager.WithEditor(m.cfg.Editor)), icons: ui.NewIcons(m.cfg.UI.Icons)}
	t.pager.Focus()
	t.pager.SetWrap(wrap)
	m.openModalOver(t, over)
	return t.pager.SetContent(name, text)
}

// textModal shows text in a pager, in a modal over the screen, until the
// pager asks to close.
type textModal struct {
	title string
	// back is off, and the modal takes the key so that it does nothing,
	// unless it opened over another modal, which the key returns to:
	// returns says so.
	back    key.Binding
	returns bool
	pager   pager.Model
	// icons mark what the pager says went wrong.
	icons ui.Icons
}

// Title returns the title the modal was opened with.
func (t *textModal) Title() string { return t.title }

// Update closes the modal when the pager asks, and passes the rest to the
// pager.
func (t *textModal) Update(msg tea.Msg) tea.Cmd {
	if c, ok := msg.(pager.CloseMsg); ok {
		if c.ID != t.pager.ID() {
			return nil
		}
		return ui.CloseModal(t)
	}
	var cmd tea.Cmd
	t.pager, cmd = t.pager.Update(msg)
	return cmd
}

// View renders the pager.
func (t *textModal) View() string { return t.pager.View() }

// SetSize sets the size of the pager.
func (t *textModal) SetSize(width, height int) { t.pager.SetSize(width, height) }

// SetTheme styles the pager.
func (t *textModal) SetTheme(th ui.Theme) { t.pager.SetStyles(th.Pager(t.icons)) }

// Commands implements ui.Commanded: the text takes only the commands that act on the app.
func (t *textModal) Commands() []string { return nil }

// KeyLayers implements ui.Keyed: the pager's keys.
func (t *textModal) KeyLayers() []keyhelp.Layer {
	return []keyhelp.Layer{ui.PagerLayer("text", &t.pager, t.back)}
}

var _ ui.Actor = (*textModal)(nil)

// Act implements ui.Actor. The quit key closes the modal, and the dismiss
// key clears the search or the filter first. The back key does nothing,
// unless the modal replaced another, which it returns to.
// Every other intent is the app's to refuse while it is open.
func (t *textModal) Act(action string) (tea.Cmd, bool) {
	switch action {
	case ui.ActQuit:
		return ui.CloseModal(t), true
	case ui.ActDismiss:
		if cmd, ok := t.pager.ClearTransient(); ok {
			return cmd, true
		}
		return ui.CloseModal(t), true
	case ui.ActBack:
		return nil, !t.returns
	}
	return nil, false
}
