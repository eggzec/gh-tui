package tui

import (
	"slices"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/cmdline"
	"github.com/eggzec/gh-tui/pkg/bubbles/toast"
)

// copyKind is what copy copies of the selection.
type copyKind struct {
	name string
	// detail says what it is, beside its name in the candidates, and noun
	// names it when the selection has none.
	detail, noun string
	of           func(ui.Selection) string
}

// copyKinds are what copy copies, in the order they complete.
var copyKinds = []copyKind{
	{name: "url", detail: "its page on GitHub", noun: "link", of: func(s ui.Selection) string { return s.URL }},
	{name: "ref", detail: "owner/name, or owner/name#number", noun: "reference", of: ref},
	{name: "sha", detail: "its commit SHA", noun: "commit SHA", of: func(s ui.Selection) string { return s.SHA }},
	{name: "path", detail: "its file path", noun: "path", of: func(s ui.Selection) string { return s.Path }},
}

// ref returns owner/name#number of s, or owner/name without a number.
func ref(s ui.Selection) string {
	if s.Repo.Owner == "" {
		return ""
	}
	if s.Number == 0 {
		return s.Repo.String()
	}
	return s.Repo.String() + "#" + strconv.Itoa(s.Number)
}

// maxCopy is the most bytes copy puts on the clipboard. OSC 52 carries
// them in one sequence, which terminals cap.
const maxCopy = 64 << 10

// maxCopied is the most characters of what was copied that its toast
// shows.
const maxCopied = 60

// copyCommand copies what arg names of the selection of the focused view
// to the system clipboard, through the terminal (OSC 52).
func (m *Model) copyCommand(arg string) tea.Cmd {
	i := slices.IndexFunc(copyKinds, func(k copyKind) bool { return k.name == arg })
	if i < 0 {
		what := "Copy what?"
		if arg != "" {
			what = "Can't copy " + strconv.Quote(ui.OneLine(arg)) + "."
		}
		return m.toast.Push(toast.Error, what+" Use copy url, ref, sha or path.")
	}
	k := copyKinds[i]
	var sel ui.Selection
	ok := false
	if p := m.focused(); p != nil {
		if s, is := p.section.(ui.Selector); is {
			sel, ok = s.Selected()
		}
	}
	if !ok {
		return m.toast.Push(toast.Error, "Nothing is selected to copy.")
	}
	text := k.of(sel)
	switch {
	case text == "":
		return m.toast.Push(toast.Error, article(sel.What)+" has no "+k.noun+" to copy.")
	case len(text) > maxCopy:
		return m.toast.Push(toast.Error, "The "+k.noun+" is too long to copy.")
	}
	return tea.Batch(tea.SetClipboard(text), m.toast.Push(toast.Info, "Copied "+m.shorten(ui.OneLine(text), maxCopied)+"."))
}

// article returns noun after "A" or "An", as its sound needs, capitalized
// to start a sentence.
func article(noun string) string {
	if noun == "" {
		return "The selection"
	}
	if strings.ContainsRune("aeiou", rune(noun[0])) {
		return "An " + noun
	}
	return "A " + noun
}

// completeCopy completes what copy copies.
func completeCopy(_ *Model, arg string, cursor, end int, _ bool) []cmdline.Candidate {
	word := strings.TrimLeft(arg, " ")
	if strings.Contains(word, " ") {
		return nil
	}
	var out []cmdline.Candidate
	for _, k := range copyKinds {
		if strings.HasPrefix(k.name, word) {
			out = append(out, cmdline.Candidate{Text: k.name, Detail: k.detail, Start: cursor - len(word), End: end})
		}
	}
	return out
}
