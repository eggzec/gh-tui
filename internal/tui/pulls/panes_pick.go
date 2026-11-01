package pulls

import (
	"slices"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/optimistic"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/keyhelp"
	"github.com/eggzec/gh-tui/pkg/bubbles/keymap"
	"github.com/eggzec/gh-tui/pkg/bubbles/panes"
	"github.com/eggzec/gh-tui/pkg/bubbles/picker"
)

// labelEditor is what the labels picker needs to change the labels.
type labelEditor interface {
	// EditLabels shows the labels in add as put on the pull request, and those
	// named in remove as taken off, in the cache at once. The returned Op sends
	// the change.
	EditLabels(repo core.RepoRef, number int, add []core.Label, remove []string) *optimistic.Op
}

// labelsReadMsg is the answer of the read of the labels of the repository.
type labelsReadMsg struct {
	id     int64
	labels []core.Label
	err    error
}

// openLabels reads the labels the repository has and opens the picker of the
// labels of the pull request over them. Without a read of the labels it offers
// those the pull request has.
func (m *panesModal) openLabels() tea.Cmd {
	if !m.loaded || m.top() != nil {
		return nil
	}
	if cmd, refused := m.gate().Refuse(ui.ActLabel, &m.detail.Issue); refused {
		return cmd
	}
	f := m.list.facets
	if f == nil {
		m.showLabels(nil)
		return m.focusPicker()
	}
	id, ctx, repo := m.id, m.ctx, m.repo
	return func() tea.Msg {
		labels, err := f.Labels(ctx, repo)
		return labelsReadMsg{id: id, labels: labels, err: err}
	}
}

// labelsRead opens the picker over the labels that were read.
func (m *panesModal) labelsRead(msg labelsReadMsg) tea.Cmd {
	if msg.id != m.id || m.top() != nil {
		return nil
	}
	if msg.err != nil {
		if m.ctx.Err() != nil {
			return nil
		}
		return ui.Fail("labels of #"+strconv.Itoa(m.number), core.About(core.Target{Repo: m.repo, Number: m.number}.String(), msg.err))
	}
	m.showLabels(msg.labels)
	return m.focusPicker()
}

// focusPicker focuses the picker that was just opened.
func (m *panesModal) focusPicker() tea.Cmd {
	if o, ok := m.top().(*labelsOverlay); ok {
		return o.p.Focus()
	}
	return nil
}

// showLabels opens the picker over known and the labels the pull request has,
// which stay on offer though the read of the repository lacks them.
func (m *panesModal) showLabels(known []core.Label) {
	have := m.detail.Labels
	all := slices.Clone(known)
	for _, l := range have {
		if !slices.ContainsFunc(all, func(o core.Label) bool { return strings.EqualFold(o.Name, l.Name) }) {
			all = append(all, l)
		}
	}
	labels := map[string]core.Label{}
	o := &labelsOverlay{
		heading: "Labels",
		order:   strings.ToLower,
		on:      map[string]bool{},
		was:     map[string]bool{},
		change: func(on, was map[string]bool) tea.Cmd {
			var add []core.Label
			var remove []string
			for name, l := range labels {
				switch {
				case on[name] && !was[name]:
					add = append(add, l)
				case !on[name] && was[name]:
					remove = append(remove, name)
				}
			}
			if len(add) == 0 && len(remove) == 0 {
				return nil
			}
			slices.SortFunc(add, func(a, b core.Label) int { return strings.Compare(a.Name, b.Name) })
			slices.Sort(remove)
			return m.applyLabels(add, remove)
		},
	}
	items := make([]picker.Item, len(all))
	for i, l := range all {
		labels[l.Name] = l
		items[i] = picker.Item{Title: l.Name, Detail: l.Description, Value: l.Name}
	}
	for _, l := range have {
		// The pull request's name may differ in case from the repository's.
		for _, a := range all {
			if strings.EqualFold(a.Name, l.Name) {
				o.on[a.Name], o.was[a.Name] = true, true
			}
		}
	}
	st := m.theme.Picker(m.icons)
	st.Frame = lipgloss.NewStyle()
	o.rows = len(items)
	o.p = picker.New(nil,
		picker.WithContext(m.ctx),
		picker.WithItems(items),
		picker.WithGroupHeaders(false),
		picker.WithModes(true),
		picker.WithFilterLine(true),
		picker.WithMarks(kindOn, kindOff),
		picker.WithTicks(),
		picker.WithMarkedFirst(true),
		picker.WithDetailMatch(true),
		picker.WithTitleStyle(func(it picker.Item, base lipgloss.Style) lipgloss.Style {
			name, _ := it.Value.(string)
			return m.chips.Tint(base, labels[name].Color)
		}),
		picker.WithKeyMap(m.keys.pick),
		picker.WithStyles(st),
	)
	o.p.SetMarked(o.marks())
	// The ticked labels are listed first, so the cursor starts on the first.
	if first, ok := o.firstTicked(); ok {
		o.p.Select(first)
	}
	m.open(o)
}

// labelsOverlay is the checklist of the labels of the pull request, keyed by
// label name.
type labelsOverlay = tickOverlay[string]

