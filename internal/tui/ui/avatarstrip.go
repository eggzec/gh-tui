package ui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// Person is someone shown by an avatar, or by their login where avatars
// are not drawn.
type Person struct{ Login, AvatarURL string }

// AvatarStrip is the avatars of users side by side on one line, as many as
// fit in width cells, and more(n) for the n that do not, which it keeps room
// for. Where avatars are not shown it is the @login of each instead, under
// the same rules. An avatar that is not loaded, or failed, is blank. The
// avatars take the first row of size, so size.Rows is 1 for a one-line strip.
func AvatarStrip(imgs *Images, users []Person, size ImageSize, width int, more func(n int) string) string {
	cells := make([]string, len(users))
	widths := make([]int, len(users))
	for i, u := range users {
		if imgs.Shown() {
			if rows := imgs.Box(SizedAvatar(u.AvatarURL, size), size); len(rows) > 0 {
				cells[i] = rows[0]
			}
			widths[i] = max(size.Cols, 1)
			continue
		}
		cells[i] = "@" + u.Login
		widths[i] = ansi.StringWidth(cells[i])
	}
	n := len(users)
	for k := n; k >= 0; k-- {
		need := max(k-1, 0)
		for _, w := range widths[:k] {
			need += w
		}
		var rest string
		if k < n {
			rest = more(n - k)
			need += ansi.StringWidth(rest)
			if k > 0 {
				need++
			}
		}
		if need > width && k > 0 {
			continue
		}
		line := strings.Join(cells[:k], " ")
		if rest != "" {
			if k > 0 {
				line += " "
			}
			line += rest
		}
		return line
	}
	return ""
}
