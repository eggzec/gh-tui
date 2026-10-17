package conv

import (
	"strings"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/pkg/termtext"
)

// ghost is who GitHub names for an account that was deleted.
const ghost = "ghost"

// Name returns how u is named: its name when it has one, else its login,
// without an @, and ghost for a deleted account. It is safe to draw.
func Name(u core.User) string {
	s, _ := label(u)
	return s
}

// label returns the name of u and whether it is a deleted account.
func label(u core.User) (name string, gone bool) {
	for _, s := range []string{u.Name, u.Login} {
		if s = strings.TrimSpace(termtext.OneLine(s)); s != "" {
			return s, false
		}
	}
	return ghost, true
}

// SubjectName returns how the person or the team that an event is about is
// named: a team reads as its slug and "team".
func SubjectName(s core.TimelineSubject) string {
	if s.Team == "" {
		return Name(s.User)
	}
	return teamSlug(s.Team) + " team"
}

// teamSlug returns the name of a team without its organization.
func teamSlug(team string) string {
	if i := strings.LastIndexByte(team, '/'); i >= 0 {
		team = team[i+1:]
	}
	return strings.TrimSpace(termtext.OneLine(team))
}
