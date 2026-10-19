package files

import (
	"slices"
	"strconv"
	"strings"

	"github.com/eggzec/gh-tui/internal/core"
)

// Finding is what the checks found in one file: how many annotations they left
// on it, at any level, and the worst level among them.
type Finding struct {
	Count int
	Level core.AnnotationLevel
	// Notes are the annotations that sit on a line, which the diff shows under
	// it. An annotation of the file as a whole is counted and not listed.
	Notes []Note
}

// Note is one annotation of a check run, on the last line it names.
type Note struct {
	Line    int
	Level   core.AnnotationLevel
	App     string
	Message string
}

// Add returns f with one more annotation of level l.
func (f Finding) Add(l core.AnnotationLevel) Finding {
	if f.Count == 0 || rank(l) > rank(f.Level) {
		f.Level = l
	}
	f.Count++
	return f
}

// ByLine returns the notes of f by the line they are on, each line's worst
// level first, then by the name of the app.
func (f Finding) ByLine() map[int][]Note {
	if len(f.Notes) == 0 {
		return nil
	}
	out := map[int][]Note{}
	for _, n := range f.Notes {
		out[n.Line] = append(out[n.Line], n)
	}
	for _, ns := range out {
		slices.SortStableFunc(ns, func(a, b Note) int {
			if c := rank(b.Level) - rank(a.Level); c != 0 {
				return c
			}
			return strings.Compare(a.App, b.App)
		})
	}
	return out
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
	return AddFindings(nil, "", notes)
}

// AddFindings adds to out, which may be nil, the annotations notes of a check
// run of the app named app, and returns it. The notes of a line keep the app's
// name and the first line of their message.
func AddFindings(out map[string]Finding, app string, notes []core.Annotation) map[string]Finding {
	for _, n := range notes {
		// A problem matcher of an Action may name a file as ./a.go.
		p := strings.TrimPrefix(n.Path, "./")
		if p == "" {
			continue
		}
		if out == nil {
			out = map[string]Finding{}
		}
		f := out[p].Add(n.Level)
		if line := n.EndLine; line > 0 {
			msg, _, _ := strings.Cut(strings.TrimSpace(n.Message), "\n")
			f.Notes = append(slices.Clip(f.Notes), Note{Line: line, Level: n.Level, App: app, Message: strings.TrimSpace(msg)})
		}
		out[p] = f
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
