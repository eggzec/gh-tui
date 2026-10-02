package toast

import (
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/pkg/termtext"
)

// Room is how much of the area a toast may take.
type Room struct {
	// Share is the most of the width, in percent, that the toast may take.
	Share int
	// Lines is how many lines its text may wrap to before it is cut.
	Lines int
}

// DefaultRoom returns the room of a level: 40% of the width and three
// lines, and for errors 60% and five lines, since an error often says what
// to do about it and that must show whole.
func DefaultRoom(level Level) Room {
	if level == Error {
		return Room{Share: 60, Lines: 5}
	}
	return Room{Share: 40, Lines: 3}
}

// valid returns r with a share from 1 to 100 and at least one line.
func (r Room) valid() Room {
	return Room{Share: min(max(r.Share, 1), 100), Lines: max(r.Lines, 1)}
}

func defaultRooms() [Error + 1]Room {
	var rooms [Error + 1]Room
	for l := Info; l <= Error; l++ {
		rooms[l] = DefaultRoom(l)
	}
	return rooms
}

// Room returns the room of a level.
func (m Model) Room(level Level) Room {
	if !level.valid() {
		level = Info
	}
	return m.rooms[level]
}

// SetRoom sets the room of a level. A share is kept from 1 to 100, and
// lines to at least one.
func (m *Model) SetRoom(level Level, r Room) {
	if !level.valid() {
		return
	}
	m.rooms[level] = r.valid()
	m.changed()
}

// maxCount is the most a toast counts its repeats, so that the room Fits
// keeps for the count is always enough.
const maxCount = 99

// Fits reports whether text shows whole in a toast of level at the
// current size, however often it repeats, so that the parent can shorten
// what it says until it does.
func (m Model) Fits(level Level, text string) bool {
	if !level.valid() {
		level = Info
	}
	d := m.derived
	width := m.maxInner(level) - d.glyphWidth - 1 - m.countWidth(maxCount)
	if width < 1 {
		return false
	}
	lines := m.Room(level).Lines
	if m.height > 0 {
		// The frame takes lines of the height too.
		lines = min(lines, m.height-d.frameHeight)
		if lines < 1 {
			return false
		}
	}
	_, cut := wrap(clean(text), width, lines, m.styles.Ellipsis)
	return !cut
}

// maxInner is the widest the content of a toast of level may be: its
// share of the width, but no narrower than minWidth and never wider than
// the width within the inset on both sides, less the frame.
func (m Model) maxInner(level Level) int {
	block := minWidth
	if m.width > 0 {
		block = min(max(m.width*m.Room(level).Share/100, minWidth), m.width-2*m.inset[0])
	}
	return block - m.derived.frameWidth
}

// wrap wraps text to width cells and at most lines lines, and reports
// whether it had to cut the text, which then ends in ellipsis. A word
// longer than the width is broken.
func wrap(text string, width, lines int, ellipsis string) ([]string, bool) {
	wrapped := fit(strings.Split(ansi.Wrap(text, width, ""), "\n"), width)
	if len(wrapped) <= lines {
		return wrapped, false
	}
	wrapped = wrapped[:lines]
	last := strings.TrimRight(wrapped[lines-1], " ")
	wrapped[lines-1] = termtext.Truncate(last+" "+ellipsis, width, ellipsis)
	return wrapped, true
}

// fit returns lines with no trailing spaces, each at most width cells,
// breaking any that is wider and dropping any left empty. [ansi.Wrap] can
// leave a line wider than the width, such as one that ends at a hyphen,
// and the count after it would then stick out of the toast; it can also
// end with an empty line, which would count against the toast's lines.
func fit(lines []string, width int) []string {
	out := make([]string, 0, len(lines))
	keep := func(l string) {
		if l = strings.TrimRight(l, " "); l != "" {
			out = append(out, l)
		}
	}
	for _, l := range lines {
		if ansi.StringWidth(strings.TrimRight(l, " ")) <= width {
			keep(l)
			continue
		}
		for h := range strings.SplitSeq(ansi.Hardwrap(l, width, false), "\n") {
			keep(h)
		}
	}
	return out
}
