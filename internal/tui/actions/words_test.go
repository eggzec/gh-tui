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
		{"free words join", "deploy", []string{"prod"}, `deploy "prod"`},
		{"unclaimed qualifier is kept", "", []string{"status:foo"}, `"status:foo"`},
		{"nothing", "", nil, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			v := map[string]filterform.Value{ui.SearchField.Key: filterform.TextValue(tt.text)}
			got := (&Modal{}).filterOf(v, tt.free).Words
			if got != tt.want {
				t.Fatalf("Words = %q, want %q", got, tt.want)
			}
			if len(tt.free) == 1 && tt.free[0] == "status:foo" {
				if w := ui.Words(got); len(w) != 1 || w[0] != "status:foo" {
					t.Fatalf("ui.Words = %q, want [status:foo]", w)
				}
			}
		})
	}
}
