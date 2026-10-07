package filterform

import (
	"context"
	"slices"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/pkg/bubbles/picker"
	"github.com/eggzec/gh-tui/pkg/termtext"
)

// dropRows is the most items a dropdown shows at once. A list with more
// scrolls, and has a filter line.
const dropRows = 8

// listKind is what a dropdown lists.
type listKind int

const (
	// listChoice lists the options of a Choice, listSortBy those a list
	// can be sorted by, and listOrder the two orders of the sort in force.
	listChoice listKind = iota
	listSortBy
	listOrder
	// listMulti is a checklist of a Multi's options, and listPerson the
	// people for a Person, with what the user types.
	listMulti
	listPerson
)

// listOf returns what the dropdown of the row in focus lists, or false if
// the row has none: a Toggle or Text field, the query line, and the order
// of a sort that has none.
func (m *Model) listOf() (listKind, bool) {
	if m.row == m.queryRow() {
		return 0, false
	}
	if m.tab == SortTab {
		switch {
		case m.spec.Sort == nil:
		case m.row == sortByRow:
			return listSortBy, true
		case m.row == sortOrderRow && m.spec.Sort.index(m.state.sort.By) >= 0 && m.state.sort.By != "":
			return listOrder, true
		}
		return 0, false
	}
	switch m.spec.Fields[m.row].Kind {
	case Choice:
		return listChoice, true
	case Multi:
		return listMulti, true
	case Person:
		return listPerson, true
	default:
		return 0, false
	}
}

// openList opens the dropdown of the row in focus, which first loads its
// options if it has a Loader.
func (m *Model) openList() tea.Cmd {
	kind, ok := m.listOf()
	if !ok {
		return nil
	}
	m.mode = listMode
	if kind == listMulti {
		// Esc puts back what the checklist had when it opened.
		m.before = m.state.values[m.row].clone()
	}
	if kind == listMulti || kind == listPerson {
		i := m.row
		if m.spec.Fields[i].Load != nil && m.fields[i].state != loaded {
			if m.fields[i].state == loading {
				// The options are on their way, and open it when they land.
				return nil
			}
			return m.load(i)
		}
	}
	return m.openPicker(kind)
}

// closeEditor leaves insert mode or closes an open dropdown, keeping what
// was typed or chosen, or, without keep, putting back the value a Text
// field or a checklist had when it opened. A list changes its value only
// when it chooses, so it has nothing to put back.
func (m *Model) closeEditor(keep bool) {
	switch m.mode {
	case rowsMode:
		return
	case insertMode:
		switch {
		case m.row == m.queryRow():
			m.query.Blur()
		case keep:
			m.text.Blur()
			m.setValue(m.row, TextValue(strings.TrimSpace(m.text.Value())))
		default:
			m.text.Blur()
			m.setValue(m.row, m.before)
		}
	case listMode:
		if !keep && m.tab == FiltersTab && m.kind() == Multi {
			m.setValue(m.row, m.before)
		}
	}
	m.mode, m.picking = rowsMode, false
	m.pick = picker.Model{}
	m.syncQuery()
}

// pressList handles a key while a dropdown is open or waits for its
// options. The keys that act on the form, those that choose, close, retry
// and switch tabs, are looked for first, unless the filter takes the
// letters; the rest go to the picker, which moves and filters.
func (m *Model) pressList(msg tea.KeyPressMsg) tea.Cmd {
	k := m.keys
	typing := m.picking && m.pick.Typing()
	switch {
	case typing:
		if key.Matches(msg, k.List.Choose) {
			return m.chooseHighlighted()
		}
		return m.typeIn(msg)
	case key.Matches(msg, k.Quit):
		return send(CancelMsg{ID: m.id})
	case m.tabbed() && key.Matches(msg, k.NextTab):
		m.switchTab(1)
		return nil
	case m.tabbed() && key.Matches(msg, k.PrevTab):
		m.switchTab(-1)
		return nil
	case !m.picking:
		// The options are loading or failed to.
		switch {
		case key.Matches(msg, k.List.Cancel):
			m.closeEditor(false)
		case key.Matches(msg, k.Retry) && m.tab == FiltersTab && m.fields[m.row].state == failed:
			return m.load(m.row)
		}
		return nil
	case key.Matches(msg, k.ListToggle) && m.kind() == Multi:
		m.checkHighlighted(true)
		return nil
	case key.Matches(msg, k.ListClear):
		m.clearList()
		return nil
	case key.Matches(msg, k.List.Choose):
		return m.chooseHighlighted()
	case key.Matches(msg, k.List.Cancel):
		m.closeEditor(false)
		return nil
	}
	return m.typeIn(msg)
}

