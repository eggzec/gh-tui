package main

import (
	"fmt"
	"os"
	"slices"
	"strings"

	"charm.land/bubbles/v2/filepicker"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/pkg/bubbles/panes"
)

// The overlays work as the notifications filter does: two panes in one
// frame, the checked rows of the first feeding the second.

// oitem is one row of an overlay pane.
type oitem struct {
	label, field string
	on           bool
}

// opane is one list of an overlay.
type opane struct {
	items []oitem
	cur   int
	q     string // search
}

// vis is the indexes of the rows the search keeps.
func (p opane) vis() []int {
	var out []int
	for i, it := range p.items {
		if p.q == "" || strings.Contains(strings.ToLower(it.label), strings.ToLower(p.q)) {
			out = append(out, i)
		}
	}
	return out
}

// at is the item under the cursor, or -1.
func (p *opane) at() int {
	v := p.vis()
	if len(v) == 0 {
		return -1
	}
	p.cur = max(0, min(p.cur, len(v)-1))
	return v[p.cur]
}

func (m *model) openOv(kind int) {
	m.ov, m.ofocus, m.osearch = kind, 0, false
	m.op = [2]opane{}
	switch kind {
	case 1:
		for i, g := range groups {
			m.op[0].items = append(m.op[0].items, oitem{label: g})
			if g == m.groupBy {
				m.op[0].cur = i
			}
		}
	case 2:
		for _, f := range facets {
			m.op[0].items = append(m.op[0].items, oitem{label: f, field: f, on: len(m.flt[f]) > 0})
		}
	case 3:
		for i, s := range sorts {
			m.op[0].items = append(m.op[0].items, oitem{label: s})
			if s == m.sortBy {
				m.op[0].cur = i
			}
		}
	}
	m.refeed()
}

// refeed rebuilds the second pane from the first one's selection.
func (m *model) refeed() {
	old := m.op[1]
	var items []oitem
	p0 := &m.op[0]
	switch m.ov {
	case 1:
		if i := p0.at(); i >= 0 {
			g := groups[i]
			if g == "none" {
				items = append(items, oitem{label: "runs are not grouped"})
			}
			if g != "none" {
				for _, v := range facetValuesBy(g) {
					items = append(items, oitem{label: v})
				}
			}
		}
	case 2:
		var fs []string
		for _, it := range p0.items {
			if it.on {
				fs = append(fs, it.field)
			}
		}
		if i := p0.at(); len(fs) == 0 && i >= 0 {
			fs = append(fs, p0.items[i].field)
		}
		for _, f := range fs {
			for _, v := range facetValues(f)[1:] {
				on := slices.Contains(m.flt[f], v)
				if j := slices.IndexFunc(old.items, func(o oitem) bool { return o.field == f && o.label == v }); j >= 0 {
					on = old.items[j].on
				}
				items = append(items, oitem{label: f + ": " + v, field: f, on: on})
			}
		}
	case 3:
		items = []oitem{{label: "as listed"}, {label: "reversed", on: m.sortRev}}
		if len(old.items) > 0 {
			items[1].on = old.items[1].on
		}
	}
	m.op[1] = opane{items: items, cur: old.cur, q: old.q}
	m.op[1].at()
}

// facetValuesBy is the values a grouping has, from its facet.
func facetValuesBy(g string) []string {
	f := g
	switch g {
	case "status", "branch", "event", "actor", "workflow":
	default:
		return nil
	}
	return facetValues(f)[1:]
}

