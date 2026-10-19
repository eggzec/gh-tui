package files

import (
	"strconv"
	"strings"

	"github.com/eggzec/gh-tui/internal/core"
)

// Finding is what the checks found in one file: how many annotations they left
// on it, at any level, and the worst level among them.
type Finding struct {
	Count int
	Level core.AnnotationLevel
}

// Add returns f with one more annotation of level l.
func (f Finding) Add(l core.AnnotationLevel) Finding {
	if f.Count == 0 || rank(l) > rank(f.Level) {
		f.Level = l
	}
	f.Count++
	return f
}

// rank orders the levels of annotations by how severe they are. A level that
// GitHub adds later counts as a notice.
func rank(l core.AnnotationLevel) int {
	switch l {
	case core.AnnotationFailure:
		return 2
	case core.AnnotationWarning:
		return 1
	case core.AnnotationNotice:
		return 0
	}
	return 0
}

// Findings returns the finding of each path that notes name.
func Findings(notes []core.Annotation) map[string]Finding {
	var out map[string]Finding
	for _, n := range notes {
		// A problem matcher of an Action may name a file as ./a.go.
		p := strings.TrimPrefix(n.Path, "./")
		if p == "" {
			continue
		}
		if out == nil {
			out = map[string]Finding{}
		}
		out[p] = out[p].Add(n.Level)
	}
	return out
}

// badge is the mark of the findings of the file at p: the glyph of their worst
// level and their count, dim. It is "" for a file with none.
func (b *builder) badge(p string) string {
	f, ok := b.findings[p]
	if !ok || f.Count == 0 {
		return ""
	}
	glyph := b.st.notice
	switch f.Level {
	case core.AnnotationFailure:
		glyph = b.st.failure
	case core.AnnotationWarning:
		glyph = b.st.warning
	case core.AnnotationNotice:
	}
	return glyph + " " + b.st.dim.Render(strconv.Itoa(f.Count))
}
