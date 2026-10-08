package keymap_test

import (
	"slices"
	"testing"

	"charm.land/bubbles/v2/key"

	"github.com/eggzec/gh-tui/pkg/bubbles/keymap"
)

type scoped map[string][]string

func (s scoped) Of(action string) []string { return s[action] }

func (scoped) Scope() string { return "pulls" }

type pane struct {
	Merge key.Binding `keymap:"merge" help:"merge"`
	Star  key.Binding `keymap:"star" help:"star"`
	Gone  key.Binding `keymap:"gone" help:"gone"`
	Other key.Binding `keymap:"global.select" help:"open"`
}

func TestFillRecordsPaths(t *testing.T) {
	var p pane
	keymap.Fill(&p, scoped{"merge": {"m"}, "star": {}, "global.select": {"enter"}})
	for _, tt := range []struct {
		name string
		b    key.Binding
		want []string
	}{
		{"bare name gets its context", p.Merge, []string{"pulls.merge"}},
		{"another context's keeps its own", p.Other, []string{"global.select"}},
		{"unbound is found without keys", p.Star, []string{"pulls.star"}},
		{"not in the config is no action", p.Gone, nil},
	} {
		if got := keymap.Actions(tt.b); !slices.Equal(got, tt.want) {
			t.Errorf("%s: actions %v, want %v", tt.name, got, tt.want)
		}
	}
	// A copy with other words or state is the same binding.
	c := p.Merge
	c.SetHelp("m", "merge it")
	c.SetEnabled(false)
	if got := keymap.Actions(c); !slices.Equal(got, []string{"pulls.merge"}) {
		t.Errorf("copy: actions %v", got)
	}
}

func TestFillWithoutContextRecordsNames(t *testing.T) {
	var p pane
	keymap.Fill(&p, keymap.Func(func(a string) []string { return []string{a} }))
	if got := keymap.Actions(p.Merge); !slices.Equal(got, []string{"merge"}) {
		t.Errorf("actions %v, want the bare name", got)
	}
}

func TestJoinAndDeriveUnion(t *testing.T) {
	a := keymap.Record(key.NewBinding(key.WithKeys("q"), key.WithHelp("q", "quit")), "global.quit")
	b := keymap.Record(key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "close")), "global.dismiss")
	j := keymap.Join(a, b)
	if got := keymap.Actions(j); !slices.Equal(got, []string{"global.quit", "global.dismiss"}) {
		t.Errorf("Join actions %v", got)
	}
	d := keymap.Derive(key.NewBinding(key.WithKeys("x")), a)
	if got := keymap.Actions(d); !slices.Equal(got, []string{"global.quit"}) {
		t.Errorf("Derive actions %v", got)
	}
	if got := keymap.Actions(key.NewBinding(key.WithKeys("z"))); got != nil {
		t.Errorf("a binding nobody recorded has actions %v", got)
	}
}

func TestSharedKeysDoNotShareNames(t *testing.T) {
	quit := key.NewBinding(key.WithKeys("ctrl+c"), key.WithHelp("^c", "quit"))
	other := keymap.Record(key.NewBinding(key.WithKeys("q")), "global.quit")
	d := keymap.Derive(key.NewBinding(key.WithKeys(quit.Keys()...)), other)
	if got := keymap.Actions(d); !slices.Equal(got, []string{"global.quit"}) {
		t.Errorf("derived: %v", got)
	}
	if got := keymap.Actions(quit); got != nil {
		t.Errorf("the binding whose keys it shared got %v", got)
	}
	// Recording other actions on keys that already have names gives them
	// an array of their own.
	a := keymap.Record(key.NewBinding(key.WithKeys("a")), "x.a")
	b := keymap.Record(a, "x.b")
	if got := keymap.Actions(a); !slices.Equal(got, []string{"x.a"}) {
		t.Errorf("first: %v", got)
	}
	if got := keymap.Actions(b); !slices.Equal(got, []string{"x.a", "x.b"}) {
		t.Errorf("second: %v", got)
	}
}

func TestEnableKeepsUnboundOff(t *testing.T) {
	b := keymap.Unbound(key.NewBinding(key.WithHelp("", "next")))
	keymap.Enable(&b, true)
	if b.Enabled() {
		t.Error("a binding without keys was switched on")
	}
	k := key.NewBinding(key.WithKeys("n"), key.WithDisabled())
	keymap.Enable(&k, true)
	if !k.Enabled() {
		t.Error("a binding with keys stayed off")
	}
	keymap.Enable(&k, false)
	if k.Enabled() {
		t.Error("a binding switched off stayed on")
	}
}
