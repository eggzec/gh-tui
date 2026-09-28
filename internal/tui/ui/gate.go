package ui

import (
	"strconv"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/pkg/bubbles/toast"
)

// Action is a change that the viewer may or may not be allowed to make.
type Action int

// The actions that a Gate decides.
const (
	ActMerge Action = iota + 1
	ActClose
	ActReopen
	// ActDraft converts a pull request to a draft or marks it ready.
	ActDraft
	ActComment
	ActLabel
	// ActRerun re-runs a workflow run or some of its jobs.
	ActRerun
	ActCancelRun
	// ActMarkRead marks notifications read or done, which the Gate decides
	// by the token alone.
	ActMarkRead
)

// Gate decides what the viewer may do in one repository, from what GitHub
// said of it and of the issue or pull request at hand, and what the token
// may do. What isn't known yet is allowed, so that GitHub decides, as it
// would anyway.
type Gate struct {
	Repo core.RepoRef
	Caps core.RepoCaps
	// Viewer is the login of the signed-in user, or empty while it is
	// unknown.
	Viewer string
	// Token is what the token may do, or nil, which allows everything.
	Token *Token
}

// Allow reports whether the viewer may take a on it, which is nil for the
// actions on the repository, such as a re-run. When not, why tells the
// user, in a sentence. The repository is asked first, since what it says,
// such as that it is archived, says more than what the token lacks.
func (g Gate) Allow(a Action, it *core.Issue) (ok bool, why string) {
	if ok, why := g.allowRepo(a, it); !ok {
		return false, why
	}
	return g.allowToken(a)
}

// allowToken reports whether the token may take a.
func (g Gate) allowToken(a Action) (ok bool, why string) {
	need, gerund := g.need(a)
	if gerund == "" {
		return true, ""
	}
	if err := g.Token.Check(need); err != nil {
		return false, g.Token.refusal(gerund, err)
	}
	return true, ""
}

// need returns what a needs of the token, and the gerund that names a in
// a sentence, or "" when a needs nothing.
func (g Gate) need(a Action) (need core.Need, gerund string) {
	switch a {
	case ActRerun:
		return core.NeedRuns, "Re-running"
	case ActCancelRun:
		return core.NeedRuns, "Cancelling"
	case ActMarkRead:
		return core.NeedNotifications, "Marking notifications"
	case ActMerge:
		gerund = "Merging"
	case ActClose:
		gerund = "Closing"
	case ActReopen:
		gerund = "Reopening"
	case ActDraft:
		gerund = "Changing a draft"
	case ActComment:
		gerund = "Commenting"
	case ActLabel:
		gerund = "Labeling"
	default:
		return core.Need{}, ""
	}
	return core.NeedWrite(g.Caps), gerund
}

// allowRepo reports whether the repository and it let the viewer take a.
func (g Gate) allowRepo(a Action, it *core.Issue) (ok bool, why string) {
	// Notifications aren't a repository's.
	if a == ActMarkRead {
		return true, ""
	}
	c, repo := g.Caps, g.Repo.String()
	switch {
	case c.Known && c.Archived:
		return false, repo + " is archived, so it's read-only."
	case c.Known && c.Locked:
		return false, repo + " is locked, so it's read-only."
	}
	switch a {
	case ActMerge:
		if !c.CanWrite() {
			return false, g.cannot("merge")
		}
		if _, ok := c.MergeMethod(""); !ok {
			return false, repo + " allows no merge method."
		}
	case ActClose, ActReopen:
		// GitHub says one of them only when the state allows it, so either
		// one says whether the viewer may change the state.
		if it != nil && it.Caps.Known {
			if it.Caps.Close || it.Caps.Reopen {
				return true, ""
			}
			return false, g.cannot(verb(a) + " " + number(it))
		}
		if !c.CanTriage() && !g.authored(it) {
			return false, g.cannot(verb(a) + " " + number(it))
		}
	case ActDraft:
		if it != nil && it.Caps.Known {
			if it.Caps.Update {
				return true, ""
			}
			return false, g.cannot("change " + number(it))
		}
		if !c.CanWrite() && !g.authored(it) {
			return false, g.cannot("change " + number(it))
		}
	case ActComment:
		if it != nil && it.Locked && !c.CanWrite() {
			return false, lockedText(it)
		}
	case ActLabel:
		if it != nil && it.Caps.Known {
			if it.Caps.Label {
				return true, ""
			}
			return false, "Labeling needs triage access to " + repo + "."
		}
		if !c.CanTriage() {
			return false, "Labeling needs triage access to " + repo + "."
		}
	case ActRerun:
		if !c.CanWrite() {
			return false, "Re-running needs write access to " + repo + "."
		}
	case ActCancelRun:
		if !c.CanWrite() {
			return false, "Cancelling needs write access to " + repo + "."
		}
	case ActMarkRead:
	}
	return true, ""
}

// Refuse returns the command that tells the user why they may not take a
// on it, in a toast, and reports true, or reports false when they may.
func (g Gate) Refuse(a Action, it *core.Issue) (tea.Cmd, bool) {
	ok, why := g.Allow(a, it)
	if ok {
		return nil, false
	}
	return Notify(toast.Info, why), true
}

// Gated returns b, disabled when the viewer may not take a on it, so that
// help leaves it out.
func (g Gate) Gated(b key.Binding, a Action, it *core.Issue) key.Binding {
	if ok, _ := g.Allow(a, it); !ok {
		b.SetEnabled(false)
	}
	return b
}

// authored reports whether the viewer opened it, taking an unknown viewer
// for its author.
func (g Gate) authored(it *core.Issue) bool {
	if it == nil {
		return false
	}
	return it.Caps.Authored || g.Viewer == "" || strings.EqualFold(it.Author.Login, g.Viewer)
}

// cannot says that the viewer can't do what in the repository, with the
// access they have if GitHub said.
func (g Gate) cannot(what string) string {
	s := "You can't " + what + " in " + g.Repo.String()
	if p := g.Caps.Permission; g.Caps.Known && p != "" {
		s += " (" + string(p) + " access)"
	}
	return s + "."
}

func verb(a Action) string {
	if a == ActReopen {
		return "reopen"
	}
	return "close"
}

func number(it *core.Issue) string {
	if it == nil {
		return "it"
	}
	return "#" + strconv.Itoa(it.Number)
}

// lockedText says that it is locked, and why if GitHub said.
func lockedText(it *core.Issue) string {
	s := number(it) + " is locked"
	if r := it.LockReason; r != "" {
		s += " as " + strings.ReplaceAll(r, "_", " ")
	}
	return s + " · only collaborators can comment."
}
