package config

import "testing"

func TestLinksEnabled(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, setting, goos string
		env                 map[string]string
		want                bool
	}{
		{"linux auto", LinksAuto, "linux", map[string]string{"TERM": "xterm"}, true},
		{"linux auto dumb", LinksAuto, "linux", map[string]string{"TERM": "dumb"}, false},
		{"linux auto empty term", LinksAuto, "linux", nil, false},
		{"linux auto ignores WT_SESSION", LinksAuto, "linux", map[string]string{"WT_SESSION": "x"}, false},
		{"darwin auto", LinksAuto, "darwin", map[string]string{"TERM": "xterm"}, true},
		{"windows auto in Windows Terminal", LinksAuto, "windows", map[string]string{"WT_SESSION": "abc"}, true},
		{"windows auto with TERM only", LinksAuto, "windows", map[string]string{"TERM": "xterm"}, false},
		{"windows auto nothing", LinksAuto, "windows", nil, false},
		{"windows on", LinksOn, "windows", nil, true},
		{"linux on dumb", LinksOn, "linux", map[string]string{"TERM": "dumb"}, true},
		{"off", LinksOff, "linux", map[string]string{"TERM": "xterm", "WT_SESSION": "x"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := LinksEnabled(tt.setting, tt.goos, func(k string) string { return tt.env[k] })
			if got != tt.want {
				t.Errorf("LinksEnabled(%q, %q) = %v, want %v", tt.setting, tt.goos, got, tt.want)
			}
		})
	}
}
