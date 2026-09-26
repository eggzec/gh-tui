package ui

// NumberWidth is the width of the number column in the lists of issues and
// pull requests, such as "#12345". Both lists start a row with the glyph of
// the state and a space, then the number left-aligned in this width and a
// space, so their titles line up.
const NumberWidth = 6

// NumberOver returns how many cells num, such as "#123", takes past
// NumberWidth. A row takes those from its title, so it keeps its width and
// the space after the number.
func NumberOver(num string) int {
	return max(len(num)-NumberWidth, 0)
}
