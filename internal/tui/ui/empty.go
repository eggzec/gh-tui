package ui

import "charm.land/bubbles/v2/key"

// None words a list with nothing in it, named by what it would hold, such
// as "No open pull requests.".
func None(what string) string {
	return "No " + what + "."
}

// NoMatch words a list the filters leave empty, such as "No issues match
// the filters. Press c to clear them.", without the hint when clearKey is
// unbound.
func NoMatch(what, clearKey string) string {
	return Press("No "+what+" match the filters.", clearKey, "clear them")
}

// Press adds to text the key that does something, such as "Press ] to
// show closed ones.", or leaves text as it is when the key is unbound.
func Press(text, k, does string) string {
	if k == "" {
		return text
	}
	return text + " Press " + k + " to " + does + "."
}

// KeyOf is the key of b as its help names it, for Press, or empty while b
// is off.
func KeyOf(b key.Binding) string {
	if !b.Enabled() {
		return ""
	}
	return b.Help().Key
}
