// Package mermaid names the kind of a mermaid diagram and links to it on
// mermaid.live, which draws it in the browser. It needs no network: the
// link carries the diagram, compressed, after the # of the address, which
// the browser doesn't send.
package mermaid

import (
	"bytes"
	"compress/zlib"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/eggzec/gh-tui/pkg/termtext"
)

// MaxURL is how long a link is at most, in bytes: as long as a terminal
// takes a hyperlink's address. A longer diagram gets no link.
const MaxURL = termtext.MaxLink

// maxCode is how long the code of a diagram is at most for [ViewURL] to
// compress it. Only code that repeats itself far more than a diagram does
// would make a link short enough, and compressing it would take time.
const maxCode = 64 * MaxURL

// view is where mermaid.live shows a diagram without its editor.
const view = "https://mermaid.live/view#"

// state is what mermaid.live keeps of a diagram, as its defaultState has
// it, with the fields in the order JSON.stringify writes them.
type state struct {
	Code          string `json:"code"`
	Grid          bool   `json:"grid"`
	Mermaid       string `json:"mermaid"` // mermaid's config, as JSON
	PanZoom       bool   `json:"panZoom"`
	Rough         bool   `json:"rough"`
	UpdateDiagram bool   `json:"updateDiagram"`
}

// Encode returns code as mermaid.live's "pako:" payload: its state as
// JSON, compressed with zlib at level 9 and in URL-safe base64 without
// padding, as pako and js-base64 make it.
func Encode(code string) (string, error) {
	var js bytes.Buffer
	enc := json.NewEncoder(&js)
	// JSON.stringify leaves <, > and & as they are, as in "A-->B".
	enc.SetEscapeHTML(false)
	s := state{Code: code, Grid: true, Mermaid: "{}", PanZoom: true, UpdateDiagram: true}
	if err := enc.Encode(s); err != nil {
		return "", fmt.Errorf("encode diagram: %w", err)
	}
	var z bytes.Buffer
	w, err := zlib.NewWriterLevel(&z, zlib.BestCompression)
	if err != nil {
		return "", fmt.Errorf("encode diagram: %w", err)
	}
	if _, err := w.Write(bytes.TrimSuffix(js.Bytes(), []byte("\n"))); err != nil {
		return "", fmt.Errorf("encode diagram: %w", err)
	}
	if err := w.Close(); err != nil {
		return "", fmt.Errorf("encode diagram: %w", err)
	}
	return "pako:" + base64.RawURLEncoding.EncodeToString(z.Bytes()), nil
}

// ViewURL returns the page of mermaid.live that shows the diagram code
// draws, or "" if the link would be longer than [MaxURL].
func ViewURL(code string) string {
	if len(code) > maxCode {
		return ""
	}
	p, err := Encode(code)
	if err != nil || len(view)+len(p) > MaxURL {
		return ""
	}
	return view + p
}

// kinds names the kinds of diagram by the keyword that starts them.
var kinds = map[string]string{
	"graph":              "flowchart",
	"flowchart":          "flowchart",
	"flowchart-elk":      "flowchart",
	"sequenceDiagram":    "sequence diagram",
	"classDiagram":       "class diagram",
	"classDiagram-v2":    "class diagram",
	"stateDiagram":       "state diagram",
	"stateDiagram-v2":    "state diagram",
	"erDiagram":          "ER diagram",
	"gantt":              "Gantt chart",
	"pie":                "pie chart",
	"journey":            "user journey",
	"gitGraph":           "git graph",
	"mindmap":            "mindmap",
	"timeline":           "timeline",
	"quadrantChart":      "quadrant chart",
	"requirementDiagram": "requirement diagram",
	"C4Context":          "C4 diagram",
	"C4Container":        "C4 diagram",
	"C4Component":        "C4 diagram",
	"C4Dynamic":          "C4 diagram",
	"C4Deployment":       "C4 diagram",
	"sankey":             "Sankey diagram",
	"sankey-beta":        "Sankey diagram",
	"xychart":            "XY chart",
	"xychart-beta":       "XY chart",
	"block":              "block diagram",
	"block-beta":         "block diagram",
	"packet":             "packet diagram",
	"packet-beta":        "packet diagram",
	"architecture":       "architecture diagram",
	"architecture-beta":  "architecture diagram",
	"kanban":             "kanban board",
	"radar-beta":         "radar chart",
	"zenuml":             "ZenUML diagram",
	"treemap-beta":       "treemap",
}

// Kind returns the kind of diagram code draws, such as "sequence
// diagram", from the keyword it starts with, or "diagram" if it starts
// with none it knows. It is always one of a few fixed names, never text
// of code.
func Kind(code string) string {
	lines := strings.Split(code, "\n")
	first := true
	for i := 0; i < len(lines); i++ {
		l := strings.TrimSpace(lines[i])
		switch {
		case l == "", strings.HasPrefix(l, "%%"):
			// Blank lines, comments and directives such as %%{init: …}%%.
			continue
		case l == "---" && first:
			// Front matter, such as a title or a config.
			end := slices.IndexFunc(lines[i+1:], func(l string) bool { return strings.TrimSpace(l) == "---" })
			if end < 0 {
				return "diagram"
			}
			i += end + 1
			first = false
			continue
		}
		word, _, _ := strings.Cut(strings.Fields(l)[0], ";")
		if k, ok := kinds[word]; ok {
			return k
		}
		return "diagram"
	}
	return "diagram"
}
