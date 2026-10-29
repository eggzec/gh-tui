package pulls

import (
	"context"
	"log/slog"
	"slices"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/optimistic"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/composer"
	"github.com/eggzec/gh-tui/pkg/bubbles/keyhelp"
	"github.com/eggzec/gh-tui/pkg/bubbles/keymap"
	"github.com/eggzec/gh-tui/pkg/bubbles/prompt"
	"github.com/eggzec/gh-tui/pkg/bubbles/toast"
)

// mergeDialogKeys are the keys of the dialog that merges a pull request.
// The dialog types, so they are control keys and the arrows.
type mergeDialogKeys struct {
	Submit     key.Binding `keymap:"submit" help:"merge"`
	Cancel     key.Binding `keymap:"cancel" help:"cancel"`
	Edit       key.Binding `keymap:"edit" help:"editor"`
	NextField  key.Binding `keymap:"next_field" help:"next field"`
	PrevField  key.Binding `keymap:"prev_field" help:"previous field"`
	PrevMethod key.Binding `keymap:"prev_method" help:"previous method"`
	NextMethod key.Binding `keymap:"next_method" help:"next method"`
}

// newMergeDialogKeys returns the keys of the merge dialog from keys.
func newMergeDialogKeys(keys config.Keymap) mergeDialogKeys {
	var k mergeDialogKeys
	keymap.Fill(&k, ui.Lookup(keys, "merge_dialog"))
	return k
}

// ShortHelp implements help.KeyMap.
func (k mergeDialogKeys) ShortHelp() []key.Binding {
	return []key.Binding{k.Submit, k.Cancel, k.Edit, k.NextField, k.PrevField, k.PrevMethod, k.NextMethod}
}

// FullHelp implements help.KeyMap.
func (k mergeDialogKeys) FullHelp() [][]key.Binding { return [][]key.Binding{k.ShortHelp()} }

// mergeAsk is what the merge dialog needs to make the merge it asks for:
// the change c that the key asked of a pull request, and what confirms it
// (see keyMap.confirmed). drafts keeps the message the user changed under
// draftKey, and back is the modal the dialog opened over, or nil when it
// opened from the list.
type mergeAsk struct {
	ctx      context.Context
	svc      Service
	keys     keyMap
	c        change
	key      tea.KeyPressMsg
	now      func() (core.PullRequestDetail, ui.Gate, bool)
	send     func(c change, op *optimistic.Op) tea.Cmd
	again    *rereader
	drafts   composer.Drafts
	draftKey string
	icons    ui.Icons
	back     ui.Modal
}

// mergeDraftKey is the key of the draft of the merge message of pull
// request number of repo, after scope, the host and account.
func mergeDraftKey(scope string, repo core.RepoRef, number int) string {
	return strings.Join([]string{scope, repo.String(), strconv.Itoa(number), "merge"}, "/")
}

// mergeTextMsg carries GitHub's messages for the merge of dialog, and the
// draft kept of an earlier one.
type mergeTextMsg struct {
	dialog *mergeDialog
	texts  core.MergeTexts
	err    error
	draft  string
}

// The rows of the dialog that take the cursor.
const (
	focusMethod = iota
	focusTitle
	focusBody
)

// mergeBodyLines is how many lines the body of the message takes.
const mergeBodyLines = 8

// mergeDialog picks the method of a merge and edits its commit message,
// which starts as GitHub's for the method. A field the user didn't change
// follows the method; one they changed stays as they wrote it.
type mergeDialog struct {
	ask     mergeAsk
	keys    mergeDialogKeys
	methods []core.MergeMethod
	method  int
	focus   int
	title   prompt.Model
	body    composer.Model
	texts   core.MergeTexts
	// filled is what the title and body were last filled with, so that a
	// value that differs from it was edited.
	filled [2]string
	loaded bool
	note   string
	// done is set once the dialog merged or closed.
	done          bool
	theme         ui.Theme
	icons         ui.Icons
	width, height int
}

// newMergeDialog returns the dialog that asks for the merge of a.
func newMergeDialog(a mergeAsk, keys mergeDialogKeys) *mergeDialog {
	d := &mergeDialog{ask: a, keys: keys, methods: a.c.methods, icons: a.icons}
	for i, m := range d.methods {
		if m == a.c.of.method {
			d.method = i
		}
	}
	if !d.choosable() {
		// The method row takes no cursor, so the title does.
		d.focus = focusTitle
		d.keys.PrevMethod.SetEnabled(false)
		d.keys.NextMethod.SetEnabled(false)
	}
	d.title = prompt.New(prompt.WithMode(prompt.SingleLine), prompt.WithTitle("Title"),
		prompt.WithPlaceholder("GitHub's default title"))
	d.body = composer.New("", composer.WithOptional(), composer.WithTitle("Body"),
		composer.WithKeyMap(composer.KeyMap{Edit: keys.Edit}))
	return d
}

