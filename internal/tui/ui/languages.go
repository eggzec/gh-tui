package ui

import (
	"math"
	"strconv"
	"strings"
)

// nerdLanguages are the Nerd Font glyphs of languages, by the names GitHub
// gives them. They come from Seti and Devicons, which most Nerd Fonts
// carry, rather than the newer Material Design range.
var nerdLanguages = map[string]string{
	"Assembly":         "\ue637",
	"C":                "\ue649",
	"C#":               "\ue648",
	"C++":              "\ue646",
	"CMake":            "\ue794",
	"CSS":              "\ue749",
	"Clojure":          "\ue768",
	"CoffeeScript":     "\ue751",
	"Crystal":          "\ue62f",
	"Dart":             "\ue798",
	"Dockerfile":       "\ue7b0",
	"Elixir":           "\ue62d",
	"Elm":              "\ue62c",
	"Emacs Lisp":       "\ue632",
	"Erlang":           "\ue7b1",
	"Go":               "\ue627",
	"GraphQL":          "\ue662",
	"Groovy":           "\ue775",
	"HCL":              "\ue69a",
	"HTML":             "\ue736",
	"Haskell":          "\ue61f",
	"Java":             "\ue738",
	"JavaScript":       "\ue60c",
	"Julia":            "\ue624",
	"Jupyter Notebook": "\ue80f",
	"Kotlin":           "\ue634",
	"Lua":              "\ue620",
	"MATLAB":           "\ue82a",
	"Makefile":         "\ue673",
	"Markdown":         "\ue73e",
	"Nim":              "\ue677",
	"Nix":              "\uf313",
	"OCaml":            "\ue67a",
	"PHP":              "\ue73d",
	"Perl":             "\ue769",
	"PowerShell":       "\ue683",
	"Python":           "\ue606",
	"R":                "\ue68a",
	"Ruby":             "\ue739",
	"Rust":             "\ue7a8",
	"SCSS":             "\ue603",
	"Sass":             "\ue603",
	"Scala":            "\ue737",
	"Shell":            "\ue795",
	"Solidity":         "\ue8a6",
	"Svelte":           "\ue697",
	"Swift":            "\ue755",
	"TeX":              "\ue69b",
	"TypeScript":       "\ue628",
	"Vim Script":       "\ue62b",
	"Vue":              "\ue6a0",
	"YAML":             "\ue8eb",
	"Zig":              "\ue6a9",
}

// languageColors are the colors of GitHub's linguist for common languages,
// for reads that don't carry the color, such as REST search.
var languageColors = map[string]string{
	"Assembly":         "#6E4C13",
	"Astro":            "#ff5a03",
	"Batchfile":        "#C1F12E",
	"C":                "#555555",
	"C#":               "#178600",
	"C++":              "#f34b7d",
	"CMake":            "#DA3434",
	"CSS":              "#663399",
	"Clojure":          "#db5855",
	"CoffeeScript":     "#244776",
	"Crystal":          "#000100",
	"D":                "#ba595e",
	"Dart":             "#00B4AB",
	"Dockerfile":       "#384d54",
	"Elixir":           "#6e4a7e",
	"Elm":              "#60B5CC",
	"Emacs Lisp":       "#c065db",
	"Erlang":           "#B83998",
	"F#":               "#b845fc",
	"Fortran":          "#4d41b1",
	"Gleam":            "#ffaff3",
	"Go":               "#00ADD8",
	"GraphQL":          "#e10098",
	"Groovy":           "#4298b8",
	"HCL":              "#844FBA",
	"HTML":             "#e34c26",
	"Haskell":          "#5e5086",
	"Java":             "#b07219",
	"JavaScript":       "#f1e05a",
	"Julia":            "#a270ba",
	"Jupyter Notebook": "#DA5B0B",
	"Kotlin":           "#A97BFF",
	"Lua":              "#000080",
	"MATLAB":           "#e16737",
	"Makefile":         "#427819",
	"Markdown":         "#083fa1",
	"Nim":              "#ffc200",
	"Nix":              "#7e7eff",
	"OCaml":            "#ef7a08",
	"Objective-C":      "#438eff",
	"PHP":              "#4F5D95",
	"Perl":             "#0298c3",
	"PowerShell":       "#012456",
	"Python":           "#3572A5",
	"R":                "#198CE7",
	"Ruby":             "#701516",
	"Rust":             "#dea584",
	"SCSS":             "#c6538c",
	"Sass":             "#a53b70",
	"Scala":            "#c22d40",
	"Shell":            "#89e051",
	"Solidity":         "#AA6746",
	"Svelte":           "#ff3e00",
	"Swift":            "#F05138",
	"TeX":              "#3D6117",
	"TypeScript":       "#3178c6",
	"Typst":            "#239dad",
	"Vim Script":       "#199f4b",
	"Vue":              "#41b883",
	"YAML":             "#cb171e",
	"Zig":              "#ec915c",
}

// minContrast is the least contrast a language's color must have with the
// terminal's background to be used, as WCAG measures it. Linguist gives
// Lua navy and JavaScript yellow, which vanish on dark and light
// backgrounds.
const minContrast = 1.8

// LanguageColor returns the hex color of the language named name: color,
// when the read carried it, or the one linguist gives the language. It
// returns "" when neither is known, or when the color would be hard to
// see on a dark or light background.
func LanguageColor(name, color string, dark bool) string {
	if color == "" {
		color = languageColors[name]
	}
	l, ok := luminance(color)
	if !ok {
		return ""
	}
	// The contrast with black or white, whose luminance is 0 or 1.
	c := (1 + 0.05) / (l + 0.05)
	if dark {
		c = (l + 0.05) / 0.05
	}
	if c < minContrast {
		return ""
	}
	return color
}

// luminance returns the relative luminance of hex color c, such as
// "#00ADD8", as WCAG defines it.
func luminance(c string) (float64, bool) {
	c = strings.TrimPrefix(c, "#")
	if len(c) != 6 {
		return 0, false
	}
	v, err := strconv.ParseUint(c, 16, 32)
	if err != nil {
		return 0, false
	}
	lin := func(b uint64) float64 {
		s := float64(b) / 255
		if s <= 0.04045 {
			return s / 12.92
		}
		return math.Pow((s+0.055)/1.055, 2.4)
	}
	return 0.2126*lin(v>>16&0xff) + 0.7152*lin(v>>8&0xff) + 0.0722*lin(v&0xff), true
}