// chooseHighlighted handles the key that chooses: it chooses the
// highlighted item at once, so that the keys that follow find the dropdown
// closed. A Person takes what was typed instead while its search runs or
// failed, since the list isn't what enter chooses then, and when the
// highlighted item is the typed one.
func (m *Model) chooseHighlighted() tea.Cmd {
	text := login(m.pick.Query().Text)
	if m.kind() == Person && text != "" && (m.pick.Loading() || m.pick.Err() != nil) {
		m.chooseText(text)
		return nil
	}
	if it, ok := m.pick.Selected(); ok {
		return m.chosen(it)
	}
	if m.kind() == Person && text != "" {
		m.chooseText(text)
	}
	return nil
}

// login returns what the user typed as a login: on one line, and trimmed.
func login(text string) string { return strings.TrimSpace(termtext.OneLine(text)) }

// chosen handles the item chosen in the open dropdown: a list or a Person
// takes it and closes, and a checklist checks the highlighted item and
// closes, or, in its filter, checks it and returns to the list.
func (m *Model) chosen(it picker.Item) tea.Cmd {
	kind, _ := m.listOf()
	value, _ := it.Value.(string)
	switch kind {
	case listChoice:
		m.setValue(m.row, TextValue(value))
	case listSortBy:
		if i := m.spec.Sort.index(value); i >= 0 {
			m.sortBy(m.spec.Sort.Options[i])
		}
	case listOrder:
		m.state.sort.Desc = value == orderDesc
		m.syncQuery()
	case listPerson:
		m.chooseText(login(value))
		return nil
	case listMulti:
		return m.chosenMulti(value)
	}
	m.closeEditor(true)
	return nil
}

// chooseText sets the Person in focus to text and closes its dropdown.
func (m *Model) chooseText(text string) {
	if text != "" {
		m.setValue(m.row, TextValue(text))
	}
	m.closeEditor(true)
}

// chosenMulti handles enter in a checklist. In the list, enter closes it
// and doesn't leave the highlighted item out: it checks it, unless it is
// checked, or space just acted on it, so that unchecking an item and
// pressing enter doesn't check it again. In the filter, enter checks the
// item, empties the filter and goes back to the list, so that the next
// item can be filtered for. Enter in the filter only adds: it never
// unchecks.
func (m *Model) chosenMulti(value string) tea.Cmd {
	if !m.pick.Typing() {
		if value != m.toggled {
			m.checkHighlighted(false)
		}
		m.closeEditor(true)
		return nil
	}
	m.checkHighlighted(false)
	cmd := m.pick.Reset()
	m.pick.Focus()
	m.pick.Select(value)
	return cmd
}

// checkHighlighted checks the highlighted item of a checklist, or with
// flip unchecks it if it is checked.
func (m *Model) checkHighlighted(flip bool) {
	it, ok := m.pick.Selected()
	value, isString := it.Value.(string)
	if !ok || !isString {
		return
	}
	list := m.state.values[m.row].list
	switch j := slices.Index(list, value); {
	case j < 0:
		list = append(slices.Clone(list), value)
	case flip:
		list = slices.Delete(slices.Clone(list), j, j+1)
	default:
		return
	}
	m.setValue(m.row, Value{list: list})
	m.pick.SetMarked(anys(list))
	m.toggled = value
}

// doneWord is what enter does in a checklist, for the help line: it adds
// the highlighted item if it isn't checked and space didn't just act on it,
// and closes the list.
func (m *Model) doneWord() string {
	it, ok := m.pick.Selected()
	value, isString := it.Value.(string)
	if ok && isString && value != m.toggled && !slices.Contains(m.state.values[m.row].list, value) {
		return "add & close"
	}
	return "done"
}

