package actions

import (
	"testing"

	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/filterform"
)

func TestRunsWords(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		text string
		free []string
		want string
	}{
		{"search row", "  deploy prod ", nil, "deploy prod"},
		{"free words join", "deploy", []string{"prod"}, "deploy prod"},
		{"unclaimed qualifier is kept", "", []string{"status:foo"}, "status:foo"},
		{"nothing", "", nil, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			v := map[string]filterform.Value{ui.SearchField.Key: filterform.TextValue(tt.text)}
			if got := (&Modal{}).filterOf(v, tt.free).Words; got != tt.want {
				t.Fatalf("Words = %q, want %q", got, tt.want)
			}
		})
	}
}
