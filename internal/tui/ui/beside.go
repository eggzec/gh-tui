package ui

// Beside appends to lines those of left, each with the line of right
// beside it.
func Beside(lines, left, right []string) []string {
	for i, l := range left {
		if i < len(right) {
			l += right[i]
		}
		lines = append(lines, l)
	}
	return lines
}

// Indent puts a space before each of lines, as the lines of a pane start.
func Indent(lines []string) []string {
	for i := range lines {
		lines[i] = " " + lines[i]
	}
	return lines
}
