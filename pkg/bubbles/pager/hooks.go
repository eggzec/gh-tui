package pager

import (
	"slices"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/pkg/bubbles/keymap"
)

// Handler sees a key that the pager does not bind, and says whether it
// took it. It gets the pager as it is, and returns it, changed or not, with
// the command to run; a pager that sees the key handled stops there.
type Handler func(m Model, k tea.KeyPressMsg) (Model, tea.Cmd, bool)

// OptionID names an option that the option mode offers.
type OptionID int

// The options of the option mode, each set by the key of its binding in
// [OptionKeyMap].
const (
	OptionChop OptionID = iota
	OptionNumbers
	OptionSqueeze
	OptionSmartCase
	OptionIgnoreCase
)

// WithHandler sets what sees the keys that the pager does not bind, such
// as those of a host that adds its own. The pager's own keys keep their
// order, so the handler never shadows one. While the host captures keys,
// as [WithHelp] says, the handler sees all of them and the pager's own
// keys are not matched.
func WithHandler(h Handler) Option {
	return func(s *settings) {
		s.handler = h
	}
}

// WithHelp adds the host's keys to the help and tells the pager when the
// host takes every key. extra lists the bindings help shows after the
// pager's own, in the short help and as one more row of the full help, and
// may be nil. capturing reports whether the host takes every key, such as
// while it has a prompt open: the pager then leaves its own keys out of the
// help as it does for its prompt, and gives the keys to the handler.
func WithHelp(extra func() []key.Binding, capturing func() bool) Option {
	return func(s *settings) {
		s.extraHelp, s.hostCapturing = extra, capturing
	}
}

// WithOptions sets which options the option mode offers, in the order
// help lists them. The others read as no such option. By default it offers
// all of them.
func WithOptions(ids ...OptionID) Option {
	return func(s *settings) {
		s.offered = slices.Clone(ids)
		s.restricted = true
	}
}

// offers reports whether the option mode offers id.
func (s settings) offers(id OptionID) bool {
	return !s.restricted || slices.Contains(s.offered, id)
}

// hostTakes reports whether the host takes every key.
func (s settings) hostTakes() bool {
	return s.hostCapturing != nil && s.hostCapturing()
}

// Choosing reports whether the pager waits for the name of an option, the
// key after the one bound to Option. It takes the next key then, as it does
// while [Model.Capturing].
func (m Model) Choosing() bool { return m.opt }

// Forward gives the pager a message that is not a key, such as the result of
// a command it started, for the host to pass on without the open prompt
// taking it, as it does a paste. The messages that aren't the pager's own
// are ignored. It returns the command to run.
func (m *Model) Forward(msg tea.Msg) tea.Cmd {
	out, cmd, ok := m.own(msg)
	if !ok {
		return nil
	}
	*m = out
	m.fitRendered(false)
	return cmd
}

// optionKeys returns the option bindings that the option mode offers.
func (m Model) optionKeys() []key.Binding {
	o := m.keys.Options
	var bs []key.Binding
	for _, e := range []struct {
		id OptionID
		b  key.Binding
	}{
		{OptionChop, o.Chop},
		{OptionNumbers, o.LineNumbers},
		{OptionSqueeze, o.Squeeze},
		{OptionSmartCase, o.SmartCase},
		{OptionIgnoreCase, o.IgnoreCase},
	} {
		if m.offers(e.id) {
			bs = append(bs, e.b)
		}
	}
	return bs
}

// restrictOptions disables the option keys that the option mode doesn't
// offer.
func (m *Model) restrictOptions() {
	o := &m.keys.Options
	for _, e := range []struct {
		id OptionID
		b  *key.Binding
	}{
		{OptionChop, &o.Chop},
		{OptionNumbers, &o.LineNumbers},
		{OptionSqueeze, &o.Squeeze},
		{OptionSmartCase, &o.SmartCase},
		{OptionIgnoreCase, &o.IgnoreCase},
	} {
		if !m.offers(e.id) {
			keymap.Enable(e.b, false)
		}
	}
}

// hostHelp returns the host's bindings for the help.
func (m Model) hostHelp() []key.Binding {
	if m.extraHelp == nil {
		return nil
	}
	return m.extraHelp()
}