// Init reads the draft and GitHub's messages, in one command.
func (d *mergeDialog) Init() tea.Cmd {
	a := d.ask
	load := func() tea.Msg {
		var draft string
		if a.drafts != nil {
			draft, _ = a.drafts.Load(a.draftKey)
		}
		texts, err := a.svc.MergeTexts(a.ctx, a.c.of.repo, a.c.of.number)
		return mergeTextMsg{dialog: d, texts: texts, err: err, draft: draft}
	}
	cmds := []tea.Cmd{d.body.Init(), load}
	if d.focus == focusTitle {
		cmds = append(cmds, d.title.Focus())
	}
	return tea.Batch(cmds...)
}

// choosable reports whether the user may pick the method: not when a merge
// queue sets it, and not when the repository allows only one.
func (d *mergeDialog) choosable() bool {
	return !d.ask.c.queued && len(d.methods) > 1
}

// Title implements ui.Modal: it says "when ready" for an auto-merge, which
// waits for the checks.
func (d *mergeDialog) Title() string {
	t := "Merge #" + strconv.Itoa(d.ask.c.of.number)
	if d.ask.c.of.kind == mergeAuto {
		t += " when ready"
	}
	return t
}

// current is the method picked.
func (d *mergeDialog) current() core.MergeMethod {
	if len(d.methods) == 0 {
		return d.ask.c.of.method
	}
	return d.methods[d.method]
}

// hasMessage reports whether the method picked takes a message: rebasing
// keeps each commit's own. A merge queue takes the message whatever the
// method shown.
func (d *mergeDialog) hasMessage() bool { return d.ask.c.queued || d.current() != core.MergeRebase }

// Update implements ui.Modal.
func (d *mergeDialog) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case ui.SettingsMsg:
		msg.ReloadIcons(&d.icons)
		d.SetTheme(d.theme)
		return nil
	case mergeTextMsg:
		if msg.dialog == d {
			d.fill(msg)
		}
		return nil
	case tea.KeyPressMsg:
		return d.press(msg)
	}
	// The fields' own messages, such as the editor's result, carry the ID of
	// the field they are for, so each takes only its own.
	var tc, bc tea.Cmd
	d.title, tc = d.title.Update(msg)
	d.body, bc = d.body.Update(msg)
	return tea.Batch(tc, bc)
}

// fill fills the fields with GitHub's message for the method, and then
// with the draft, which counts as edited.
func (d *mergeDialog) fill(msg mergeTextMsg) {
	d.loaded = true
	if msg.err != nil {
		// Say's text is the cause, with no hint that names a key the
		// dialog doesn't take.
		cause, _ := ui.Say(core.Explain("read GitHub's merge message", msg.err), ui.Voice{})
		d.note = "Couldn't read GitHub's message: " + strings.TrimSuffix(cause, ".") + ". An empty title is GitHub's default."
	} else {
		d.texts = msg.texts
	}
	d.prefill()
	// The draft keeps the method it was written for, then the title and the
	// body.
	if method, text, ok := strings.Cut(msg.draft, "\n"); ok {
		if i := slices.Index(d.methods, core.MergeMethod(method)); i >= 0 && d.choosable() {
			d.method = i
		}
		title, body, _ := strings.Cut(text, "\n")
		d.title.SetValue(title)
		d.body.SetValue(body)
	}
}

// textFor is GitHub's message for method m.
func (d *mergeDialog) textFor(m core.MergeMethod) (core.MergeMessage, bool) {
	switch m {
	case core.MergeSquash:
		return d.texts.Squash, true
	case core.MergeCommit:
		return d.texts.Merge, true
	case core.MergeRebase:
	}
	return core.MergeMessage{}, false
}

// prefill fills the fields the user didn't edit with GitHub's message for
// the method picked.
func (d *mergeDialog) prefill() {
	m := d.current()
	if d.ask.c.queued && m == core.MergeRebase {
		m = core.MergeSquash
	}
	msg, ok := d.textFor(m)
	if !ok {
		return
	}
	if d.title.Value() == d.filled[0] {
		d.title.SetValue(msg.Title)
	}
	if d.body.Value() == d.filled[1] {
		d.body.SetValue(msg.Body)
	}
	d.filled = [2]string{msg.Title, msg.Body}
}

