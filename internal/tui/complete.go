package tui

import (
	"iter"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/pkg/bubbles/cmdline"
)

// Recall is what the command line completes from: what the app has read
// already. The command line completes on every edit, so its methods read
// memory only and must be fast.
type Recall interface {
	// Repos returns repositories the app knows of, such as those of the
	// dashboard and those pinned, the most relevant first.
	Repos() []core.RepoRef
	// Numbers returns issues and pull requests of repo that the app has
	// read, the most relevant first.
	Numbers(repo core.RepoRef) []Numbered
}

// Numbered is an issue or pull request that goto completes.
type Numbered struct {
	Number int
	Title  string
}

// WithRecall sets what the command line completes repositories and
// numbers from. Without it, it completes the names of commands, and the
// repositories opened in this session or named in the history.
func WithRecall(r Recall) Option {
	return func(m *Model) { m.recall = r }
}

// maxCandidates is the most candidates the command line offers, so the
// row stays short and each is quick to reach with tab.
const maxCandidates = 8

// maxDetail is the most characters of a title a candidate shows, so that
// the row holds a few candidates rather than one long title.
const maxDetail = 28

// maxRecent is how many repositories opened in this session the app
// remembers, for completion.
const maxRecent = 10

// remember puts repo first among the repositories opened recently.
func (m *Model) remember(repo core.RepoRef) {
	m.recent = slices.DeleteFunc(m.recent, repo.Same)
	m.recent = slices.Insert(m.recent, 0, repo)
	m.recent = m.recent[:min(len(m.recent), maxRecent)]
}

// complete returns the candidates that complete line at cursor: the names
// of commands in the first word, and the argument of a command that
// completes it.
func (m *Model) complete(line string, cursor int) []cmdline.Candidate {
	before := line[:cursor]
	start := len(before) - len(strings.TrimLeft(before, " "))
	name, arg, hasArg := strings.Cut(before[start:], " ")
	end := cursor + wordLen(line[cursor:])
	if !hasArg {
		if name == "" {
			// The placeholder says what to type.
			return nil
		}
		return completeCommand(name, start, end, end == len(line), m.topModal() != nil)
	}
	c, ok := findCommand(name)
	if !ok || c.complete == nil || m.topModal() != nil && !c.overModal {
		// A command that is refused over the modal gets no suggestions.
		return nil
	}
	return c.complete(m, arg, cursor, end, end == len(line))
}

// completeTarget completes what goto opens: a repository, or a number
// after '#'. It takes one argument, so only its first word completes.
func (m *Model) completeTarget(arg string, cursor, end int, _ bool) []cmdline.Candidate {
	word := strings.TrimLeft(arg, " ")
	if strings.Contains(word, " ") {
		return nil
	}
	wordStart := cursor - len(word)
	if strings.Contains(word, "#") {
		return m.completeNumber(word, wordStart, end)
	}
	return m.completeRepo(word, wordStart, end)
}

// wordLen returns the length of the word that s starts with.
func wordLen(s string) int {
	if i := strings.IndexByte(s, ' '); i >= 0 {
		return i
	}
	return len(s)
}

// completeCommand completes the name of a command from prefix, which
// spans start to end of the line, of those that run over a modal if
// overModal is set. A command that takes an argument gets a space after
// it at the end of the line, ready for the argument.
func completeCommand(prefix string, start, end int, atEnd, overModal bool) []cmdline.Candidate {
	var out []cmdline.Candidate
	for _, c := range commands {
		if !strings.HasPrefix(c.name, prefix) || overModal && !c.overModal {
			continue
		}
		text := c.name
		if c.args && atEnd {
			text += " "
		}
		out = append(out, cmdline.Candidate{Text: text, Label: c.name, Detail: c.detail, Start: start, End: end})
	}
	return out
}

