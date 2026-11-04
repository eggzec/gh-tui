package prpanes

import (
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/composer"
	"github.com/eggzec/gh-tui/pkg/bubbles/keyhelp"
	"github.com/eggzec/gh-tui/pkg/bubbles/keymap"
	"github.com/eggzec/gh-tui/pkg/bubbles/panes"
	"github.com/eggzec/gh-tui/pkg/bubbles/prompt"
	"github.com/eggzec/gh-tui/pkg/bubbles/toast"
	"github.com/eggzec/gh-tui/pkg/termtext"
)

// DescEditKeys are the keys of the editor of a title and a description. The
// editor types, so they are control keys.
type DescEditKeys struct {
	Submit    key.Binding `keymap:"submit" help:"save"`
	Cancel    key.Binding `keymap:"cancel" help:"cancel"`
	Edit      key.Binding `keymap:"edit" help:"editor"`
	NextField key.Binding `keymap:"next_field" help:"next field"`
	PrevField key.Binding `keymap:"prev_field" help:"previous field"`
}

// NewDescEditKeys returns the keys of the editor from the context ctx of keys.
func NewDescEditKeys(keys config.Keymap, ctx string) DescEditKeys {
	var k DescEditKeys
	keymap.Fill(&k, ui.Lookup(keys, ctx))
	return k
}

// ShortHelp implements help.KeyMap.
func (k DescEditKeys) ShortHelp() []key.Binding {
	return []key.Binding{k.Submit, k.Cancel, k.Edit, k.NextField, k.PrevField}
}

// FullHelp implements help.KeyMap.
func (k DescEditKeys) FullHelp() [][]key.Binding { return [][]key.Binding{k.ShortHelp()} }

// The fields of the editor that take the cursor.
const (
	editTitle = iota
	editBody
)

// descEditBodyMin is the least number of lines the body of the editor takes.
const descEditBodyMin = 3

// DescEdit edits the title and the description of a pull request or an
// issue. It types: it takes every key itself, and its own keys save and
// close it.
type DescEdit struct {
	// Head is what its frame says.
	Head string
	// Noun names what is edited, for the refusal of an empty title.
	Noun string
	// Ctx is the context of its keys.
	Ctx string
	// Send sends the title and the description, or returns the command that
	// says why it can't, and reports whether the editor may close.
	Send func(title, body string) (cmd tea.Cmd, ok bool)

	keys   DescEditKeys
	titleF prompt.Model
	bodyF  composer.Model
	focus  int
	done   bool
}

// NewDescEdit returns the editor, filled with title and body.
func NewDescEdit(head, noun, ctx string, keys DescEditKeys, st prompt.Styles, title, body string) *DescEdit {
	o := &DescEdit{Head: head, Noun: noun, Ctx: ctx, keys: keys}
	o.titleF = prompt.New(prompt.WithMode(prompt.SingleLine), prompt.WithTitle("Title"))
	o.bodyF = composer.New("", composer.WithOptional(), composer.WithTitle("Description"),
		composer.WithKeyMap(composer.KeyMap{Edit: keys.Edit}))
	o.titleF.SetStyles(st)
	o.bodyF.SetStyles(st)
	o.titleF.SetValue(title)
	o.bodyF.SetValue(body)
	return o
}

// Init starts the fields and gives the title the cursor.
func (o *DescEdit) Init() tea.Cmd { return tea.Batch(o.bodyF.Init(), o.titleF.Focus()) }

// Changed reports whether the title or the description differ from title and
// body.
func (o *DescEdit) Changed(title, body string) bool {
	return o.titleF.Value() != title || o.bodyF.Value() != body
}

// Title is the heading of the frame.
func (o *DescEdit) Title() string { return o.Head }

// Kind says the frame is a viewer.
func (o *DescEdit) Kind() panes.OverlayKind { return panes.Viewer }

// Items is zero: the editor has no rows.
func (o *DescEdit) Items() int { return 0 }

// Text is zero: the editor has no text to find in.
func (o *DescEdit) Text() int { return 0 }

// Clear reports false: the editor has no find or filter to clear.
func (o *DescEdit) Clear() bool { return false }

// Closed reports whether the overlay is done.
func (o *DescEdit) Closed() bool { return o.done }

// Typing reports that the overlay takes the keys of the app's actions as text.
func (o *DescEdit) Typing() bool { return true }

// Layers lists the keys of the editor for the help.
func (o *DescEdit) Layers() []keyhelp.Layer {
	k := o.keys
	k.Edit.SetEnabled(k.Edit.Enabled() && o.focus == editBody)
	return []keyhelp.Layer{ui.ContextHelp(o.Ctx, k, true)}
}

// View draws the title over the description at the size of the frame.
func (o *DescEdit) View(w, h int) string {
	o.titleF.SetSize(w, prompt.SingleLineHeight)
	o.bodyF.SetSize(w, max(h-prompt.SingleLineHeight, descEditBodyMin))
	lines := append(strings.Split(o.titleF.View(), "\n"), strings.Split(o.bodyF.View(), "\n")...)
	return strings.Join(termtext.PadLines(lines, w, h), "\n")
}

// Update handles a message.
func (o *DescEdit) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch {
		case key.Matches(msg, o.keys.Cancel):
			o.done = true
			return nil
		case key.Matches(msg, o.keys.Submit):
			return o.save()
		case key.Matches(msg, o.keys.NextField), key.Matches(msg, o.keys.PrevField):
			return o.switchField()
		}
		return o.forward(msg)
	case tea.PasteMsg:
		return o.forward(msg)
	}
	// The messages of the fields, such as the editor's result, carry the ID of
	// the field they are for, so each takes only its own.
	var tc, bc tea.Cmd
	o.titleF, tc = o.titleF.Update(msg)
	o.bodyF, bc = o.bodyF.Update(msg)
	return tea.Batch(tc, bc)
}

// forward gives msg to the field that has the cursor.
func (o *DescEdit) forward(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	if o.focus == editTitle {
		o.titleF, cmd = o.titleF.Update(msg)
	} else {
		o.bodyF, cmd = o.bodyF.Update(msg)
	}
	return cmd
}

// switchField moves the cursor to the other field.
func (o *DescEdit) switchField() tea.Cmd {
	if o.focus == editTitle {
		o.focus = editBody
		o.titleF.Blur()
		return o.bodyF.Focus()
	}
	o.focus = editTitle
	o.bodyF.Blur()
	return o.titleF.Focus()
}

// save sends the title and the description as they are.
func (o *DescEdit) save() tea.Cmd {
	title := strings.TrimSpace(o.titleF.Value())
	if title == "" {
		return ui.Notify(toast.Info, "A "+o.Noun+" needs a title.")
	}
	cmd, ok := o.Send(title, o.bodyF.Value())
	o.done = ok
	return cmd
}
