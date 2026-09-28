package obs

import "testing"

func TestLogKey(t *testing.T) {
	tests := []struct{ key, want string }{
		{"pulls:o/r?filter=author%3Ame&state=open", "pulls:o/r"},
		{"search?kind=issues&q=secret", "search"},
		{"issue:o/r#12", "issue:o/r#12"},
		{"", ""},
	}
	for _, tt := range tests {
		if got := LogKey(tt.key); got != tt.want {
			t.Errorf("LogKey(%q) = %q, want %q", tt.key, got, tt.want)
		}
	}
}
