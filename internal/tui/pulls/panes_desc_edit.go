package pulls

import (
	"strconv"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/composer"
	"github.com/eggzec/gh-tui/pkg/bubbles/keyhelp"
	"github.com/eggzec/gh-tui/pkg/bubbles/keymap"
	"github.com/eggzec/gh-tui/pkg/bubbles/panes"
	"github.com/eggzec/gh-tui/pkg/bubbles/prompt"
	"github.com/eggzec/gh-tui/pkg/bubbles/toast"
	"github.com/eggzec/gh-tui/pkg/termtext"
)

// ctxDescEdit is the context of the keys of the editor of the title and the
// description. The editor types, so they are control keys.
const ctxDescEdit = "pull_desc_edit"

// descEditKeys are the keys of the editor of the title and the description.
type descEditKeys struct {
	Submit    key.Binding `keymap:"submit" help:"save"`
	Cancel    key.Binding `keymap:"cancel" help:"cancel"`
	Edit      key.Binding `keymap:"edit" help:"editor"`
	NextField key.Binding `keymap:"next_field" help:"next field"`
	PrevField key.Binding `keymap:"prev_field" help:"previous field"`
}

// newDescEditKeys returns the keys of the editor from keys.
func newDescEditKeys(keys config.Keymap) descEditKeys {
	var k descEditKeys
	keymap.Fill(&k, ui.Lookup(keys, ctxDescEdit))
	return k
}

// ShortHelp implements help.KeyMap.
func (k descEditKeys) ShortHelp() []key.Binding {
	return []key.Binding{k.Submit, k.Cancel, k.Edit, k.NextField, k.PrevField}
}

// FullHelp implements help.KeyMap.
func (k descEditKeys) FullHelp() [][]key.Binding { return [][]key.Binding{k.ShortHelp()} }

// quote returns body as a markdown quote, ready to be answered under: each
// line behind a "> ", and a blank line after.
func quote(body string) string {
	lines := strings.Split(strings.TrimRight(body, "\n"), "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight("> "+l, " ")
	}
	return strings.Join(lines, "\n") + "\n\n"
}

// openDescEdit opens the editor of the title and the description, if the
// viewer may change them.
func (m *panesModal) openDescEdit() tea.Cmd {
	if !m.loaded {
		return ui.Notify(toast.Info, checking(m.icons, m.number))
	}
	if cmd, refused := m.gate().Refuse(ui.ActDraft, &m.detail.Issue); refused {
		return cmd
	}
	o := newDescEdit(m)
	m.open(o)
	return tea.Batch(o.bodyF.Init(), o.titleF.Focus())
}

// quoteDescription opens the composer of a new comment, which starts with the
// description quoted.
func (m *panesModal) quoteDescription() tea.Cmd {
	if !m.loaded {
		return ui.Notify(toast.Info, checking(m.icons, m.number))
	}
	body := strings.TrimSpace(m.detail.Body)
	if body == "" {
		return ui.Notify(toast.Info, "There is no description to quote.")
	}
	if cmd, refused := m.gate().Refuse(ui.ActComment, &m.detail.Issue); refused {
		return cmd
	}
	num := "#" + strconv.Itoa(m.number)
	c := composer.New("comment/"+m.repo.String()+"/"+strconv.Itoa(m.number),
		composer.WithTitle("Comment on "+num),
		composer.WithPlaceholder("Write the comment in markdown."),
		composer.WithKeyMap(m.keys.composer),
		composer.WithStyles(m.theme.Prompt(m.icons)),
		composer.WithSize(m.width, m.areaHeight()),
		m.drafts(),
	)
	c.SetValue(quote(body))
	o := &composerOverlay{m: m, comp: c, head: "Comment on " + num, submit: (*panesModal).postComment}
	m.open(o)
	return tea.Batch(o.comp.Init(), o.comp.Focus())
}

// postComment adds the comment the composer holds to the conversation. The
// service shows it at once and the modal reads the pull request once GitHub
// answered.
func (m *panesModal) postComment(o *composerOverlay) tea.Cmd {
	body := strings.TrimSpace(o.comp.Value())
	if body == "" {
		return ui.Notify(toast.Info, "Nothing to send: the comment is empty.")
	}
	if cmd, refused := m.gate().Refuse(ui.ActComment, &m.detail.Issue); refused {
		return cmd
	}
	o.done = true
	op := m.svc.Comment(m.repo, m.number, body, m.conv.viewer)
	return m.sendOp(op, "comment on #"+strconv.Itoa(m.number))
}

