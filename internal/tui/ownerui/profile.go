package ownerui

import "github.com/eggzec/gh-tui/internal/tui/ui"

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
		lines[i] = ui.Fit("", w)
	}
	return lines
}
