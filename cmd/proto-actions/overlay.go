package main

import (
	"context"
	"fmt"
	"os"
	"slices"
	"strings"

	"charm.land/bubbles/v2/filepicker"
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/filterform"
	"github.com/eggzec/gh-tui/pkg/bubbles/orderpick"
	"github.com/eggzec/gh-tui/pkg/bubbles/panes"
	"github.com/eggzec/gh-tui/pkg/termtext"
)

// The group, filter and sort overlays are the app's own modals (ui.GroupModal,
// ui.FilterModal, the sort levels modal), drawn in the frame of the root
// model's modals.

// runFilter lets the filter modal apply its values to the prototype's runs.
type runFilter struct{ m *model }

func (f runFilter) Filter() (ui.Filter, bool) {
	var fields []filterform.Field
	for _, name := range facets {
		var opts []filterform.Item
		for _, v := range facetValues(name)[1:] {
			opts = append(opts, filterform.Item{Label: v, Value: v})
		}
		fields = append(fields, filterform.Field{Key: name, Label: strings.ToUpper(name[:1]) + name[1:],
			Kind: filterform.Multi, Qualifier: name, Options: opts, Hint: "any " + name,
			Default: filterform.ListValue(f.m.flt[name]...)})
	}
	return ui.Filter{Spec: filterform.Spec{Fields: fields}, Subject: "runs"}, true
}

func (f runFilter) ApplyFilter(msg filterform.AppliedMsg) tea.Cmd {
	f.m.flt = map[string][]string{}
	for _, name := range facets {
		if l := msg.Values[name].List(); len(l) > 0 {
			f.m.flt[name] = slices.Clone(l)
		}
	}
	f.m.fltOff, f.m.sel = false, 0
	return nil
}

// levelItems are the fields a level modal offers.
func levelItems(names []string) []orderpick.Item {
	var items []orderpick.Item
	for _, n := range names {
		if n != "none" {
			items = append(items, orderpick.Item{Value: n, Title: n, AscLabel: "ascending", DescLabel: "descending"})
		}
	}
	return items
}

func (m *model) openOv(kind int) {
	keys := ui.OrderPickKeys(m.cfg.Keys)
	switch kind {
	case 1:
		var chosen []orderpick.Choice
		if g := m.grouped(); g != "" {
			chosen = []orderpick.Choice{{Value: g}}
		}
		m.modal = ui.NewGroupModal("runs", levelItems(groups), chosen, keys, m.ic, func(c []orderpick.Choice) tea.Cmd {
			m.groupBy, m.groupOff, m.sel = "none", false, 0
			if len(c) > 0 {
				// The tree nests one level; the first one chosen is it.
				m.groupBy = c[0].Value.(string)
			}
			return nil
		})
	case 2:
		f, _ := runFilter{m}.Filter()
		m.modal = ui.NewFilterModal(context.Background(), "runs", runFilter{m}, f,
			ui.WithFormKeys(ui.FilterFormKeys(m.cfg.Keys, "filter")), ui.WithFormIcons(m.ic))
	case 3:
		chosen := []orderpick.Choice{{Value: m.sortBy, Desc: m.sortRev}}
		m.modal = ui.NewSortLevelsModal("runs", levelItems(sorts), chosen, keys, m.ic, func(c []orderpick.Choice) tea.Cmd {
			m.sortBy, m.sortRev, m.sel = "newest", false, 0
			if len(c) > 0 {
				m.sortBy, m.sortRev = c[0].Value.(string), c[0].Desc
			}
			return nil
		})
	}
	m.ov = kind
	m.modal.SetTheme(m.th)
	m.sizeModal()
}

// frameSize is the size of the modal with its frame, as the root model's.
func (m *model) frameSize() (int, int) {
	w, h := max(m.w-2*max(m.w/10, 2), 0), max(m.h-2*max(m.h/10, 1), 0)
	if f, ok := m.modal.(ui.Fitter); ok {
		fw, fh := f.Fit(max(w-4, 0), max(h-2, 0))
		w, h = min(w, fw+4), min(h, fh+2)
	}
	return w, h
}

func (m *model) sizeModal() {
	w, h := m.frameSize()
	m.modal.SetSize(max(w-4, 0), max(h-2, 0))
}

// closeOv closes the open overlay.
func (m *model) closeOv() { m.modal, m.ov = nil, 0 }