// completeRepo completes the name of a repository from word: first those
// whose full name starts with it, then those whose name does, then those
// that contain it, each in order of relevance, the repositories opened
// most recently first.
func (m *Model) completeRepo(word string, start, end int) []cmdline.Candidate {
	if isLink(word) {
		return nil
	}
	lower := strings.ToLower(word)
	var tiers [3][]core.RepoRef
	seen := make(map[string]bool)
	for repo := range m.knownRepos() {
		// GitHub matches names regardless of case.
		full := strings.ToLower(repo.String())
		if seen[full] {
			continue
		}
		seen[full] = true
		switch {
		case strings.HasPrefix(full, lower):
			tiers[0] = append(tiers[0], repo)
		case strings.HasPrefix(strings.ToLower(repo.Name), lower):
			tiers[1] = append(tiers[1], repo)
		case strings.Contains(full, lower):
			tiers[2] = append(tiers[2], repo)
		}
		if len(tiers[0]) >= maxCandidates {
			break
		}
	}
	out := make([]cmdline.Candidate, 0, maxCandidates)
	for _, tier := range tiers {
		for _, repo := range tier {
			if len(out) == maxCandidates {
				return out
			}
			out = append(out, cmdline.Candidate{Text: repo.String(), Start: start, End: end})
		}
	}
	return out
}

// knownRepos yields the repositories to complete: those selected in this
// session, then those that goto went to in the history of the command
// line, which an earlier session may have kept, the latest first, then
// those Recall knows of.
func (m *Model) knownRepos() iter.Seq[core.RepoRef] {
	return func(yield func(core.RepoRef) bool) {
		for _, r := range m.recent {
			if !yield(r) {
				return
			}
		}
		for _, line := range slices.Backward(m.line.History()) {
			if r, ok := m.wentTo(line); ok && !yield(r) {
				return
			}
		}
		if m.recall == nil {
			return
		}
		for _, r := range m.recall.Repos() {
			if !yield(r) {
				return
			}
		}
	}
}

// wentTo returns the repository that line, of the history, went to, if
// it is a goto that names one.
func (m *Model) wentTo(line string) (core.RepoRef, bool) {
	name, arg, _ := strings.Cut(line, " ")
	if name != "goto" {
		return core.RepoRef{}, false
	}
	t, err := core.ParseTarget(arg, m.host)
	return t.Repo, err == nil && t.HasRepo()
}

// completeNumber completes the number after '#' in word, in the
// repository before it or else the selected one, from the issues and pull
// requests the app has read, with their titles.
func (m *Model) completeNumber(word string, start, end int) []cmdline.Candidate {
	if m.recall == nil || isLink(word) {
		return nil
	}
	name, digits, _ := strings.Cut(word, "#")
	repo := m.repo
	if name != "" {
		r, err := core.ParseRepoRef(name)
		if err != nil {
			return nil
		}
		repo = r
	}
	if repo == (core.RepoRef{}) || strings.Trim(digits, "0123456789") != "" {
		return nil
	}
	var out []cmdline.Candidate
	for _, n := range m.recall.Numbers(repo) {
		num := strconv.Itoa(n.Number)
		if !strings.HasPrefix(num, digits) {
			continue
		}
		out = append(out, cmdline.Candidate{
			Text: name + "#" + num, Label: "#" + num, Detail: m.shorten(n.Title, maxDetail), Start: start, End: end,
		})
		if len(out) == maxCandidates {
			break
		}
	}
	return out
}

// shorten cuts s to n characters, ending with the icon set's ellipsis if
// it cut.
func (m *Model) shorten(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	tail := m.icons.Ellipsis
	r := []rune(s)
	return strings.TrimRight(string(r[:max(n-utf8.RuneCountInString(tail), 0)]), " ") + tail
}

// isLink reports whether word starts a link rather than a name, which
// completion leaves alone.
func isLink(word string) bool {
	first, _, _ := strings.Cut(word, "/")
	return strings.ContainsAny(first, ".:")
}