// overlayKey handles a key in the group, filter or sort overlay.
func (m *model) overlayKey(k string) {
	p := &m.op[m.ofocus]
	if m.osearch {
		switch k {
		case "esc":
			m.osearch, p.q = false, ""
		case "enter":
			m.osearch = false
		case "backspace":
			if p.q == "" {
				m.osearch = false
			} else {
				p.q = p.q[:len(p.q)-1]
			}
		default:
			if len(k) == 1 {
				p.q += k
			}
		}
		p.cur = 0
		m.refeed()
		return
	}
	switch k {
	case "j", "down":
		p.cur++
		p.at()
	case "k", "up":
		p.cur = max(0, p.cur-1)
	case "/":
		m.osearch = true
	case "space", " ":
		m.toggle()
	case "tab", "l", "right":
		if m.ofocus == 0 {
			m.ofocus = 1
		}
	case "shift+tab", "h", "left":
		m.ofocus = 0
	case "backspace":
		if m.ofocus == 1 {
			m.ofocus = 0
		} else {
			m.ov = 0
		}
	case "esc":
		if p.q != "" {
			p.q = ""
			break
		}
		if slices.ContainsFunc(p.items, func(o oitem) bool { return o.on }) && m.ov != 3 {
			for i := range p.items {
				p.items[i].on = false
			}
			break
		}
		m.ov = 0
	case "enter":
		if m.ov == 1 || m.ofocus == 1 {
			m.apply()
			return
		}
		m.ofocus = 1
	}
	if m.ofocus == 0 {
		m.refeed()
	}
}

// toggle checks the row under the cursor.
func (m *model) toggle() {
	p := &m.op[m.ofocus]
	i := p.at()
	if i < 0 || m.ov == 1 {
		return
	}
	if m.ov == 3 && m.ofocus == 0 {
		return
	}
	p.items[i].on = !p.items[i].on
}

// apply writes the overlay's choice to the model and closes it.
func (m *model) apply() {
	switch m.ov {
	case 1:
		if i := m.op[0].at(); i >= 0 {
			m.groupBy, m.groupOff = groups[i], false
		}
	case 2:
		m.flt = map[string][]string{}
		for _, it := range m.op[1].items {
			if it.on {
				v := strings.TrimPrefix(it.label, it.field+": ")
				m.flt[it.field] = append(m.flt[it.field], v)
			}
		}
		m.fltOff = false
	case 3:
		if i := m.op[0].at(); i >= 0 {
			m.sortBy = sorts[i]
		}
		m.sortRev = len(m.op[1].items) > 1 && m.op[1].items[1].on
	}
	m.ov, m.sel = 0, 0
}

// overlayHint is the foot while an overlay is open.
func (m *model) overlayHint() []string {
	if m.osearch {
		return []string{"type to search", "enter keep", "esc clear"}
	}
	h := []string{"j k move", "/ search"}
	if m.ov != 1 {
		h = append(h, "space toggle", "tab next pane")
	}
	return append(h, "enter apply", "backspace back", "esc clear/close")
}

// overlay draws the group, filter or sort panes over the screen.
func (m *model) overlay(base string) string {
	if m.ov == 4 {
		return m.pickerFrame(base)
	}
	names := [][2]string{{"Group by", "Groups"}, {"Fields", "Values"}, {"Sort by", "Direction"}}[m.ov-1]
	if m.ov == 1 {
		names[1] = "Preview"
	}
	ow := min(m.w, 76)
	lw := ow / 2
	rows := 4
	for _, p := range m.op {
		rows = max(rows, len(p.items))
	}
	oh := min(max(m.h-4, 6), rows+3)
	l := panes.Layout{Width: ow, Height: oh, Focus: panes.Pane(m.ofocus + 1),
		Panes: []panes.Def{{Name: names[0]}, {Name: names[1]}},
		Boxes: []panes.Box{
			{Pane: 1, Rect: panes.Rect{W: lw, H: oh}},
			{Pane: 2, Rect: panes.Rect{X: lw, W: ow - lw, H: oh}},
		}}
	box := m.frames.Draw(l, func(b panes.Box) panes.Content {
		i := int(b.Pane) - 1
		p := m.op[i]
		c := panes.Content{TopLeft: []string{l.Title(b.Pane)}, Focus: b.Pane == l.Focus}
		in := b.Rect.Inner()
		n := 0
		for _, it := range p.items {
			if it.on {
				n++
			}
		}
		if m.ov != 1 && (m.ov == 2 || i == 0) {
			c.TopRight = []string{fmt.Sprintf("%d of %d", n, len(p.items)), ""}
		}
		if p.q != "" || (m.osearch && m.ofocus == i) {
			c.BottomLeft = []string{"/" + p.q, ""}
		}
		var lines []string
		sel := p.cur
		v := p.vis()
		for j, idx := range v {
			it := p.items[idx]
			mark := ""
			switch {
			case m.ov == 1 && i == 0, m.ov == 3 && i == 0:
				mark = "  "
				if it.label == m.groupBy && m.ov == 1 || it.label == m.sortBy && m.ov == 3 {
					mark = m.th.Accent.Render(m.ic.Cursor) + " "
				}
			case !(m.ov == 1 && i == 1):
				mark = m.th.Subtle.Render("[ ]") + " "
				if it.on {
					mark = m.th.Accent.Render("[x]") + " "
				}
			}
			txt := m.th.Text.Render(it.label)
			if j == sel && b.Pane == l.Focus {
				txt = m.th.Title.Render(it.label)
			}
			lines = append(lines, m.cursor(j == sel, b.Pane == l.Focus)+mark+txt)
		}
		if len(lines) == 0 {
			lines = []string{m.th.Subtle.Render("Nothing to show")}
		}
		for j := range lines {
			lines[j] = ansi.Truncate(lines[j], in.W, m.ic.Ellipsis)
		}
		c.Body = strings.Join(window(lines, sel, in.H), "\n")
		return c
	})
	return m.place(base, box, ow)
}

