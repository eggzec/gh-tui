package jobview

import (
	"strconv"
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
)

// stepLines makes n lines of step, with an error at each index of errs.
func stepLines(step, n int, errs ...int) []core.LogLine {
	lines := make([]core.LogLine, n)
	for i := range lines {
		lines[i] = core.LogLine{Text: "line " + strconv.Itoa(i), Step: step}
	}
	for _, e := range errs {
		lines[e].Kind = core.LogError
	}
	return lines
}

func TestFirstError(t *testing.T) {
	t.Parallel()
	failed := []core.Step{
		{Number: 1, Name: "Set up", Conclusion: core.ConclusionSuccess},
		{Number: 2, Name: "Test", Conclusion: core.ConclusionFailure},
	}
	tests := []struct {
		name  string
		lines []core.LogLine
		steps []core.Step
		want  int
		ok    bool
	}{
		{"error at 2", stepLines(1, 10, 2), nil, 2, true},
		{"error at 40", stepLines(1, 60, 40, 50), nil, 40, true},
		{"no error", stepLines(1, 10), nil, 0, false},
		{"failed step wins over an earlier error", append(stepLines(1, 5, 1), stepLines(2, 10, 4)...), failed, 9, true},
		{"failed step without an error opens on its head", append(stepLines(1, 5, 1), stepLines(2, 10)...), failed, 5, true},
		{"no failed step", append(stepLines(1, 5, 3), stepLines(2, 10, 4)...), failed[:1], 3, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, ok := FirstError(tt.lines, tt.steps)
			if got != tt.want || ok != tt.ok {
				t.Errorf("FirstError = %d, %v; want %d, %v", got, ok, tt.want, tt.ok)
			}
		})
	}
}
