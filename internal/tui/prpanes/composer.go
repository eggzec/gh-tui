package prpanes

import (
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/composer"
	"github.com/eggzec/gh-tui/pkg/bubbles/keyhelp"
	"github.com/eggzec/gh-tui/pkg/bubbles/panes"
)

// ComposerOverlay takes the summary of a review, or the text of a comment. It
// is a viewer in the way it is placed, but it types: it takes every key itself,
// and its own keys close it. Head is what its frame says, and Submit what its
// submit key does.
type ComposerOverlay struct {
	Comp    composer.Model
	Verdict int
	Head    string
	// Busy, when set, says that a send is in flight: the text stays as it is
	// until the answer comes, and the overlay does not close.
	Busy   func() bool
	Submit func(*ComposerOverlay) tea.Cmd
	done   bool
}

// Close ends the overlay, which takes no more keys.
func (o *ComposerOverlay) Close() { o.done = true }

func (o *ComposerOverlay) busy() bool { return o.Busy != nil && o.Busy() }

func (o *ComposerOverlay) Title() string           { return o.Head }
func (o *ComposerOverlay) Kind() panes.OverlayKind { return panes.Viewer }
func (o *ComposerOverlay) Items() int              { return 0 }
func (o *ComposerOverlay) Text() int               { return 0 }
func (o *ComposerOverlay) Clear() bool             { return false }
func (o *ComposerOverlay) Closed() bool            { return o.done }

// Typing reports that the overlay takes the keys of the app's actions as text.
func (o *ComposerOverlay) Typing() bool { return true }

func (o *ComposerOverlay) Layers() []keyhelp.Layer {
	return []keyhelp.Layer{ui.ContextHelp("composer", &o.Comp, true)}
}

func (o *ComposerOverlay) View(w, h int) string {
	o.Comp.SetSize(w, h)
	return o.Comp.View()
}

func (o *ComposerOverlay) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case composer.SubmitMsg:
		if msg.ID == o.Comp.ID() {
			return o.Submit(o)
		}
		return nil
	case composer.CancelMsg:
		if msg.ID == o.Comp.ID() && !o.busy() {
			o.done = true
		}
		return nil
	case tea.KeyPressMsg, tea.PasteMsg:
		// GitHub has the review: the summary stays until it answers.
		if o.busy() {
			return nil
		}
	}
	var cmd tea.Cmd
	o.Comp, cmd = o.Comp.Update(msg)
	return cmd
}