// clearList handles the clear key in a dropdown. A list chooses its empty
// option and closes, where it has one; a checklist unchecks every item and
// stays open, and enter then closes it without adding the highlighted item,
// as it does after space.
func (m *Model) clearList() {
	if !m.canClear() {
		return
	}
	multi := m.kind() == Multi
	m.remove()
	if multi {
		it, _ := m.pick.Selected()
		m.toggled, _ = it.Value.(string)
		m.pick.SetMarked(nil)
		return
	}
	m.closeEditor(true)
}

// orderDesc and orderAsc are the values of the two items of the list of
// orders.
const (
	orderDesc = "desc"
	orderAsc  = "asc"
)

// Err returns why the options of a field failed to load, the first
// field's, or nil if none failed.
func (m Model) Err() error {
	for _, fs := range m.fields {
		if fs.state == failed {
			return fs.err
		}
	}
	return nil
}

// Retry loads again the options of each field whose load failed, as
// opening the field does, such as once the network is back. It returns
// nil if none failed.
func (m *Model) Retry() tea.Cmd {
	var cmds []tea.Cmd
	for i := range m.fields {
		if m.fields[i].state != failed {
			continue
		}
		if cmds == nil {
			// Copies of the model share the fields.
			m.fields = slices.Clone(m.fields)
		}
		cmds = append(cmds, m.load(i))
	}
	if cmds == nil {
		return nil
	}
	m.render()
	return tea.Batch(cmds...)
}

// load starts loading field i's options.
func (m *Model) load(i int) tea.Cmd {
	m.seq++
	fs := &m.fields[i]
	fs.state, fs.err, fs.seq = loading, nil, m.seq
	load, ctx, id, seq := m.spec.Fields[i].Load, m.ctx, m.id, m.seq
	cmd := func() tea.Msg {
		items, err := load(ctx, "")
		return loadedMsg{id: id, field: i, seq: seq, items: items, err: err}
	}
	if m.spinning {
		return cmd
	}
	m.spinning = true
	return tea.Batch(cmd, m.spin.Tick)
}

// receive keeps what a load returned, and opens the dropdown's picker if it
// is waiting for it.
func (m *Model) receive(msg loadedMsg) tea.Cmd {
	if msg.field >= len(m.fields) {
		return nil
	}
	fs := m.fields[msg.field]
	if fs.state != loading || fs.seq != msg.seq {
		return nil
	}
	m.fields = slices.Clone(m.fields)
	if msg.err != nil {
		fs.state, fs.err = failed, msg.err
	} else {
		fs.state, fs.items = loaded, slices.Clone(msg.items)
	}
	m.fields[msg.field] = fs
	var cmd tea.Cmd
	if kind, ok := m.listOf(); ok && m.mode == listMode && m.row == msg.field && fs.state == loaded {
		cmd = m.openPicker(kind)
	}
	m.render()
	return cmd
}

// listKeyMap returns the keys of the dropdown's picker: those of List,
// with Typing's up and down moving while its filter types.
func (m *Model) listKeyMap() picker.KeyMap {
	km := m.keys.List
	km.Up, km.Down = m.keys.Typing.Up, m.keys.Typing.Down
	return km
}

// openPicker opens the picker of the dropdown that lists kind on the row
// in focus, over its options, with the current value highlighted and
// marked. A Person with a Loader also searches with it for what the user
// types.
func (m *Model) openPicker(kind listKind) tea.Cmd {
	g := m.styles.Glyphs
	items, marked, current := m.listItems(kind)
	opts := []picker.Option{
		picker.WithItems(items),
		picker.WithContext(m.ctx),
		picker.WithModes(true),
		picker.WithGroupHeaders(false),
		picker.WithKeyMap(m.listKeyMap()),
		picker.WithStyles(m.dropPickerStyles()),
	}
	var search picker.Search
	filter := len(items) > dropRows
	empty := "Nothing to choose from."
	placeholder := "Filter" + g.Ellipsis
	if m.tab == FiltersTab {
		f := &m.spec.Fields[m.row]
		filter = filter || f.Load != nil
		if f.Empty != "" {
			empty = f.Empty
		}
	}
	// Checks are boxes, and a list's one choice a radio.
	on, off := g.On, g.Off
	if kind == listMulti {
		on, off = boxOn, boxOff
	}
	opts = append(opts, picker.WithMarks(on, off))
	markW := max(ansi.StringWidth(on), ansi.StringWidth(off))
	if kind == listPerson {
		filter = true
		placeholder = "Filter, or type a login" + g.Ellipsis
		opts = append(opts, picker.WithTyped(m.typedLogin))
		if load := m.spec.Fields[m.row].Load; load != nil {
			search = func(ctx context.Context, q picker.Query) ([]picker.Item, error) {
				found, err := load(ctx, q.Text)
				return toPickerItems(found), err
			}
		}
	}
	opts = append(opts,
		picker.WithFilterLine(filter),
		picker.WithPlaceholder(placeholder),
		picker.WithEmptyText(empty),
	)
	if say := m.errorText; say != nil {
		opts = append(opts, picker.WithErrorText(func(err error) (text, hint string) {
			text, _ = say(err)
			return text, ""
		}))
	}
	m.pick = picker.New(search, opts...)
	m.toggled = ""
	m.pick.SetMarked(marked)
	if current != nil {
		m.pick.Select(current)
	}
	m.picking = true
	m.dropItems = len(items)
	// The cells that the filter line and the empty text need.
	inner := 0
	if filter {
		// The prompt and the cursor.
		inner = ansi.StringWidth(placeholder) + 3
	}
	if len(items) == 0 {
		inner = max(inner, ansi.StringWidth(empty))
	}
	m.dropWidth = m.wantWidth(items, markW, inner)
	return tea.Batch(m.pick.Focus(), m.pick.Init())
}

