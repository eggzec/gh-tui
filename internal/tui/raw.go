package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/cmdline"
	"github.com/eggzec/gh-tui/pkg/bubbles/toast"
)

// rawWords are what the raw command takes, in the order they complete.
var rawWords = []cmdline.Candidate{
	{Text: "on", Detail: "show the source"},
	{Text: "off", Detail: "show it rendered"},
}

// rawCommand shows the file open, such as a markdown file in the
// preview, as its source with on, or rendered with off. Alone it says
// which it shows, as set does of a setting. It changes only the file
// open: files.markdown says how the next one shows.
func (m *Model) rawCommand(arg string) tea.Cmd {
	s, ok := m.topModal().(ui.Sourced)
	raw, renders := false, false
	if ok {
		raw, renders = s.Raw()
	}
	switch {
	case arg != "" && arg != "on" && arg != "off":
		return m.toast.Push(toast.Warning, "Write it as raw on or raw off.")
	case !renders:
		return m.toast.Push(toast.Warning, "The open file has no rendered view.")
	case arg == "":
		if raw {
			return m.toast.Push(toast.Info, "raw is on: the file shows as its source.")
		}
		return m.toast.Push(toast.Info, "raw is off: the file shows rendered.")
	}
	return s.SetRaw(arg == "on")
}

// completeRaw completes what the raw command takes.
func completeRaw(_ *Model, arg string, cursor, end int, _ bool) []cmdline.Candidate {
	word := strings.TrimLeft(arg, " ")
	if strings.Contains(word, " ") {
		return nil
	}
	var out []cmdline.Candidate
	for _, c := range rawWords {
		if strings.HasPrefix(c.Text, word) {
			c.Start, c.End = cursor-len(word), end
			out = append(out, c)
		}
	}
	return out
}
