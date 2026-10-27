package issues

import (
	"testing"

	"github.com/eggzec/gh-tui/pkg/bubbles/prompt"
)

func TestHasDraft(t *testing.T) {
	tests := []struct {
		name  string
		what  composing
		value string
		want  bool
	}{
		{"no prompt", composeNone, "", false},
		{"an empty comment", composeComment, "  ", false},
		{"a typed comment", composeComment, "On it.", true},
		{"typed labels are no draft", composeLabels, "bug", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, m := opened(t, newFakeService(sampleIssues(12)), 30)
			m.prompt = prompt.New(prompt.WithValue(tt.value))
			m.composing = tt.what
			if got := m.HasDraft(); got != tt.want {
				t.Errorf("HasDraft() = %v, want %v", got, tt.want)
			}
		})
	}
}
