package tui

import (
	"reflect"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/cmdline"
	"github.com/eggzec/gh-tui/pkg/bubbles/toast"
)

// WithSettings sets the function told the config whenever the set command
// changes a setting for the session, for what the app doesn't hold itself,
// such as the polling. It must not block.
func WithSettings(apply func(config.Config)) Option {
	return func(m *Model) { m.settings = apply }
}

// setCommand changes a setting for this session, as "key=value", with
// the value read and validated as in the config file, which it never
// writes. A key alone shows the value of the setting, and a key with &
// after it, as in vim, drops what the session set, so the setting is
// what gh-tui started with again: the config file's, or the log level
// that --debug, GH_DEBUG or GH_TUI_LOG raised it to.
func (m *Model) setCommand(arg string) tea.Cmd {
	key, value, assign := strings.Cut(arg, "=")
	key, value = strings.TrimSpace(key), strings.TrimSpace(value)
	key, reset := strings.CutSuffix(key, "&")
	key = strings.TrimSpace(key)
	switch {
	case key == "":
		return m.toast.Push(toast.Error, "Set what? Use set key=value, set key to see its value, or set key& to reset it.")
	case reset && assign:
		return m.toast.Push(toast.Error, "Set "+ui.OneLine(key)+"& resets it, and takes no value.")
	case strings.HasPrefix(key, "keys.") || strings.HasPrefix(key, "themes."):
		return m.toast.Push(toast.Error, "Keys and themes can't be set here: change them in the config file, then restart gh-tui.")
	}
	was, err := m.cfg.Get(key)
	if err != nil {
		// A setting and its value split by white space, with nothing typed
		// that the hint would drop: no =, no &.
		if i := strings.IndexAny(key, " \t"); i >= 0 && !assign && !reset {
			first, rest := key[:i], strings.TrimSpace(key[i:])
			if _, err := m.cfg.Get(first); err == nil {
				return m.toast.Push(toast.Error, "Write it as set "+first+"="+ui.OneLine(rest)+".")
			}
		}
		return m.toast.Push(toast.Error, "Unknown setting: "+ui.OneLine(key)+".")
	}
	if reset {
		return m.resetSetting(key, was)
	}
	if !assign {
		return m.toast.Push(toast.Info, key+" is "+ui.OneLine(was)+".")
	}
	if why, ok := config.Startup(key); ok {
		return m.toast.Push(toast.Error, key+" can't change while gh-tui runs: "+why+". Set it in the config file, then restart.")
	}
	cfg, err := m.cfg.Set(key, value)
	if err != nil {
		return m.toast.Push(toast.Error, "Can't set "+key+": "+reason(key, err)+".")
	}
	m.cfg = cfg
	now, _ := cfg.Get(key)
	return tea.Batch(m.applySettings(), m.toast.Push(toast.Info, key+" is "+ui.OneLine(now)+" for this session."))
}

// resetSetting sets key back to what gh-tui started with, from was,
// what the session has.
func (m *Model) resetSetting(key, was string) tea.Cmd {
	cfg, err := m.cfg.Reset(key, m.file)
	if err == nil && reflect.DeepEqual(cfg, m.cfg) {
		return m.toast.Push(toast.Info, key+" is "+ui.OneLine(was)+", as gh-tui started with.")
	}
	if err != nil {
		// What the file says was valid with what the session set of the
		// others, such as a size within a bound another sets.
		return m.toast.Push(toast.Error, "Can't reset "+key+": "+reason(key, err)+".")
	}
	m.cfg = cfg
	now, _ := cfg.Get(key)
	return tea.Batch(m.applySettings(), m.toast.Push(toast.Info, key+" is "+ui.OneLine(now)+" again, as gh-tui started with."))
}

// reason words err, which says what is wrong with the value of key, for a
// toast that names key already.
func reason(key string, err error) string {
	var lines []string
	for line := range strings.SplitSeq(err.Error(), "\n") {
		if _, after, ok := strings.Cut(line, key+": "); ok {
			line = after
		}
		if line = strings.TrimSpace(line); line != "" {
			lines = append(lines, line)
		}
	}
	return ui.OneLine(strings.Join(lines, "; "))
}

// applySettings applies the config to what reads it while the app runs:
// the sections, which take what they use of it, whatever WithSettings
// tells, and the theme, which draws the sections with what they took.
func (m *Model) applySettings() tea.Cmd {
	// Toasts shown keep the time they were given; later ones take the new.
	if m.cfg.UI.Toast != m.toastTimes {
		m.toastTimes = m.cfg.UI.Toast
		m.toast.SetDuration(m.toastTimes.Info)
		m.toast.SetErrorDuration(m.toastTimes.Error)
	}
	// The modals opened from now on start as it says.
	m.maximizedModals = m.cfg.UI.Maximized
	if m.settings != nil {
		m.settings(m.cfg)
	}
	cmd := m.broadcast(ui.SettingsMsg{Config: m.cfg})
	m.applyTheme(m.theme.Dark)
	return cmd
}

// maxValue is the most characters of the value of a setting that its
// candidate shows.
const maxValue = 20

// completeSet completes the key of a setting, and then its value, if it
// chooses from a few. A key completes with the '=' after it at the end
// of the line, ready for the value.
func (m *Model) completeSet(arg string, cursor, end int, atEnd bool) []cmdline.Candidate {
	word := strings.TrimLeft(arg, " ")
	start := cursor - len(word)
	if key, typed, ok := strings.Cut(word, "="); ok {
		var out []cmdline.Candidate
		for _, v := range m.cfg.Values(strings.TrimSpace(key)) {
			if strings.HasPrefix(v, typed) {
				out = append(out, cmdline.Candidate{Text: v, Start: start + len(key) + 1, End: end})
			}
		}
		return out
	}
	if strings.Contains(word, " ") {
		return nil
	}
	var first, inside []string
	for _, key := range config.Keys() {
		switch {
		case strings.HasPrefix(key, word):
			first = append(first, key)
		case strings.Contains(key, word):
			inside = append(inside, key)
		}
	}
	out := make([]cmdline.Candidate, 0, maxCandidates)
	for _, key := range append(first, inside...) {
		if len(out) == maxCandidates {
			break
		}
		text := key
		if atEnd {
			text += "="
		}
		detail, _ := m.cfg.Get(key)
		detail = m.shorten(ui.OneLine(detail), maxValue)
		if _, ok := config.Startup(key); ok {
			detail += ", at startup"
		}
		out = append(out, cmdline.Candidate{Text: text, Label: key, Detail: detail, Start: start, End: end})
	}
	return out
}
