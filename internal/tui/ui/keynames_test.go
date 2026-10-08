package ui

import (
	"slices"
	"testing"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/pkg/bubbles/keymap"
)

// TestContextOfSaysUnboundFromMissing pins what a context's lookup returns:
// nil for an action the config doesn't have, which is not recorded, and
// an empty slice for one it unbinds.
func TestContextOfSaysUnboundFromMissing(t *testing.T) {
	keys := config.Default().Keys
	keys.Set("files.up", []string{})
	c := In(keys, "files")
	if got := c.Of("up"); got == nil || len(got) != 0 {
		t.Errorf("unbound action gives %#v, want an empty slice", got)
	}
	if got := c.Of("no_such_action"); got != nil {
		t.Errorf("missing action gives %#v, want nil", got)
	}
	if got := c.Of("global.select"); len(got) == 0 {
		t.Errorf("another context's action gives %#v, want its keys", got)
	}
}

// TestUnboundBindingsKeepTheirOwnPath checks that bindings without keys
// are told apart: each lists its own path and no other, though they say
// the same.
func TestUnboundBindingsKeepTheirOwnPath(t *testing.T) {
	keys := config.Default().Keys
	keys.Set("files.up", []string{})
	keys.Set("issues.up", []string{})
	files, issues := In(keys, "files").Binding("up", "up"), In(keys, "issues").Binding("up", "up")
	if files.Enabled() || issues.Enabled() || len(files.Keys()) != 0 {
		t.Fatalf("unbound bindings are enabled: %v %v", files.Enabled(), issues.Enabled())
	}
	if got := keymap.Actions(files); !slices.Equal(got, []string{"files.up"}) {
		t.Errorf("files: %v", got)
	}
	if got := keymap.Actions(issues); !slices.Equal(got, []string{"issues.up"}) {
		t.Errorf("issues: %v", got)
	}
}
