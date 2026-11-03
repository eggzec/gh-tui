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

// Title is the heading of the frame.
func (o *ComposerOverlay) Title() string { return o.Head }

// Kind says the frame is a viewer.
func (o *ComposerOverlay) Kind() panes.OverlayKind { return panes.Viewer }

// Items is zero: the composer has no rows.
func (o *ComposerOverlay) Items() int { return 0 }

// Text is zero: the composer has no text to find in.
func (o *ComposerOverlay) Text() int { return 0 }

// Clear reports false: the composer has no find or filter to clear.
func (o *ComposerOverlay) Clear() bool { return false }

// Closed reports whether the overlay is done.
func (o *ComposerOverlay) Closed() bool { return o.done }

// Typing reports that the overlay takes the keys of the app's actions as text.
func (o *ComposerOverlay) Typing() bool { return true }

// Layers lists the keys of the composer for the help.
func (o *ComposerOverlay) Layers() []keyhelp.Layer {
	return []keyhelp.Layer{ui.ContextHelp("composer", &o.Comp, true)}
}

// View draws the composer at the size of the frame.
func (o *ComposerOverlay) View(w, h int) string {
	o.Comp.SetSize(w, h)
	return o.Comp.View()
}

// Update passes a message to the composer and submits it when it is sent.
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
