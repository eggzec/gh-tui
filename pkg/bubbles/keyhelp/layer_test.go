package keyhelp

import (
	"testing"

	"charm.land/bubbles/v2/key"
)

func layerBind(k, h string) key.Binding { return key.NewBinding(key.WithKeys(k), key.WithHelp(k, h)) }

func baseLayer() Layer {
	return Layer{
		Source:   "src",
		Context:  "ctx",
		Bindings: []key.Binding{layerBind("a", "alpha"), layerBind("b", "beta")},
		Actions:  [][]string{{"x.a"}},
		Short:    []key.Binding{layerBind("a", "alpha")},
	}
}

func TestLayerEqual(t *testing.T) {
	t.Parallel()
	disabled := func(l Layer) Layer {
		l.Bindings[0].SetEnabled(false)
		return l
	}
	for _, tc := range []struct {
		name   string
		change func(Layer) Layer
		want   bool
	}{
		{"clone", func(l Layer) Layer { return l }, true},
		{"source", func(l Layer) Layer { l.Source = "other"; return l }, false},
		{"typing", func(l Layer) Layer { l.Typing = true; return l }, false},
		{"key", func(l Layer) Layer { l.Bindings[1] = layerBind("c", "beta"); return l }, false},
		{"help", func(l Layer) Layer { l.Bindings[1] = layerBind("b", "gamma"); return l }, false},
		{"enabled", disabled, false},
		{"short", func(l Layer) Layer { l.Short = nil; return l }, false},
		{"context", func(l Layer) Layer { l.Context = "other"; return l }, true},
		{"actions", func(l Layer) Layer { l.Actions = nil; return l }, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			a := baseLayer()
			b := tc.change(a.Clone())
			if got := a.Equal(b); got != tc.want {
				t.Errorf("Equal = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestLayerCloneIndependent(t *testing.T) {
	t.Parallel()
	a := baseLayer()
	c := a.Clone()
	c.Bindings[0] = layerBind("z", "zed")
	c.Short[0] = layerBind("z", "zed")
	if a.Bindings[0].Help().Key != "a" || a.Short[0].Help().Key != "a" {
		t.Error("mutating the clone changed the original")
	}
}
