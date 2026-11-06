package issues

import "testing"

func TestAssigneeQuestion(t *testing.T) {
	t.Parallel()
	tests := []struct {
		added, removed []string
		want           string
	}{
		{[]string{"a"}, []string{"b"}, "Assign @a and unassign @b on #12?"},
		{[]string{"a", "c"}, nil, "Assign @a, @c on #12?"},
		{nil, []string{"b"}, "Unassign @b on #12?"},
	}
	for _, tt := range tests {
		if got := assigneeQuestion("#12", tt.added, tt.removed); got != tt.want {
			t.Errorf("assigneeQuestion(%v, %v) = %q, want %q", tt.added, tt.removed, got, tt.want)
		}
	}
}
