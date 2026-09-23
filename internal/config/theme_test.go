package config

import "testing"

func TestPalette(t *testing.T) {
	builtin := builtinThemes[DefaultTheme]
	mine := Theme{
		Light: Palette{Accent: "#111111"},
		Dark:  Palette{Accent: "#eeeeee"},
	}
	tests := []struct {
		name    string
		theme   string
		themes  map[string]Theme
		dark    bool
		want    Palette
		wantErr bool
	}{
		{name: "builtin light", theme: DefaultTheme, want: builtin.Light},
		{name: "builtin dark", theme: DefaultTheme, dark: true, want: builtin.Dark},
		{name: "user theme", theme: "mine", themes: map[string]Theme{"mine": mine}, dark: true, want: mine.Dark},
		{name: "user shadows builtin", theme: DefaultTheme, themes: map[string]Theme{DefaultTheme: mine}, want: mine.Light},
		{name: "unknown", theme: "nope", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{Theme: tt.theme, Themes: tt.themes}
			got, err := cfg.Palette(tt.dark)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Palette(%v) error = %v, wantErr %v", tt.dark, err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("Palette(%v) = %+v, want %+v", tt.dark, got, tt.want)
			}
		})
	}
}

func TestIsHexColor(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{"#7aa2f7", true},
		{"#7AA2F7", true},
		{"#abc", true},
		{"", false},
		{"7aa2f7", false},
		{"#7aa2f", false},
		{"#7aa2g7", false},
		{"#7aa2f7ff", false},
	}
	for _, tt := range tests {
		if got := isHexColor(tt.in); got != tt.want {
			t.Errorf("isHexColor(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}
