package ui

import (
	"strconv"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/pkg/termtext"
)

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

// CountForms returns the forms of a list count for the top-right of a pane,
// longest first: "total[loaded]", then "total". Under a filter it is
// "matched/total[loaded]", "matched/total" and the bare "matched". The loaded
// and matched numbers are exact; the total is abbreviated (1.2k), and "?"
// stands for a total that is not known. A list with no
// further pages knows its total: what it loaded.
func CountForms(s CountSpec) []string {
	total := "?"
	switch {
	case s.Total >= s.Loaded && s.Total > 0:
		total = Count(s.Total, false)
	case !s.More:
		total = Count(s.Loaded, false)
	}
	loaded := "[" + strconv.Itoa(s.Loaded) + "]"
	if !s.Filtered {
		return []string{total + loaded, total}
	}
	matched := strconv.Itoa(s.Matched)
	head := matched + "/" + total
	return []string{head + loaded, head, matched}
}

// TitleBeside returns the forms of a pane title, longest first, for a frame
// that also shows the count forms in its top-right. The count wins over the
// title: only the forms that leave the shortest count its room and four cells
// besides are kept, and the shortest form cut to that room ends the list.
// Without a count the forms are returned as they are. w is the width the top
// edge leaves the title.
func TitleBeside(forms, count []string, w int, ellipsis string) []string {
	if len(count) == 0 || len(forms) == 0 {
		return forms
	}
	room := max(w-ansi.StringWidth(count[len(count)-1])-4, 0)
	var out []string
	for _, f := range forms {
		if ansi.StringWidth(f) <= room {
			out = append(out, f)
		}
	}
	if cut := termtext.Truncate(forms[len(forms)-1], room, ellipsis); cut != "" && (len(out) == 0 || out[len(out)-1] != cut) {
		out = append(out, cut)
	}
	return out
}

// Counter is implemented by a section that shows a count at the top-right of
// its pane's frame. The forms are longest first; none means no count.
type Counter interface {
	CountForms() []string
}