// typedLogin is the item that stands for what the user typed in a Person's
// filter: it chooses the text as it is.
func (m *Model) typedLogin(text string) (picker.Item, bool) {
	text = login(text)
	if text == "" {
		return picker.Item{}, false
	}
	enter := m.keys.List.Choose.Help().Key
	if m.keyName != nil {
		enter = m.keyName(enter)
	}
	return picker.Item{Title: enter + ` use "` + text + `"`, Value: text}, true
}

// dropPickerStyles returns the styles of the dropdown's picker, which the
// dropdown's own frame surrounds.
func (m *Model) dropPickerStyles() picker.Styles {
	s := m.styles.Picker
	s.Frame = lipgloss.NewStyle()
	return s
}

// listItems returns the items of the dropdown that lists kind on the row in
// focus, the values of those to mark, and the value to highlight, or nil.
func (m *Model) listItems(kind listKind) (items []picker.Item, marked []any, current any) {
	g := m.styles.Glyphs
	switch kind {
	case listSortBy:
		sf, so := m.spec.Sort, m.state.sort
		items = make([]picker.Item, len(sf.Options))
		for j, opt := range sf.Options {
			items[j] = picker.Item{Title: optionLabel(opt.Label, opt.Value), Value: opt.Value}
		}
		if sf.index(so.By) >= 0 {
			current = so.By
		}
		return items, []any{current}, current
	case listOrder:
		opt := m.spec.Sort.Options[m.spec.Sort.index(m.state.sort.By)]
		items = []picker.Item{
			{Title: g.Down + " " + opt.Desc, Value: orderDesc},
			{Title: g.Up + " " + opt.Asc, Value: orderAsc},
		}
		current = orderAsc
		if m.state.sort.Desc {
			current = orderDesc
		}
		return items, []any{current}, current
	case listChoice, listMulti, listPerson:
	}
	i := m.row
	f, v := &m.spec.Fields[i], m.state.values[i]
	items = toPickerItems(m.items(i))
	for j := range items {
		if items[j].Title == "" {
			items[j].Title = orDefault(f.Hint, "any")
		}
	}
	switch kind {
	case listMulti:
		return items, anys(v.list), nil
	case listPerson:
		if v.text != "" {
			current, marked = v.text, []any{v.text}
		}
		return items, marked, current
	default:
		if slices.ContainsFunc(f.Options, func(it Item) bool { return it.Value == v.text }) {
			current = v.text
			return items, []any{current}, current
		}
		return items, nil, nil
	}
}

// anys returns list as the values a picker marks.
func anys(list []string) []any {
	out := make([]any, len(list))
	for i, s := range list {
		out[i] = s
	}
	return out
}

// toPickerItems returns items as a picker lists them.
func toPickerItems(items []Item) []picker.Item {
	out := make([]picker.Item, len(items))
	for j, it := range items {
		out[j] = picker.Item{Title: orDefault(it.Label, it.Value), Detail: it.Detail, Value: it.Value}
	}
	return out
}
