package jobview

import (
	"time"

	"github.com/eggzec/gh-tui/internal/core"
)

// FirstError returns the index of the line of lines that the view of a failed
// job opens on: the first error of the first step that failed, or the head of
// that step when it wrote none, or, when no step failed or the steps aren't
// known, the first error of the log. ok is false when there is none.
func FirstError(lines []core.LogLine, steps []core.Step) (line int, ok bool) {
	for _, s := range stepSections(lines, steps, time.Time{}) {
		if !s.Failed {
			continue
		}
		for i := s.Start; i < min(s.End, len(lines)); i++ {
			if lines[i].Kind == core.LogError {
				return i, true
			}
		}
		return s.Start, true
	}
	for i, ln := range lines {
		if ln.Kind == core.LogError {
			return i, true
		}
	}
	return 0, false
}
