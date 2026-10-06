package pager

// Prompting reports whether the search or filter prompt is open, which
// is what Capturing means unless the pager waits for the name of an option
// or for the key after a count.
func (m Model) Prompting() bool { return m.prompt.Focused() }

// ChoosingOption reports whether the pager waits for the name of an
// option, after the option key.
func (m Model) ChoosingOption() bool { return m.opt }
