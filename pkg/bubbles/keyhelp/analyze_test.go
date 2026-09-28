package keyhelp

import (
	"slices"
	"testing"

	"charm.land/bubbles/v2/key"
)

func bind(desc string, keys ...string) key.Binding {
	return key.NewBinding(key.WithKeys(keys...), key.WithHelp(keys[0], desc))
}

func off(b key.Binding) key.Binding {
	b.SetEnabled(false)
	return b
}

func TestAnalyze(t *testing.T) {
	// want is a row as the test states it: its description, status, and
	// each lost key with the description of the binding that gets it.
	type loss struct {
		key, by, source string
		status          Status
	}
	type want struct {
		desc   string
		status Status
		lost   []loss
	}
	tests := []struct {
		name   string
		layers []Layer
		want   []want
	}{
		{
			name:   "one binding wins its keys",
			layers: []Layer{{Source: "list", Bindings: []key.Binding{bind("down", "down", "j"), bind("up", "up", "k")}}},
			want:   []want{{desc: "down", status: Active}, {desc: "up", status: Active}},
		},
		{
			name: "a later binding of the layer conflicts",
			layers: []Layer{{Source: "pager", Bindings: []key.Binding{
				bind("cancel", "esc"), bind("close", "q", "esc"),
			}}},
			want: []want{
				{desc: "cancel", status: Active},
				{desc: "close", status: Conflict, lost: []loss{{key: "esc", by: "cancel", source: "pager", status: Conflict}}},
			},
		},
		{
			name: "a binding of a later layer is shadowed",
			layers: []Layer{
				{Source: "app", Bindings: []key.Binding{bind("back", "esc")}},
				{Source: "list", Bindings: []key.Binding{bind("clear", "esc", "ctrl+l")}},
			},
			want: []want{
				{desc: "back", status: Active},
				{desc: "clear", status: Shadowed, lost: []loss{{key: "esc", by: "back", source: "app", status: Shadowed}}},
			},
		},
		{
			name: "a conflict outranks a shadow",
			layers: []Layer{
				{Source: "app", Bindings: []key.Binding{bind("quit", "q")}},
				{Source: "list", Bindings: []key.Binding{bind("back", "esc"), bind("close", "q", "esc")}},
			},
			want: []want{
				{desc: "quit", status: Active},
				{desc: "back", status: Active},
				{desc: "close", status: Conflict, lost: []loss{
					{key: "q", by: "quit", source: "app", status: Shadowed},
					{key: "esc", by: "back", source: "list", status: Conflict},
				}},
			},
		},
		{
			name: "printable keys under a typing layer are typed",
			layers: []Layer{
				{Source: "finder", Typing: true, Bindings: []key.Binding{bind("open", "enter"), bind("find", "f")}},
				{Source: "list", Bindings: []key.Binding{bind("down", "down", "j"), bind("reload", "r"), bind("page", "space")}},
			},
			want: []want{
				{desc: "open", status: Active},
				// A typing layer's own bindings come before its typing.
				{desc: "find", status: Active},
				{desc: "down", status: Active, lost: []loss{{key: "j", source: "finder", status: Typed}}},
				{desc: "reload", status: Typed, lost: []loss{{key: "r", source: "finder", status: Typed}}},
				{desc: "page", status: Typed, lost: []loss{{key: "space", source: "finder", status: Typed}}},
			},
		},
		{
			name: "a binding held before the typing layer still wins",
			layers: []Layer{
				{Source: "app", Bindings: []key.Binding{bind("help", "?")}},
				{Source: "finder", Typing: true},
				{Source: "list", Bindings: []key.Binding{bind("help too", "?")}},
			},
			want: []want{
				{desc: "help", status: Active},
				{desc: "help too", status: Shadowed, lost: []loss{{key: "?", by: "help", source: "app", status: Shadowed}}},
			},
		},
		{
			name: "a disabled binding takes no key",
			layers: []Layer{{Source: "pager", Bindings: []key.Binding{
				off(bind("cancel", "esc")), bind("close", "q", "esc"),
			}}},
			want: []want{{desc: "cancel", status: Disabled}, {desc: "close", status: Active}},
		},
		{
			name: "every binding is listed, twins too",
			layers: []Layer{
				{Source: "a", Bindings: []key.Binding{bind("down", "j")}},
				{Source: "b", Bindings: []key.Binding{bind("down", "j")}},
			},
			want: []want{
				{desc: "down", status: Active},
				{desc: "down", status: Shadowed, lost: []loss{{key: "j", by: "down", source: "a", status: Shadowed}}},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rows := Analyze(tt.layers)
			got := make([]want, len(rows))
			for i, r := range rows {
				got[i] = want{desc: r.Binding.Help().Desc, status: r.Status}
				for _, l := range r.Lost {
					got[i].lost = append(got[i].lost, loss{key: l.Key, by: l.By.Help().Desc, source: l.Source, status: l.Status})
				}
			}
			if !slices.EqualFunc(got, tt.want, func(a, b want) bool {
				return a.desc == b.desc && a.status == b.status && slices.Equal(a.lost, b.lost)
			}) {
				t.Errorf("Analyze() =\n%+v\nwant\n%+v", got, tt.want)
			}
		})
	}
}

// A row keeps its binding whole, with every key, and its layer.
func TestAnalyzeKeepsBindings(t *testing.T) {
	rows := Analyze([]Layer{
		{Source: "a", Bindings: []key.Binding{bind("x", "x")}},
		{Source: "b", Bindings: []key.Binding{bind("down", "down", "j", "ctrl+n")}},
	})
	r := rows[1]
	if r.Source != "b" || r.Layer != 1 || !slices.Equal(r.Binding.Keys(), []string{"down", "j", "ctrl+n"}) {
		t.Errorf("row = %+v, want b's down with all three keys", r)
	}
}

func TestPrintable(t *testing.T) {
	for k, want := range map[string]bool{
		"a": true, "?": true, "é": true, "space": true, "G": true,
		"enter": false, "ctrl+r": false, "up": false, "shift+tab": false, "f1": false, "": false,
	} {
		if got := Printable(k); got != want {
			t.Errorf("Printable(%q) = %v, want %v", k, got, want)
		}
	}
}

func TestStatusString(t *testing.T) {
	for s, want := range map[Status]string{
		Active: "active", Disabled: "disabled", Shadowed: "shadowed", Conflict: "conflict", Typed: "typed", Status(99): "unknown",
	} {
		if got := s.String(); got != want {
			t.Errorf("Status(%d).String() = %q, want %q", s, got, want)
		}
	}
}
