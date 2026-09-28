package ui

import (
	"slices"
	"testing"

	"charm.land/bubbles/v2/key"

	"github.com/eggzec/gh-tui/pkg/bubbles/keyhelp"
	"github.com/eggzec/gh-tui/pkg/bubbles/keytest"
)

func hintBinding(desc string, keys ...string) key.Binding {
	return key.NewBinding(key.WithKeys(keys...), key.WithHelp(keys[0], desc))
}

func descs(bs []key.Binding) []string {
	out := make([]string, len(bs))
	for i, b := range bs {
		out[i] = b.Help().Desc
	}
	return out
}

func TestHints(t *testing.T) {
	back := hintBinding("back", "esc")
	next := hintBinding("next state", "]")
	off := hintBinding("merge", "m")
	off.SetEnabled(false)
	quit, find := hintBinding("quit", "q"), hintBinding("find", "/")
	unzoom := hintBinding("unzoom", "esc")
	// The app's next pane loses ] but keeps tab.
	pane := hintBinding("next pane", "tab", "]")
	layers := []keyhelp.Layer{
		// A section claims ] before the app.
		{Source: "claimed", Bindings: []key.Binding{next}},
		{Source: "app", Bindings: []key.Binding{quit, unzoom, find, pane}, Short: []key.Binding{unzoom, find, pane, quit}},
		{Source: "pulls", Bindings: []key.Binding{next, back, off}, Short: []key.Binding{next, off, back}},
		{Source: "input", Typing: true},
		{Source: "feed", Bindings: []key.Binding{hintBinding("down", "j", "down"), hintBinding("page", "f")}, Short: []key.Binding{hintBinding("down", "j", "down"), hintBinding("page", "f")}},
	}
	h := Hints{Layers: layers}
	// The innermost keys lead; esc goes to the app, a claimed key to its
	// twin, and f is typed into the input.
	if got, want := descs(h.ShortHelp()), []string{"down", "next state", "unzoom", "find", "next pane", "quit"}; !slices.Equal(got, want) {
		t.Errorf("ShortHelp = %q, want %q", got, want)
	}
	full := h.FullHelp()
	got := make([][]string, 0, len(full))
	for _, col := range full {
		got = append(got, descs(col))
	}
	want := [][]string{{"down"}, {"next state"}, {"quit", "unzoom", "find", "next pane"}}
	if !slices.EqualFunc(got, want, slices.Equal) {
		t.Errorf("FullHelp = %q, want %q", got, want)
	}
}

// A lead binding comes first where a key reaches it.
func TestHintsLead(t *testing.T) {
	unzoom, back := hintBinding("unzoom", "esc"), hintBinding("back", "esc")
	quit := hintBinding("quit", "q")
	layers := []keyhelp.Layer{
		{Source: "app", Bindings: []key.Binding{unzoom, quit}, Short: []key.Binding{unzoom, quit}},
		{Source: "pulls", Bindings: []key.Binding{back, hintBinding("close", "x")}, Short: []key.Binding{hintBinding("close", "x"), back}},
	}
	h := Hints{Layers: layers, Lead: []key.Binding{unzoom}}
	if got, want := descs(h.ShortHelp()), []string{"unzoom", "close", "quit"}; !slices.Equal(got, want) {
		t.Errorf("ShortHelp = %q, want %q", got, want)
	}
	off := unzoom
	off.SetEnabled(false)
	layers[0].Bindings[0], layers[0].Short[0] = off, off
	h = Hints{Layers: layers, Lead: []key.Binding{off}}
	if got, want := descs(h.ShortHelp()), []string{"close", "back", "quit"}; !slices.Equal(got, want) {
		t.Errorf("ShortHelp unzoomed = %q, want %q", got, want)
	}
}

// A short help may name a binding for what it does now.
func TestHintsRelabelled(t *testing.T) {
	reset := hintBinding("reset", "r")
	named := reset
	named.SetHelp("r", "reset filters")
	gone := hintBinding("remove", "x")
	gone.SetEnabled(false)
	h := Hints{Layers: []keyhelp.Layer{{Source: "form", Bindings: []key.Binding{reset, gone}, Short: []key.Binding{named, hintBinding("remove all", "x")}}}}
	if got, want := descs(h.ShortHelp()), []string{"reset filters"}; !slices.Equal(got, want) {
		t.Errorf("ShortHelp = %q, want %q", got, want)
	}
}

func TestConfirmKeysComplete(t *testing.T) {
	keytest.Complete(t, DefaultConfirmKeys())
	keytest.NoConflicts(t, DefaultConfirmKeys())
}
