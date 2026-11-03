package ui

// CountSpec describes what a list pane knows about its rows, for CountForms.
type CountSpec struct {
	// Total is the number of rows the source holds, 0 when unknown. A total
	// below Loaded is treated as unknown: the pages outgrew it.
	Total int
	// Loaded is the number of rows fetched so far.
	Loaded int
	// Matched is the number of loaded rows a filter keeps; read only when
	// Filtered is set.
	Matched int
	// More reports that further pages follow.
	More bool
	// Filtered reports an active filter.
	Filtered bool
}

// CountForms returns the forms of a list count for the right of a pane
// title, longest first: "all [loaded]" and the bare "all" when the total is
// known; the loaded count alone otherwise ("+" while more pages follow). Under
// a filter it is "matched/all", with "[loaded]" only when that differs from
// matched, and the bare matched count as the narrowest form.
func CountForms(s CountSpec) []string {
	loaded := Count(s.Loaded, s.More)
	total := s.Total
	if total < s.Loaded {
		total = 0
	}
	if !s.Filtered {
		if total == 0 {
			return []string{loaded}
		}
		all := Count(total, false)
		if s.Loaded == total {
			return []string{all}
		}
		return []string{all + " [" + loaded + "]", all}
	}
	matched := Count(s.Matched, false)
	if total == 0 {
		if s.Matched == s.Loaded {
			return []string{matched}
		}
		return []string{matched + " [" + loaded + "]", matched}
	}
	head := matched + "/" + Count(total, false)
	if s.Matched == s.Loaded {
		return []string{head, matched}
	}
	return []string{head + " [" + loaded + "]", head, matched}
}
