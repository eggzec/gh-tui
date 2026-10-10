package config

import (
	"slices"
	"strings"
	"testing"
)

func TestCommandable(t *testing.T) {
	tests := []struct {
		action string
		want   bool
	}{
		{"pulls.merge", true},
		{"pull_modal.merge", true},
		{"global.refresh", true},
		{"global.help", true},
		{"files.expand", true},
		{"history.reset_base", true},
		{"pulls.up", false},
		{"pulls.select", false},
		{"global.back", false},
		{"global.quit", false},
		{"global.command", false},
		{"global.pane_2", false},
		{"pulls.next_match", false},
		{"filter.toggle", false},
		// The keys of what takes every key have no commands.
		{"command_line.run", false},
		{"confirm.method", false},
		{"picker.choose", false},
		{"finder.reveal", false},
		// Not wired yet.
		{"repo.star", false},
		{"nosuch.merge", false},
		{"merge", false},
	}
	for _, tt := range tests {
		if got := Commandable(tt.action); got != tt.want {
			t.Errorf("Commandable(%q) = %v, want %v", tt.action, got, tt.want)
		}
	}
}

// TestNavigationNamesAreActions checks that every name of Navigation is
// an action of some context, so that a name that left the config leaves
// the list too.
func TestNavigationNamesAreActions(t *testing.T) {
	have := map[string]bool{}
	for _, a := range Default().Keys.Actions() {
		_, name, _ := strings.Cut(a, ".")
		have[name] = true
	}
	for _, name := range Navigation() {
		// The search page will have these two.
		if !have[name] && name != "results" && name != "kinds" {
			t.Errorf("Navigation has %q, which no context has an action of", name)
		}
	}
}

func TestActionContexts(t *testing.T) {
	names := func(name string) []string {
		cs := ActionContexts(Default().Keys, name)
		out := make([]string, 0, len(cs))
		for _, c := range cs {
			out = append(out, c.Name)
		}
		return out
	}
	if got, want := names("merge"), []string{"pulls", "pull_modal"}; !slices.Equal(got, want) {
		t.Errorf("merge is in %q, want %q", got, want)
	}
	if got := names("up"); len(got) != 0 {
		t.Errorf("up is a command in %q", got)
	}
	if got := names("run"); len(got) != 0 {
		t.Errorf("run, of the command line, is a command in %q", got)
	}
}

func TestNavigationIsACopy(t *testing.T) {
	n := Navigation()
	n[0] = "changed"
	if Navigation()[0] == "changed" {
		t.Error("Navigation returns the list itself")
	}
}
