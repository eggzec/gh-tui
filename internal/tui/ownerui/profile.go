package ownerui

import (
	"cmp"
	"strings"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// ProfileLines is how many lines the profile takes without an avatar.
const ProfileLines = 2

// MinAvatarWidth is the narrowest page that shows the avatar: room for its
// box, the space before it and some of the profile beside it.
const MinAvatarWidth = 1 + 6 + 20

// ProfileHeight is how many lines the profile takes: those of the avatar's
// box when the avatar is drawn, whether it has arrived or not, so that
// nothing moves when it does.
func ProfileHeight(avatar bool) int {
	if avatar {
		return max(ProfileLines, ui.AvatarLarge.Rows)
	}
	return ProfileLines
}

// BlankProfile is the profile of a page too narrow to show any of it: n
// lines of w spaces.
func BlankProfile(w, n int) []string {
	lines := make([]string, n)
	for i := range lines {
		lines[i] = Fit("", w)
	}
	return lines
}

// NameLine renders the first line of the profile: the name, login and bio
// of p.
func (d Drawer) NameLine(p core.Profile) string {
	st := d.Styles
	line := st.Name.Render(CleanLine(cmp.Or(p.Name, p.Login))) + " " + st.Login.Render("@"+CleanLine(p.Login))
	if p.Bio != "" {
		line += st.Subtle.Render(d.Icons.Separator) + st.Text.Render(CleanLine(p.Bio))
	}
	return line
}

// Facts renders the second line of the profile: the company, location,
// follows and status of p.
func (d Drawer) Facts(p core.Profile) string {
	st := d.Styles
	parts := make([]string, 0, 6)
	for _, t := range []string{p.Company, p.Location} {
		if t = CleanLine(t); t != "" {
			parts = append(parts, st.Muted.Render(t))
		}
	}
	parts = append(parts,
		st.Text.Render(Count(p.Followers))+st.Muted.Render(Plural(p.Followers, " follower", " followers")),
		st.Text.Render(Count(p.Following))+st.Muted.Render(" following"),
	)
	if m := CleanLine(p.Status.Message); m != "" || p.Status.Busy {
		status := m
		if p.Status.Busy {
			status = strings.TrimSpace("busy " + status)
		}
		parts = append(parts, st.Accent.Render(status))
	}
	return strings.Join(parts, st.Subtle.Render(d.Icons.Separator))
}

// Profile lays the profile out on n lines: first, and second with right at
// its end, in w cells after the avatar's box, which goes first on each line
// after a space, outside any style, since its cells name the image by
// their color. Lines that don't fit end in an ellipsis.
func (d Drawer) Profile(box []string, first, second, right string, w, n int) []string {
	tail := d.Icons.Ellipsis
	lines := []string{Spread(" "+first, "", w, tail), Spread(" "+second, right+" ", w, tail)}
	for len(lines) < n {
		lines = append(lines, Fit("", w))
	}
	for i := range box {
		lines[i] = " " + box[i] + lines[i]
	}
	return lines
}