// edited reports whether the user changed the title or the body.
func (d *mergeDialog) edited() bool {
	return d.title.Value() != d.filled[0] || d.body.Value() != d.filled[1]
}

// message is the commit message to merge with.
func (d *mergeDialog) message() core.MergeMessage {
	if !d.hasMessage() {
		return core.MergeMessage{}
	}
	body := d.body.Value()
	return core.MergeMessage{Title: d.title.Value(), Body: body, SendBody: body != "" || body != d.filled[1]}
}

// press takes a key: the dialog's own first, then the focused row's.
func (d *mergeDialog) press(k tea.KeyPressMsg) tea.Cmd {
	if d.done {
		return nil
	}
	switch {
	case key.Matches(k, d.keys.Cancel):
		return d.cancel()
	case !d.loaded:
		// The fields fill once GitHub's message is read.
		return nil
	case key.Matches(k, d.keys.Submit):
		return d.submit()
	case key.Matches(k, d.keys.NextField):
		return d.move(1)
	case key.Matches(k, d.keys.PrevField):
		return d.move(-1)
	}
	var cmd tea.Cmd
	switch d.focus {
	case focusMethod:
		switch {
		case key.Matches(k, d.keys.NextMethod):
			d.step(1)
		case key.Matches(k, d.keys.PrevMethod):
			d.step(-1)
		}
	case focusTitle:
		d.title, cmd = d.title.Update(k)
	case focusBody:
		d.body, cmd = d.body.Update(k)
	}
	return cmd
}

// step picks the method delta places on, round the end.
func (d *mergeDialog) step(delta int) {
	if n := len(d.methods); d.choosable() {
		d.method = (d.method + delta + n) % n
		d.prefill()
	}
}

// move moves the cursor delta rows on, round the end; the fields take it
// only for a method that has a message.
func (d *mergeDialog) move(delta int) tea.Cmd {
	var stops []int
	if d.choosable() {
		stops = append(stops, focusMethod)
	}
	if d.hasMessage() {
		stops = append(stops, focusTitle, focusBody)
	}
	if len(stops) == 0 {
		return nil
	}
	i := max(slices.Index(stops, d.focus), 0)
	d.focus = stops[(i+delta+len(stops))%len(stops)]
	d.title.Blur()
	d.body.Blur()
	var cmd tea.Cmd
	switch d.focus {
	case focusTitle:
		cmd = d.title.Focus()
	case focusBody:
		cmd = d.body.Focus()
	}
	return cmd
}

// close closes the dialog, which shows the modal it opened over.
func (d *mergeDialog) close() tea.Cmd {
	d.done = true
	return ui.CloseModal(d)
}

// cancel closes the dialog, and keeps the message as a draft if the user
// changed it.
func (d *mergeDialog) cancel() tea.Cmd {
	return tea.Sequence(d.close(), d.keep())
}

// submit closes the dialog and confirms the merge with the method and the
// message as they are. The message is kept as a draft first, so that it
// isn't lost if the merge doesn't go through, which a merge that did
// deletes.
func (d *mergeDialog) submit() tea.Cmd {
	a := d.ask
	c := a.c
	c.of.method = d.current()
	c.msg = d.message()
	run := a.keys.confirmed(a.svc, c, a.key, a.now, a.send, a.again)
	return tea.Sequence(d.close(), d.keep(), run())
}

// keep returns the command that saves the draft, which toasts a failure to.
func (d *mergeDialog) keep() tea.Cmd {
	save, ctx := d.saver(), d.ask.ctx
	return func() tea.Msg {
		if err := save(); err != nil {
			slog.WarnContext(ctx, "merge message draft not saved", "span", "tui", "err", ui.ShortPath(err.Error()))
			return ui.Notify(toast.Warning, "Couldn't save the draft of the merge message.")()
		}
		return nil
	}
}

// saver returns what saves the message as a draft if the user changed it,
// and drops the draft otherwise. It takes the text as it is now, so it may
// run later.
func (d *mergeDialog) saver() func() error {
	drafts, name := d.ask.drafts, d.ask.draftKey
	switch {
	case drafts == nil || !d.loaded:
		return func() error { return nil }
	case d.edited():
		text := strings.Join([]string{string(d.current()), d.title.Value(), d.body.Value()}, "\n")
		return func() error { return drafts.Save(name, text) }
	}
	return func() error { return drafts.Delete(name) }
}

// Save implements ui.Saver, for a quit while the dialog is open.
func (d *mergeDialog) Save() error { return d.saver()() }

