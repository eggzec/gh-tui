package panes

import "strings"

// Hint is one key hint of a footer: its text and how much it matters.
type Hint struct {
	Text string
	// Rank orders what is dropped when the hints do not fit: the lowest rank
	// goes first, and of equal ranks the last one.
	Rank int
}

// HintForms are the forms of a hint line for the BottomLeft slot, from the
// longest to the shortest: all the hints joined by sep, then one fewer at a
// time, dropping the lowest rank first (of equal ranks, the last) and keeping
// the order of those left. Hints with no text are ignored, and no form is
// empty, so the list is nil when there is no hint.
func HintForms(hints []Hint, sep string) []string {
	kept := make([]Hint, 0, len(hints))
	for _, h := range hints {
		if h.Text != "" {
			kept = append(kept, h)
		}
	}
	forms := make([]string, 0, len(kept))
	for len(kept) > 0 {
		texts := make([]string, len(kept))
		for i, h := range kept {
			texts[i] = h.Text
		}
		forms = append(forms, strings.Join(texts, sep))
		drop := 0
		for i, h := range kept {
			if h.Rank <= kept[drop].Rank {
				drop = i
			}
		}
		kept = append(kept[:drop:drop], kept[drop+1:]...)
	}
	return forms
}