// place draws box centred over base.
func (m *model) place(base, box string, ow int) string {
	bl := strings.Split(box, "\n")
	lines := strings.Split(base, "\n")
	x := (m.w - ow) / 2
	y := max(0, (len(lines)-len(bl))/2)
	for i, b := range bl {
		if y+i >= len(lines) {
			break
		}
		old := lines[y+i]
		lines[y+i] = sealed(ansi.Truncate(old, x, ""), x) + b + ansi.Cut(old, x+ow, m.w)
	}
	return strings.Join(lines, "\n")
}

// openPicker opens the directory picker at the download directory, which the
// prototype fakes as the home directory.
func (m *model) openPicker() {
	home, _ := os.UserHomeDir()
	m.fp = filepicker.New()
	m.fp.CurrentDirectory = home
	m.fp.DirAllowed, m.fp.FileAllowed = true, false
	m.fp.SetHeight(max(4, min(m.h-8, 14)))
	m.ov = 4
	m.pend = append(m.pend, m.fp.Init())
}

// pickerKey handles a key while the directory picker is open: c chooses the
// directory shown, esc cancels, the rest is the picker's.
func (m *model) pickerKey(msg tea.KeyPressMsg) tea.Cmd {
	switch msg.String() {
	case "esc", "q":
		m.ov = 0
		return nil
	case "c":
		m.ov = 0
		m.dldir = strings.Replace(m.fp.CurrentDirectory, homeDir(), "~", 1)
		m.confirm = m.dwhat
		return nil
	}
	var cmd tea.Cmd
	m.fp, cmd = m.fp.Update(msg)
	return cmd
}

func homeDir() string {
	h, _ := os.UserHomeDir()
	return h
}

func (m *model) pickerFrame(base string) string {
	ow := min(m.w, 64)
	oh := m.fp.Height() + 2
	l := panes.Layout{Width: ow, Height: oh, Focus: 1, Panes: []panes.Def{{Name: "Download to"}},
		Boxes: []panes.Box{{Pane: 1, Rect: panes.Rect{W: ow, H: oh}}}}
	box := m.frames.Draw(l, func(b panes.Box) panes.Content {
		in := b.Rect.Inner()
		dir := strings.Replace(m.fp.CurrentDirectory, homeDir(), "~", 1)
		var body []string
		for _, ln := range strings.Split(m.fp.View(), "\n") {
			body = append(body, ansi.Truncate(ln, in.W, m.ic.Ellipsis))
		}
		return panes.Content{TopLeft: []string{"Download " + m.dwhat + " to"},
			TopRight: []string{dir, ""}, Focus: true, Body: strings.Join(body, "\n")}
	})
	return m.place(base, box, ow)
}