// View implements ui.Modal: the methods, a line that says what is going
// on, and the fields of the message.
func (d *mergeDialog) View() string {
	if d.width <= 0 || d.height <= 0 {
		return ""
	}
	lines := []string{ansi.Truncate(d.methodRow(), d.width, d.icons.OrUnicode().Ellipsis)}
	status := d.note
	switch {
	case !d.loaded:
		status = "Reading GitHub's message" + d.icons.OrUnicode().Ellipsis
	case !d.hasMessage():
		status = "Rebasing keeps each commit's message."
	case status == "":
		status = d.ask.c.note
	}
	lines = append(lines, d.theme.Muted.Render(ansi.Truncate(status, d.width, d.icons.OrUnicode().Ellipsis)))
	if d.loaded && d.hasMessage() {
		lines = append(lines, strings.Split(d.title.View(), "\n")...)
		lines = append(lines, strings.Split(d.body.View(), "\n")...)
	}
	return strings.Join(ui.PadLines(lines, d.width, d.height), "\n")
}

// methodRow names the methods, the one picked in the accent, and marks the
// row while it has the cursor.
func (d *mergeDialog) methodRow() string {
	mark := "  "
	if !d.choosable() {
		// Shown, but not the user's to pick.
		parts := make([]string, len(d.methods))
		for i, m := range d.methods {
			parts[i] = methodName(m)
		}
		if d.ask.c.queued {
			parts = []string{"set by the merge queue"}
		}
		return d.theme.Muted.Render(mark + "Method " + strings.Join(parts, ", "))
	}
	if d.focus == focusMethod {
		mark = d.theme.Accent.Render("> ")
	}
	parts := make([]string, len(d.methods))
	for i, m := range d.methods {
		name := methodName(m)
		if i == d.method {
			parts[i] = d.theme.Accent.Bold(true).Render("[" + name + "]")
		} else {
			parts[i] = d.theme.Muted.Render(" " + name + " ")
		}
	}
	return mark + "Method " + strings.Join(parts, " ")
}

// methodName names method m as the dialog shows it.
func methodName(m core.MergeMethod) string {
	switch m {
	case core.MergeCommit:
		return "merge commit"
	case core.MergeSquash:
		return "squash"
	case core.MergeRebase:
		return "rebase"
	}
	return string(m)
}

// SetSize implements ui.Modal.
func (d *mergeDialog) SetSize(width, height int) {
	d.width, d.height = max(width, 0), max(height, 0)
	d.title.SetSize(d.width, prompt.SingleLineHeight)
	d.body.SetSize(d.width, max(d.height-2-prompt.SingleLineHeight, 3))
}

// Fit implements ui.Fitter: as wide as a commit message reads well, and as
// high as the rows need.
func (d *mergeDialog) Fit(maxWidth, maxHeight int) (width, height int) {
	return min(maxWidth, 76), min(maxHeight, 2+prompt.SingleLineHeight+mergeBodyLines)
}

// SetTheme implements ui.Modal.
func (d *mergeDialog) SetTheme(t ui.Theme) {
	d.theme = t
	st := t.Prompt(d.icons)
	d.title.SetStyles(st)
	d.body.SetStyles(st)
}

// Commands implements ui.Commanded: the dialog takes only the commands
// that act on the app.
func (d *mergeDialog) Commands() []string { return nil }

// KeyLayers implements ui.Keyed: the dialog's keys, those of the methods
// while the cursor is on them and the editor's while it is in the body.
func (d *mergeDialog) KeyLayers() []keyhelp.Layer {
	k := d.keys
	onMethod := d.focus == focusMethod && len(d.methods) > 1
	k.PrevMethod.SetEnabled(k.PrevMethod.Enabled() && onMethod)
	k.NextMethod.SetEnabled(k.NextMethod.Enabled() && onMethod)
	k.Edit.SetEnabled(k.Edit.Enabled() && d.focus == focusBody)
	k.NextField.SetEnabled(k.NextField.Enabled() && d.hasMessage())
	k.PrevField.SetEnabled(k.PrevField.Enabled() && d.hasMessage())
	return []keyhelp.Layer{ui.ContextHelp("merge_dialog", k, true)}
}

// forgetting returns op, which also drops the draft under name of drafts
// once it went through. Without drafts it is op itself.
func forgetting(op ui.Op, drafts composer.Drafts, name string) ui.Op {
	if drafts == nil {
		return op
	}
	return forgetOp{op: op, drafts: drafts, key: name}
}

type forgetOp struct {
	op     ui.Op
	drafts composer.Drafts
	key    string
}

func (o forgetOp) Do(ctx context.Context) error {
	err := o.op.Do(ctx)
	if err == nil {
		_ = o.drafts.Delete(o.key)
	}
	return err
}
