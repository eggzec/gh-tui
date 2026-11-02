package panes

import "strconv"

// Position are the forms of a scroll mark for the BottomRight slot, from the
// longest to the shortest: "a-b/total", "b/total" and a percentage, for the
// rows top+1 to top+shown of total (top counts from 0). more adds a "+" after
// the total when the list has more rows to load than total says. It is nil
// when everything is on view and nothing more is coming.
func Position(top, shown, total int, more bool) []string {
	if shown >= total && !more {
		return nil
	}
	plus := ""
	if more {
		plus = "+"
	}
	last := min(top+shown, total)
	t := strconv.Itoa(total) + plus
	pct := 100
	if total > 0 {
		pct = last * 100 / total
	}
	return []string{
		strconv.Itoa(top+1) + "-" + strconv.Itoa(last) + "/" + t,
		strconv.Itoa(last) + "/" + t,
		strconv.Itoa(pct) + "%",
	}
}
