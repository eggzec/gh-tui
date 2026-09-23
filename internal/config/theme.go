package config

import (
	"errors"
	"fmt"
	"strconv"
)

// DefaultTheme is the name of the built-in theme used when none is set.
const DefaultTheme = "default"

// Theme is a named palette with a variant for light and dark terminals.
type Theme struct {
	Light Palette `yaml:"light"`
	Dark  Palette `yaml:"dark"`
}

// Palette holds the semantic colors of a theme as hex strings such as
// "#7aa2f7".
type Palette struct {
	// Accent marks the focused element, selections and links.
	Accent string `yaml:"accent"`
	// Foreground is the main text color.
	Foreground string `yaml:"foreground"`
	// Muted is for secondary text such as metadata.
	Muted string `yaml:"muted"`
	// Subtle is for the faintest text, such as hints and placeholders.
	Subtle string `yaml:"subtle"`
	// Border is for borders and separators.
	Border  string `yaml:"border"`
	Success string `yaml:"success"`
	Warning string `yaml:"warning"`
	Error   string `yaml:"error"`
}

var builtinThemes = map[string]Theme{
	DefaultTheme: {
		Light: Palette{
			Accent:     "#3b63c4",
			Foreground: "#1f2330",
			Muted:      "#545b6e",
			Subtle:     "#8a90a0",
			Border:     "#d3d7e0",
			Success:    "#2f7d4f",
			Warning:    "#9a6700",
			Error:      "#c0392b",
		},
		Dark: Palette{
			Accent:     "#7aa2f7",
			Foreground: "#d8dce6",
			Muted:      "#a0a7b8",
			Subtle:     "#6b7285",
			Border:     "#3a3f4d",
			Success:    "#7fc99a",
			Warning:    "#e5c07b",
			Error:      "#ef7d7d",
		},
	},
}

// Palette returns the active theme's palette for a dark or light terminal.
func (c Config) Palette(dark bool) (Palette, error) {
	t, ok := c.theme(c.Theme)
	if !ok {
		return Palette{}, fmt.Errorf("unknown theme %q", c.Theme)
	}
	if dark {
		return t.Dark, nil
	}
	return t.Light, nil
}

func (c Config) theme(name string) (Theme, bool) {
	if t, ok := c.Themes[name]; ok {
		return t, true
	}
	t, ok := builtinThemes[name]
	return t, ok
}

func (t Theme) validate(path string) error {
	return errors.Join(
		t.Light.validate(path+".light"),
		t.Dark.validate(path+".dark"),
	)
}

func (p Palette) validate(path string) error {
	colors := []struct{ name, value string }{
		{"accent", p.Accent},
		{"foreground", p.Foreground},
		{"muted", p.Muted},
		{"subtle", p.Subtle},
		{"border", p.Border},
		{"success", p.Success},
		{"warning", p.Warning},
		{"error", p.Error},
	}
	var errs []error
	for _, c := range colors {
		if !isHexColor(c.value) {
			// An unquoted "#…" is a YAML comment, which leaves the value empty.
			errs = append(errs, fmt.Errorf(`%s.%s: want a quoted hex color like "#7aa2f7", got %q`, path, c.name, c.value))
		}
	}
	return errors.Join(errs...)
}

// isHexColor reports whether s is "#rgb" or "#rrggbb".
func isHexColor(s string) bool {
	if len(s) != 4 && len(s) != 7 || s[0] != '#' {
		return false
	}
	_, err := strconv.ParseUint(s[1:], 16, 32)
	return err == nil
}
