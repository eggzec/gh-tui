package markdown

import "testing"

func TestTidy(t *testing.T) {
	tests := []struct {
		name, in, want string
	}{
		{name: "plain padding", in: "text   ", want: "text"},
		{name: "styled padding", in: "\x1b[38;5;252mtext\x1b[m\x1b[38;5;252m \x1b[m\x1b[38;5;252m \x1b[m", want: "\x1b[38;5;252mtext\x1b[m"},
		{
			name: "runs of one style join",
			in:   "\x1b[38;5;252mSteps to\x1b[m\x1b[38;5;252m reproduce\x1b[m",
			want: "\x1b[38;5;252mSteps to reproduce\x1b[m",
		},
		{
			name: "a new style resets",
			in:   "\x1b[38;5;252ma\x1b[m\x1b[38;5;39mb\x1b[m",
			want: "\x1b[38;5;252ma\x1b[m\x1b[38;5;39mb\x1b[m",
		},
		{
			name: "a style on top is added",
			in:   "\x1b[38;5;252ma\x1b[1mb\x1b[m",
			want: "\x1b[38;5;252ma\x1b[1mb\x1b[m",
		},
		{
			name: "a background shows its spaces",
			in:   "\x1b[1;48;5;63m Title \x1b[m\x1b[38;5;252m   \x1b[m",
			want: "\x1b[1;48;5;63m Title \x1b[m",
		},
		{name: "a 256 color that looks like a background", in: "\x1b[38;5;41ma \x1b[m", want: "\x1b[38;5;41ma\x1b[m"},
		{name: "reverse video shows spaces", in: "\x1b[7m  \x1b[m  ", want: "\x1b[7m  \x1b[m"},
		{
			name: "hyperlinks go",
			in:   "\x1b]8;id=1;https://x.test\x07docs\x1b]8;;\x07 \x1b]8;;https://y.test\x1b\\y\x1b]8;;\x1b\\",
			want: "docs y",
		},
		{name: "other sequences go", in: "a\x1b[2Jb\x1b[H", want: "ab"},
		{
			name: "spaces keep the style they follow",
			in:   "\x1b[38;5;35mdocs\x1b[m\x1b[38;5;252m \x1b[m\x1b[38;5;30;4murl\x1b[m",
			want: "\x1b[38;5;35mdocs \x1b[m\x1b[38;5;30;4murl\x1b[m",
		},
		{
			name: "leading spaces need no style",
			in:   "\x1b[38;5;252m  \x1b[m\x1b[38;5;39mfunc\x1b[m",
			want: "  \x1b[38;5;39mfunc\x1b[m",
		},
		{
			name: "an underline shows on spaces",
			in:   "a\x1b[4m b\x1b[m",
			want: "a\x1b[4m b\x1b[m",
		},
		{
			name: "spaces after an underline are drawn without it",
			in:   "\x1b[4ma\x1b[m\x1b[38;5;1m \x1b[mb",
			want: "\x1b[4ma\x1b[m b",
		},
		{name: "only padding", in: "\x1b[38;5;252m   \x1b[m", want: ""},
		{name: "reset with a style", in: "\x1b[1ma\x1b[0;38;5;1mb\x1b[0m", want: "\x1b[1ma\x1b[m\x1b[0;38;5;1mb\x1b[m"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tidy(tt.in); got != tt.want {
				t.Errorf("tidy(%q)\n got %q\nwant %q", tt.in, got, tt.want)
			}
		})
	}
}