// tickOverlay is a checklist of rows keyed by K, each the Value of a picker
// item. Space ticks the highlighted row, enter hands the ticks to change, and
// esc or backspace closes without applying.
type tickOverlay[K comparable] struct {
	p       picker.Model
	heading string
	rows    int
	// on is which rows are ticked, and was which were when it opened.
	on, was map[K]bool
	// order names a key for the order of the first ticked row.
	order func(K) string
	// change turns the ticks into the command that applies them, or nil.
	change func(on, was map[K]bool) tea.Cmd
	// locked, when set, answers an attempt to change a locked row.
	locked func(picker.Item) tea.Cmd
	done   bool
}

func (o *tickOverlay[K]) title() string           { return o.p.Title(o.heading) }
func (o *tickOverlay[K]) kind() panes.OverlayKind { return panes.Picker }

// items is the rows of the picker and the rule between the ticked and the
// others.
func (o *tickOverlay[K]) items() int   { return o.rows + 1 }
func (o *tickOverlay[K]) text() int    { return 0 }
func (o *tickOverlay[K]) clear() bool  { return false }
func (o *tickOverlay[K]) closed() bool { return o.done }

// typing says that the search takes the keys as text.
func (o *tickOverlay[K]) typing() bool { return o.p.Typing() }

func (o *tickOverlay[K]) layers() []keyhelp.Layer {
	ctx := "picker_normal"
	if o.p.Typing() {
		ctx = "picker"
	}
	return []keyhelp.Layer{ui.ContextHelp(ctx, labelsHelp{&o.p}, false)}
}

// labelsHelp is the picker's help with a footer that says what the keys do
// here: space ticks a label and enter applies the ticks.
type labelsHelp struct{ p *picker.Model }

func (h labelsHelp) ShortHelp() []key.Binding {
	if h.p.Typing() {
		return h.p.ShortHelp()
	}
	k := h.p.KeyMap()
	toggle := k.Normal.Toggle
	toggle.SetHelp(toggle.Help().Key, "toggle")
	apply := k.Choose
	apply.SetHelp(apply.Help().Key, "apply")
	return []key.Binding{moving(k.Normal.Down, k.Normal.Up), k.Normal.Search, toggle, apply, k.Cancel}
}

// moving is the one hint for the keys that move, named by the first key of
// each, such as "j/k"; a move that has no key is left out of the name.
func moving(down, up key.Binding) key.Binding {
	var names, keys []string
	for _, b := range []key.Binding{down, up} {
		ks := b.Keys()
		if len(ks) == 0 {
			continue
		}
		names = append(names, keymap.Label(ks[0]))
		keys = append(keys, ks...)
	}
	// The keys are what makes the hint enabled; it is shown, never pressed.
	hint := key.NewBinding(key.WithKeys(keys...), key.WithHelp(strings.Join(names, "/"), "move"))
	hint.SetEnabled(len(keys) > 0)
	return keymap.Derive(hint, down, up)
}

func (h labelsHelp) FullHelp() [][]key.Binding { return h.p.FullHelp() }

func (o *tickOverlay[K]) view(w, h int) string {
	// The frame keeps a cell of room on each side of what it holds.
	o.p.SetSize(w-2, h)
	return o.p.View()
}

// firstTicked returns the ticked key that is listed first, and whether there
// is one.
func (o *tickOverlay[K]) firstTicked() (K, bool) {
	var first K
	found := false
	for k, on := range o.on {
		if on && (!found || strings.ToLower(o.order(k)) < strings.ToLower(o.order(first))) {
			first, found = k, true
		}
	}
	return first, found
}

// marks returns the ticked keys.
func (o *tickOverlay[K]) marks() []any {
	var marked []any
	for k, on := range o.on {
		if on {
			marked = append(marked, k)
		}
	}
	return marked
}

func (o *tickOverlay[K]) update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case picker.ChosenMsg:
		if msg.ID != o.p.ID() {
			return nil
		}
		o.on = map[K]bool{}
		for _, it := range msg.Ticked {
			if k, ok := it.Value.(K); ok {
				o.on[k] = true
			}
		}
		o.done = true
		return o.change(o.on, o.was)
	case picker.LockedMsg:
		if msg.ID != o.p.ID() || o.locked == nil {
			return nil
		}
		return o.locked(msg.Item)
	case picker.CancelMsg:
		if msg.ID == o.p.ID() {
			o.done = true
		}
		return nil
	}
	var cmd tea.Cmd
	o.p, cmd = o.p.Update(msg)
	return cmd
}

// applyLabels changes the labels of the pull request, which the header shows
// at once, and reads it again once the change went through or was rolled back.
func (m *panesModal) applyLabels(add []core.Label, remove []string) tea.Cmd {
	op := m.svc.EditLabels(m.repo, m.number, add, remove)
	if d, ok := m.svc.CachedGet(m.repo, m.number); ok {
		m.detail = d
		m.refreshInvolved()
	}
	return m.sendOp(op, "labels on #"+strconv.Itoa(m.number))
}
