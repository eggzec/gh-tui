package logview

import "slices"

// issues returns the rows of errors or warnings.
func (m *Model) issues(k Kind) []int {
	if k == Warning {
		return m.warns
	}
	return m.errs
}

// stepIssue moves the cursor to the next error or warning after it, or the
// previous one before it for a negative d, wrapping around at the ends. It
// expands the folds the line is in.
func (m *Model) stepIssue(k Kind, d int) {
	list := m.issues(k)
	r := m.cursorRow()
	if len(list) == 0 || r < 0 {
		return
	}
	var i int
	if d > 0 {
		// The first after the cursor, including those a collapsed fold
		// under it hides.
		i, _ = slices.BinarySearch(list, r+1)
		if i == len(list) {
			i = 0
		}
	} else {
		i, _ = slices.BinarySearch(list, r)
		i = (i - 1 + len(list)) % len(list)
	}
	m.goToIssue(k, i)
}

// goToIssue moves the cursor to error or warning i.
func (m *Model) goToIssue(k Kind, i int) {
	m.jumped, m.at = k, i
	m.reveal(m.issues(k)[i])
}
