package filterform

import "strings"

// SortOf returns the sort that query q holds, as the form would show it: the
// last sort: token that names an option of the spec's sort, else what an
// empty query shows, which is the default, or no sort for a sort that has an
// option with no value. A spec without a sort has none.
func (s Spec) SortOf(q string) Sort {
	if s.Sort == nil {
		return Sort{}
	}
	return parse(&s, q).sort
}

// WithSort returns q with its sort: tokens that name an option of the spec's
// sort replaced by the token of so, or by none when so has no value. The
// other words stay as they are, in their order. A spec without a sort leaves
// q as it is.
func (s Spec) WithSort(q string, so Sort) string {
	if s.Sort == nil {
		return q
	}
	var words []string
	for _, tok := range Tokenize(q) {
		if _, ok := claimSort(s.Sort, tok); !ok {
			words = append(words, tok.Raw)
		}
	}
	if w := writeSort(so); w != "" {
		words = append(words, w)
	}
	return strings.Join(words, " ")
}
