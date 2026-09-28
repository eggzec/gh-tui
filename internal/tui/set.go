package tui

import (
	"slices"
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

// liveSettings are the settings that the set command changes while the
// app runs. Whatever reads the others reads them once, at startup.
var liveSettings = []string{
	"theme", "ui.icons",
	"details.prefetch.enabled", "details.prefetch.rows", "details.prefetch.hover_delay", "details.prefetch.filters",
	"dashboard.prefetch", "files.prefetch.enabled", "files.prefetch.max_size", "files.prefetch.hover_delay",
	"dashboard.calendar_glyph", "dashboard.contributions", "files.finder.preview", "notifications.mark_read_on_open",
	"history.row", "history.detail", "history.date_format", "history.show_email",
	"history.prefetch.around", "history.prefetch.hover_delay",
}

// startup says why a setting that isn't live needs a restart, by the
// start of its key; the first that matches says.
var startup = []struct{ prefix, why string }{
	{"repos", "the pinned repositories are read at startup"},
	{"cache.", "the cache is opened at startup"},
	{"sync.enabled", "the polls are set up at startup"},
	{"files.preview.", "the files are read with it from the start"},
	{"auth.", "the token's checks start with the app"},
	{"log.", "the log file is opened at startup"},
	{"", "it is read at startup"},
}

// needsRestart returns why the setting key can't change while the app
// runs, or false if it can.
func needsRestart(key string) (string, bool) {
	if slices.Contains(liveSettings, key) {
		return "", false
	}
	for _, s := range startup {
		if strings.HasPrefix(key, s.prefix) {
			return s.why, true
		}
	}
	return "", false
}

// setCommand changes a setting for this session, as "key=value", with
// the value read and validated as in the config file, which it never
// writes. A key alone shows the value of the setting.
func (m *Model) setCommand(arg string) tea.Cmd {
	key, value, assign := strings.Cut(arg, "=")
	key, value = strings.TrimSpace(key), strings.TrimSpace(value)
	switch {
	case key == "":
		return m.toast.Push(toast.Error, "Set what? Use set key=value, or set key to see its value.")
	case strings.HasPrefix(key, "keys.") || strings.HasPrefix(key, "themes."):
		return m.toast.Push(toast.Error, "Keys and themes can't be set here: change them in the config file, then restart gh-tui.")
	}
	was, err := m.cfg.Get(key)
	if err != nil {
		return m.toast.Push(toast.Error, "Unknown setting: "+ui.OneLine(key)+".")
	}
	if !assign {
		return m.toast.Push(toast.Info, key+" is "+ui.OneLine(was)+".")
	}
	if why, ok := needsRestart(key); ok {
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
		detail = shorten(ui.OneLine(detail), maxValue)
		if _, ok := needsRestart(key); ok {
			detail += ", at startup"
		}
		out = append(out, cmdline.Candidate{Text: text, Label: key, Detail: detail, Start: start, End: end})
	}
	return out
}
