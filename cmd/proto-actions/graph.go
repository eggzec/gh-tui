package main

import (
	"fmt"
	"strings"

	"github.com/AlexanderGrooff/mermaid-ascii/pkg/diagram"
	"github.com/AlexanderGrooff/mermaid-ascii/pkg/render"
	"github.com/charmbracelet/x/ansi"
)

var marks = map[string]string{"success": "✓", "failure": "✗", "running": "…", "queued": "·", "cancelled": "⊘", "skipped": "-"}

// graphNode is one node of the dependency graph: a job, or a whole matrix.
type graphNode struct {
	id, label, status string
	needs             []string
}

// graphNodes combines each matrix into one node named after its parent.
func (r run) graphNodes() []graphNode {
	var out []graphNode
	idx := map[string]int{}
	for _, j := range r.jobs {
		key := j.name
		if j.group != "" {
			key = j.group
		}
		if _, ok := idx[key]; ok {
			continue
		}
		idx[key] = len(out)
		n := graphNode{id: fmt.Sprint("n", len(out)), label: key, status: j.status, needs: j.needs}
		if j.group != "" {
			st, _, total := r.rollup(j.group)
			n.status = st
			n.label = fmt.Sprintf("%s (%d)", key, total)
		}
		out = append(out, n)
	}
	// needs name keys; turn them into ids
	for i := range out {
		var ids []string
		for _, need := range out[i].needs {
			if k, ok := idx[need]; ok {
				ids = append(ids, out[k].id)
			}
		}
		out[i].needs = ids
	}
	return out
}

// mermaid is the flowchart source of the run's jobs.
func (r run) mermaid(dir string) string {
	var b strings.Builder
	b.WriteString("flowchart " + dir + "\n")
	ns := r.graphNodes()
	for _, n := range ns {
		fmt.Fprintf(&b, "    %s[%s %s]\n", n.id, n.label, marks[n.status])
	}
	for _, n := range ns {
		for _, need := range n.needs {
			fmt.Fprintf(&b, "    %s --> %s\n", need, n.id)
		}
	}
	return b.String()
}

// graphLines renders the run's graph as text lines.
func (m *model) graphLines(r run) []string {
	key := fmt.Sprint(r.id, m.gdir)
	if l, ok := m.gcache[key]; ok {
		return l
	}
	cfg := diagram.DefaultConfig()
	cfg.GraphDirection = m.gdir
	cfg.PaddingBetweenX, cfg.PaddingBetweenY = 3, 1
	out, err := render.RenderDiagram(r.mermaid(m.gdir), cfg)
	var lines []string
	if err != nil {
		lines = []string{m.th.Error.Render("cannot draw: " + err.Error())}
	} else {
		for _, l := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
			l = strings.ReplaceAll(l, marks["failure"], m.th.Error.Render(marks["failure"]))
			l = strings.ReplaceAll(l, marks["success"], m.th.Success.Render(marks["success"]))
			lines = append(lines, l)
		}
	}
	if m.gcache == nil {
		m.gcache = map[string][]string{}
	}
	m.gcache[key] = lines
	return lines
}

// graphBody shows the drawing centred when it fits, else scrolled by gx, gy.
func (m *model) graphBody(r run, w, h int) string {
	var lines []string
	if m.gsrc {
		for _, l := range strings.Split(strings.TrimRight(r.mermaid(m.gdir), "\n"), "\n") {
			lines = append(lines, m.th.Text.Render(l))
		}
	} else {
		lines = m.graphLines(r)
	}
	gw := 0
	for _, l := range lines {
		gw = max(gw, ansi.StringWidth(l))
	}
	m.gx = max(0, min(max(0, gw-w), m.gx))
	m.gy = max(0, min(max(0, len(lines)-h), m.gy))
	padTop := max(0, (h-len(lines))/2)
	padLeft := max(0, (w-gw)/2)
	if m.gsrc {
		padLeft, padTop = 0, 0
	}
	var out []string
	for range padTop {
		out = append(out, "")
	}
	for _, l := range lines[min(m.gy, len(lines)):] {
		out = append(out, strings.Repeat(" ", padLeft)+ansi.Cut(l, m.gx, m.gx+w))
	}
	if len(out) > h {
		out = out[:h]
	}
	return strings.Join(out, "\n")
}