// sendOp sends a change of the pull request, and reads it again once it went
// through or was rolled back.
func (m *panesModal) sendOp(op ui.Op, what string) tea.Cmd {
	about := core.Target{Repo: m.repo, Number: m.number}.String()
	do := ui.Do(m.sendCtx, ui.PullsTitle, ui.About(about, op), what)
	return tea.Batch(m.list.reload(), tea.Sequence(do, m.revalidate()))
}

// The fields of the editor that take the cursor.
const (
	editTitle = iota
	editBody
)

// descEditBodyMin is the least number of lines the body of the editor takes.
const descEditBodyMin = 3

// descEditOverlay edits the title and the description of the pull request. It
// types: it takes every key itself, and its own keys save and close it.
type descEditOverlay struct {
	m      *panesModal
	keys   descEditKeys
	titleF prompt.Model
	bodyF  composer.Model
	focus  int
	done   bool
}

// newDescEdit returns the editor, filled with the title and the description.
func newDescEdit(m *panesModal) *descEditOverlay {
	o := &descEditOverlay{m: m, keys: m.keys.descEdit}
	o.titleF = prompt.New(prompt.WithMode(prompt.SingleLine), prompt.WithTitle("Title"))
	o.bodyF = composer.New("", composer.WithOptional(), composer.WithTitle("Description"),
		composer.WithKeyMap(composer.KeyMap{Edit: o.keys.Edit}))
	st := m.theme.Prompt(m.icons)
	o.titleF.SetStyles(st)
	o.bodyF.SetStyles(st)
	o.titleF.SetValue(m.detail.Title)
	o.bodyF.SetValue(m.detail.Body)
	return o
}

func (o *descEditOverlay) title() string { return "Edit #" + strconv.Itoa(o.m.number) }

func (o *descEditOverlay) kind() panes.OverlayKind { return panes.Viewer }
func (o *descEditOverlay) items() int              { return 0 }
func (o *descEditOverlay) text() int               { return 0 }
func (o *descEditOverlay) clear() bool             { return false }
func (o *descEditOverlay) closed() bool            { return o.done }

// typing reports that the overlay takes the keys of the app's actions as text.
func (o *descEditOverlay) typing() bool { return true }

func (o *descEditOverlay) layers() []keyhelp.Layer {
	k := o.keys
	k.Edit.SetEnabled(k.Edit.Enabled() && o.focus == editBody)
	return []keyhelp.Layer{ui.ContextHelp(ctxDescEdit, k, true)}
}

func (o *descEditOverlay) view(w, h int) string {
	o.titleF.SetSize(w, prompt.SingleLineHeight)
	o.bodyF.SetSize(w, max(h-prompt.SingleLineHeight, descEditBodyMin))
	lines := append(strings.Split(o.titleF.View(), "\n"), strings.Split(o.bodyF.View(), "\n")...)
	return strings.Join(termtext.PadLines(lines, w, h), "\n")
}

func (o *descEditOverlay) update(msg tea.Msg) tea.Cmd {
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
func (o *descEditOverlay) forward(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	if o.focus == editTitle {
		o.titleF, cmd = o.titleF.Update(msg)
	} else {
		o.bodyF, cmd = o.bodyF.Update(msg)
	}
	return cmd
}

// switchField moves the cursor to the other field.
func (o *descEditOverlay) switchField() tea.Cmd {
	if o.focus == editTitle {
		o.focus = editBody
		o.titleF.Blur()
		return o.bodyF.Focus()
	}
	o.focus = editTitle
	o.bodyF.Blur()
	return o.titleF.Focus()
}

// save sends the title and the description as they are. The service shows
// them at once, so the header and the pane do too.
func (o *descEditOverlay) save() tea.Cmd {
	m := o.m
	title := strings.TrimSpace(o.titleF.Value())
	if title == "" {
		return ui.Notify(toast.Info, "A pull request needs a title.")
	}
	if cmd, refused := m.gate().Refuse(ui.ActDraft, &m.detail.Issue); refused {
		return cmd
	}
	o.done = true
	op := m.svc.EditPull(m.repo, m.number, title, o.bodyF.Value())
	if d, ok := m.svc.CachedGet(m.repo, m.number); ok {
		m.detail = d
	}
	return m.sendOp(op, "edit #"+strconv.Itoa(m.number))
}