// modalUpdate gives msg to the open overlay.
func (m *model) modalUpdate(msg tea.Msg) tea.Cmd {
	switch msg.(type) {
	case ui.LeaveModalMsg, ui.CloseModalMsg:
		m.closeOv()
		return nil
	}
	cmd := m.modal.Update(msg)
	if m.modal != nil {
		m.sizeModal()
	}
	return cmd
}

// overlayHints are the short help of the open overlay's key layers.
func (m *model) overlayHints() []key.Binding {
	k, ok := m.modal.(ui.Keyed)
	if !ok {
		return nil
	}
	var out []key.Binding
	for _, l := range k.KeyLayers() {
		out = append(out, l.Short...)
	}
	return out
}

// frame draws the modal in its frame, with its title in the top edge, as the
// root model does.
func (m *model) frame() (string, int) {
	w, _ := m.frameSize()
	if w < 4 {
		return "", 0
	}
	b := m.ic.Border
	title := termtext.Truncate(" "+ui.OneLine(m.modal.Title())+" ", max(w-4, 0), m.ic.Ellipsis+" ")
	rest := max(w-3-lipgloss.Width(title), 0)
	top := m.th.Accent.Render(b.TopLeft+b.Top) + m.th.Title.Render(title) +
		m.th.Accent.Render(strings.Repeat(b.Top, rest)) + m.th.Accent.Render(b.TopRight)
	return top + "\n" + m.th.Frame().Border(b, false, true, true).Render(m.modal.View()), w
}

// overlay draws the open overlay over base.
func (m *model) overlay(base string) string {
	if m.ov == 5 || m.ov == 4 {
		base = m.dlFrame(base)
	}
	if m.ov == 4 {
		return m.pickerFrame(base)
	}
	if m.ov == 5 {
		return base
	}
	box, w := m.frame()
	if box == "" {
		return base
	}
	return m.place(base, box, w)
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
		m.ov = 5
		return nil
	case "c":
		m.ov, m.dpick = 0, nil
		m.dldir = strings.Replace(m.fp.CurrentDirectory, homeDir(), "~", 1)
		m.confirm = m.dwhat
		return nil
	}
	var cmd tea.Cmd
	m.fp, cmd = m.fp.Update(msg)
	return cmd
}

// openDownloads opens the popup listing the selected run's artifacts.
func (m *model) openDownloads() {
	ar := m.arts()
	if len(ar) == 0 {
		m.toast = "no downloads for this run"
		return
	}
	m.ov, m.dsel, m.dpick = 5, 0, map[int]bool{}
}

// dlKey handles a key in the downloads popup.
func (m *model) dlKey(k string) {
	ar := m.arts()
	switch k {
	case "esc", "q":
		m.ov, m.dpick = 0, nil
	case "j", "down":
		m.dsel = min(len(ar)-1, m.dsel+1)
	case "k", "up":
		m.dsel = max(0, m.dsel-1)
	case "space", " ":
		m.dpick[m.dsel] = !m.dpick[m.dsel]
	case "a":
		all := len(m.dpick) < len(ar)
		for i := range ar {
			m.dpick[i] = all
		}
		for i, v := range m.dpick {
			if !v {
				delete(m.dpick, i)
			}
		}
	case "enter":
		var names []string
		for i, a := range ar {
			if m.dpick[i] {
				names = append(names, a.name)
			}
		}
		switch len(names) {
		case 0:
			names = []string{ar[m.dsel].name}
			m.dwhat = names[0]
		case 1:
			m.dwhat = names[0]
		default:
			m.dwhat = fmt.Sprintf("%d downloads", len(names))
		}
		m.confirmCount = len(names)
		m.openPicker()
	}
}

// dlFrame draws the downloads popup over base.
func (m *model) dlFrame(base string) string {
	ar := m.arts()
	ow := min(m.w, 64)
	oh := min(len(ar)+2, max(m.h-4, 3))
	l := panes.Layout{Width: ow, Height: oh, Focus: 1, Panes: []panes.Def{{Name: "Downloads"}},
		Boxes: []panes.Box{{Pane: 1, Rect: panes.Rect{W: ow, H: oh}}}}
	box := m.frames.Draw(l, func(b panes.Box) panes.Content {
		in := b.Rect.Inner()
		return panes.Content{TopLeft: []string{"Downloads"}, TopRight: []string{fmt.Sprintf("%d marked", len(m.dpick)), ""},
			Focus: true, Body: m.dlBody(ar, in.W, in.H)}
	})
	return m.place(base, box, ow)
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
