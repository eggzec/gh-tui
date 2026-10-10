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
	// Owners returns the logins of users and organizations the app knows
	// of, such as the viewer's organizations, the most relevant first.
	Owners() []string
}

// Numbered is an issue or pull request that goto completes.
type Numbered struct {
	Number int
	Title  string
}

// WithRecall sets what the command line completes repositories, numbers
// and owners from. Without it, it completes the names of commands, and the
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
		return m.actionCandidates(name, start, end, end == len(line))
	}
	c, ok := findCommand(name)
	if !ok {
		return m.completeActionArg(name, arg, cursor, end)
	}
	if c.complete == nil || !m.runsOver(c) {
		// A command that is refused over the modal gets no suggestions.
		return nil
	}
	return c.complete(m, arg, cursor, end, end == len(line))
}

// completeActionArg completes the argument of the action that name is
// where the focus has it: only read takes one, all.
func (m *Model) completeActionArg(name, arg string, cursor, end int) []cmdline.Candidate {
	if action, ok := m.resolveAction(name); !ok || action != actionRead {
		return nil
	}
	word := strings.TrimLeft(arg, " ")
	if len(word) > len("all") || "all"[:len(word)] != word {
		return nil
	}
	return []cmdline.Candidate{{Text: "all", Label: "all", Detail: "mark every notification read", Start: cursor - len(word), End: end}}
}

// completeTarget completes what goto opens: a repository, a number after
// '#', or a user or organization after '@'. It takes one argument, so only
// its first word completes.
func (m *Model) completeTarget(arg string, cursor, end int, _ bool) []cmdline.Candidate {
	word := strings.TrimLeft(arg, " ")
	if strings.Contains(word, " ") {
		return nil
	}
	wordStart := cursor - len(word)
	if strings.Contains(word, "#") {
		return m.completeNumber(word, wordStart, end)
	}
	if login, ok := strings.CutPrefix(word, "@"); ok {
		return m.completeOwner(login, wordStart, end)
	}
	if word == "." {
		return m.completeHere(wordStart, end)
	}
	out := m.completeRepo(word, wordStart, end)
	if word == "" {
		out = append(m.completeHere(wordStart, end), out...)
		out = out[:min(len(out), maxCandidates)]
	}
	return out
}

// completeHere offers ".", the repository of the current directory, when
// there is one.
func (m *Model) completeHere(start, end int) []cmdline.Candidate {
	if m.here == (core.RepoRef{}) {
		return nil
	}
	return []cmdline.Candidate{{Text: ".", Detail: m.here.String(), Start: start, End: end}}
}

// wordLen returns the length of the word that s starts with.
func wordLen(s string) int {
	if i := strings.IndexByte(s, ' '); i >= 0 {
		return i
	}
	return len(s)
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
		for _, t := range slices.Backward(m.historyGotos()) {
			if t.HasRepo() && !yield(t.Repo) {
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

// gotoCache holds the targets that the goto lines of the command line's
// history named, oldest first, for the revision of the history they were
// read from.
type gotoCache struct {
	rev     uint64
	valid   bool
	targets []core.Target
	// parses counts the lines parsed, for the tests.
	parses int
}

// historyGotos returns the targets that goto went to in the history of the
// command line, oldest first. Completion asks on every keystroke, so the
// history is parsed again only after it changed.
func (m *Model) historyGotos() []core.Target {
	rev := m.line.HistoryRevision()
	if m.gotos.valid && m.gotos.rev == rev {
		return m.gotos.targets
	}
	m.gotos.targets = m.gotos.targets[:0]
	for _, line := range m.line.History() {
		name, arg, _ := strings.Cut(line, " ")
		if name != "goto" {
			continue
		}
		m.gotos.parses++
		if t, err := core.ParseTarget(arg, m.host); err == nil {
			m.gotos.targets = append(m.gotos.targets, t)
		}
	}
	m.gotos.rev, m.gotos.valid = rev, true
	return m.gotos.targets
}

// rememberOwner puts login first among the pages of owners opened
// recently. GitHub matches logins without regard to case.
func (m *Model) rememberOwner(login string) {
	m.recentOwners = slices.DeleteFunc(m.recentOwners, func(o string) bool { return strings.EqualFold(o, login) })
	m.recentOwners = slices.Insert(m.recentOwners, 0, login)
	m.recentOwners = m.recentOwners[:min(len(m.recentOwners), maxRecent)]
}

// completeOwner completes the login after '@' from prefix: first the
// logins that start with it, then those that contain it, each in order of
// relevance, the pages opened most recently first. The viewer's own login
// is left out, since it only shows the dashboard.
func (m *Model) completeOwner(prefix string, start, end int) []cmdline.Candidate {
	lower := strings.ToLower(prefix)
	var tiers [2][]string
	seen := make(map[string]bool)
	for login := range m.knownOwners() {
		key := strings.ToLower(login)
		if seen[key] || m.isViewer(login) {
			continue
		}
		seen[key] = true
		switch {
		case strings.HasPrefix(key, lower):
			tiers[0] = append(tiers[0], login)
		case strings.Contains(key, lower):
			tiers[1] = append(tiers[1], login)
		}
		if len(tiers[0]) >= maxCandidates {
			break
		}
	}
	out := make([]cmdline.Candidate, 0, maxCandidates)
	for _, tier := range tiers {
		for _, login := range tier {
			if len(out) == maxCandidates {
				return out
			}
			out = append(out, cmdline.Candidate{Text: "@" + login, Start: start, End: end})
		}
	}
	return out
}

// knownOwners yields the logins to complete: the pages opened in this
// session, then those that goto went to in the history of the command
// line, the latest first, then those Recall knows of.
func (m *Model) knownOwners() iter.Seq[string] {
	return func(yield func(string) bool) {
		for _, o := range m.recentOwners {
			if !yield(o) {
				return
			}
		}
		for _, t := range slices.Backward(m.historyGotos()) {
			if t.HasOwner() && !yield(t.Owner) {
				return
			}
		}
		if m.recall == nil {
			return
		}
		for _, o := range m.recall.Owners() {
			if !yield(o) {
				return
			}
		}
	}
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
