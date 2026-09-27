package ui

import (
	"path"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/eggzec/gh-tui/internal/core"
)

// FileIcon is the glyph of a file or directory and its color: a hex color,
// or "" for the palette's muted one.
type FileIcon struct {
	Glyph string
	Color string
}

// fileIcons are the glyphs of files and directories of an icon set. They
// are modelled on nvim-web-devicons and Seti, whose names and extensions
// most file browsers share, but keep to Seti, Devicons, Octicons and Font
// Awesome, which most Nerd Fonts carry, rather than the Material Design
// range.
type fileIcons struct {
	// names, exts and dirs are keyed in lower case, extensions without
	// the dot. stems names files such as LICENSE by the name before the
	// extension, for a text extension or none.
	names, stems, exts, dirs map[string]FileIcon

	file, dir, dirOpen, submodule, symlink FileIcon
}

// Entry returns the icon of e, a directory open or not: that of its name,
// such as go.mod or .github, else that of its extension, else a plain file
// or folder. Sets without file icons return the zero FileIcon.
func (ic Icons) Entry(e core.TreeEntry, open bool) FileIcon {
	f := ic.files
	switch {
	case f == nil:
		return FileIcon{}
	case e.Submodule():
		return f.submodule
	case e.Symlink():
		return f.symlink
	case e.Dir():
		return f.dirIcon(e.Name, open)
	default:
		return f.fileIcon(e.Name)
	}
}

func (f *fileIcons) fileIcon(name string) FileIcon {
	name = strings.ToLower(name)
	if fi, ok := f.names[name]; ok {
		return fi
	}
	ext := path.Ext(name)
	if textExts[ext] {
		if fi, ok := f.stems[strings.TrimSuffix(name, ext)]; ok {
			return fi
		}
	}
	if fi, ok := f.exts[strings.TrimPrefix(ext, ".")]; ok {
		return fi
	}
	return f.file
}

func (f *fileIcons) dirIcon(name string, open bool) FileIcon {
	if fi, ok := f.dirs[strings.ToLower(name)]; ok {
		return fi
	}
	if open {
		return f.dirOpen
	}
	return f.dir
}

// textExts are the extensions of files such as README.md, which are named
// by their stem.
var textExts = map[string]bool{"": true, ".md": true, ".markdown": true, ".txt": true, ".rst": true, ".adoc": true, ".org": true}

// FileIcon returns the style of a file icon: its color, or the palette's
// muted color when it has none or it would be hard to see.
func (t Theme) FileIcon(fi FileIcon) lipgloss.Style {
	if c := LanguageColor("", fi.Color, t.Dark); c != "" {
		return lipgloss.NewStyle().Foreground(lipgloss.Color(c))
	}
	return t.Muted
}

// lang is the icon of the language GitHub names name, so that a file
// looks like its language does on a repository.
func lang(name string) FileIcon {
	return FileIcon{nerdLanguages[name], languageColors[name]}
}

// Colors of folders, and of tools that have no language.
const (
	folderColor = "#519aba"
	gitColor    = "#f54d27"
	npmColor    = "#e8274b"
)

var nerdFiles = &fileIcons{
	file:      FileIcon{"\uf4a5", ""},          // oct-file
	dir:       FileIcon{"\ue5ff", folderColor}, // custom-folder
	dirOpen:   FileIcon{"\ue5fe", folderColor}, // custom-folder_open
	submodule: FileIcon{"\uf414", gitColor},    // oct-file_submodule
	symlink:   FileIcon{"\uf481", ""},          // oct-file_symlink_file
	dirs: map[string]FileIcon{
		".github":      {"\ue5fd", folderColor}, // custom-folder_github
		".config":      {"\ue5fc", folderColor}, // custom-folder_config
		"config":       {"\ue5fc", folderColor},
		"node_modules": {"\ue5fa", npmColor},    // custom-folder_npm
		".vscode":      {"\ue70c", "#007acc"},   // dev-visualstudio
		"docs":         {"\uf02d", folderColor}, // fa-book
		"doc":          {"\uf02d", folderColor},
		"test":         {"\uf499", folderColor}, // oct-beaker
		"tests":        {"\uf499", folderColor},
		"testdata":     {"\uf499", folderColor},
		"__tests__":    {"\uf499", folderColor},
	},
	names: map[string]FileIcon{
		"dockerfile":          lang("Dockerfile"),
		"containerfile":       lang("Dockerfile"),
		".dockerignore":       lang("Dockerfile"),
		"compose.yml":         lang("Dockerfile"),
		"compose.yaml":        lang("Dockerfile"),
		"docker-compose.yml":  lang("Dockerfile"),
		"docker-compose.yaml": lang("Dockerfile"),
		"go.mod":              lang("Go"),
		"go.sum":              lang("Go"),
		"go.work":             lang("Go"),
		"go.work.sum":         lang("Go"),
		"makefile":            lang("Makefile"),
		"gnumakefile":         lang("Makefile"),
		"cmakelists.txt":      lang("CMake"),
		"justfile":            {"\uf0ad", "#6d8086"}, // fa-wrench
		".gitignore":          {"\ue702", gitColor},  // dev-git
		".gitattributes":      {"\ue702", gitColor},
		".gitmodules":         {"\ue702", gitColor},
		".gitkeep":            {"\ue702", gitColor},
		".mailmap":            {"\ue702", gitColor},
		"codeowners":          {"\uf408", ""},       // oct-mark_github
		"package.json":        {"\ue71e", npmColor}, // dev-npm
		"package-lock.json":   {"\ue71e", npmColor},
		".npmrc":              {"\ue71e", npmColor},
		".npmignore":          {"\ue71e", npmColor},
		"yarn.lock":           {"\ue6a7", "#2c8ebb"}, // seti-yarn
		"tsconfig.json":       lang("TypeScript"),
		"cargo.toml":          lang("Rust"),
		"cargo.lock":          lang("Rust"),
		"gemfile":             lang("Ruby"),
		"gemfile.lock":        lang("Ruby"),
		"rakefile":            lang("Ruby"),
		"pyproject.toml":      lang("Python"),
		"requirements.txt":    lang("Python"),
		"pipfile":             lang("Python"),
		"pipfile.lock":        lang("Python"),
		"flake.nix":           lang("Nix"),
		"flake.lock":          lang("Nix"),
		".editorconfig":       {"\ue652", "#fff2f2"}, // seti-editorconfig
		".envrc":              {"\uf462", "#faf743"}, // oct-sliders
		".eslintrc":           {"\ue655", "#4b32c3"}, // seti-eslint
	},
	stems: map[string]FileIcon{
		"license":         {"\ue60a", "#d0bf41"}, // seti-license
		"licence":         {"\ue60a", "#d0bf41"},
		"copying":         {"\ue60a", "#d0bf41"},
		"readme":          {"\uf405", "#519aba"}, // oct-book
		"changelog":       {"\uf464", "#89e051"}, // oct-history
		"changes":         {"\uf464", "#89e051"},
		"security":        {"\uf49c", "#bec4c9"}, // oct-shield
		"code_of_conduct": {"\uf4ae", "#e41662"}, // oct-code_of_conduct
	},
	exts: map[string]FileIcon{
		"go":         lang("Go"),
		"ts":         lang("TypeScript"),
		"mts":        lang("TypeScript"),
		"cts":        lang("TypeScript"),
		"js":         lang("JavaScript"),
		"mjs":        lang("JavaScript"),
		"cjs":        lang("JavaScript"),
		"tsx":        {"\ue7ba", "#1354bf"}, // dev-react
		"jsx":        {"\ue7ba", "#20c2e3"},
		"py":         lang("Python"),
		"pyi":        lang("Python"),
		"rs":         lang("Rust"),
		"md":         lang("Markdown"),
		"markdown":   lang("Markdown"),
		"mdx":        lang("Markdown"),
		"yaml":       lang("YAML"),
		"yml":        lang("YAML"),
		"json":       {"\ue60b", "#cbcb41"}, // seti-json
		"jsonc":      {"\ue60b", "#cbcb41"},
		"json5":      {"\ue60b", "#cbcb41"},
		"toml":       {"\ue6b2", "#9c4221"}, // custom-toml
		"sh":         lang("Shell"),
		"bash":       lang("Shell"),
		"zsh":        lang("Shell"),
		"fish":       lang("Shell"),
		"bat":        {"\ue795", languageColors["Batchfile"]}, // dev-terminal
		"cmd":        {"\ue795", languageColors["Batchfile"]},
		"ps1":        lang("PowerShell"),
		"psm1":       lang("PowerShell"),
		"html":       lang("HTML"),
		"htm":        lang("HTML"),
		"css":        lang("CSS"),
		"scss":       lang("SCSS"),
		"sass":       lang("Sass"),
		"less":       {"\ue614", "#563d7c"}, // seti-css
		"c":          lang("C"),
		"h":          lang("C"),
		"cpp":        lang("C++"),
		"cc":         lang("C++"),
		"cxx":        lang("C++"),
		"hpp":        lang("C++"),
		"hh":         lang("C++"),
		"hxx":        lang("C++"),
		"cs":         lang("C#"),
		"fs":         {"\ue7a7", languageColors["F#"]}, // dev-fsharp
		"fsx":        {"\ue7a7", languageColors["F#"]},
		"java":       lang("Java"),
		"kt":         lang("Kotlin"),
		"kts":        lang("Kotlin"),
		"scala":      lang("Scala"),
		"groovy":     lang("Groovy"),
		"gradle":     {"\ue660", "#005f87"}, // seti-gradle
		"clj":        lang("Clojure"),
		"cljs":       lang("Clojure"),
		"rb":         lang("Ruby"),
		"php":        lang("PHP"),
		"pl":         lang("Perl"),
		"pm":         lang("Perl"),
		"lua":        lang("Lua"),
		"r":          lang("R"),
		"jl":         lang("Julia"),
		"hs":         lang("Haskell"),
		"ml":         lang("OCaml"),
		"mli":        lang("OCaml"),
		"elm":        lang("Elm"),
		"ex":         lang("Elixir"),
		"exs":        lang("Elixir"),
		"erl":        lang("Erlang"),
		"dart":       lang("Dart"),
		"swift":      lang("Swift"),
		"zig":        lang("Zig"),
		"nim":        lang("Nim"),
		"cr":         lang("Crystal"),
		"coffee":     lang("CoffeeScript"),
		"sol":        lang("Solidity"),
		"vue":        lang("Vue"),
		"svelte":     lang("Svelte"),
		"astro":      {"\ue6b3", languageColors["Astro"]}, // custom-astro
		"graphql":    lang("GraphQL"),
		"gql":        lang("GraphQL"),
		"tf":         lang("HCL"),
		"hcl":        lang("HCL"),
		"nix":        lang("Nix"),
		"vim":        lang("Vim Script"),
		"el":         lang("Emacs Lisp"),
		"asm":        lang("Assembly"),
		"s":          lang("Assembly"),
		"cmake":      lang("CMake"),
		"mk":         lang("Makefile"),
		"ipynb":      lang("Jupyter Notebook"),
		"tex":        lang("TeX"),
		"typ":        {"\uf37f", languageColors["Typst"]}, // linux-typst
		"dockerfile": lang("Dockerfile"),
		"proto":      {"\uf121", "#4285f4"}, // fa-code
		"sql":        {"\ue706", "#dad8d8"}, // dev-database
		"db":         {"\ue7c4", "#0f80cc"}, // dev-sqlite
		"sqlite":     {"\ue7c4", "#0f80cc"},
		"sqlite3":    {"\ue7c4", "#0f80cc"},
		"xml":        {"\ue619", "#e37933"}, // seti-xml
		"csv":        {"\ue64a", "#89e051"}, // seti-csv
		"tsv":        {"\ue64a", "#89e051"},
		"ini":        {"\ue615", "#6d8086"}, // seti-config
		"cfg":        {"\ue615", "#6d8086"},
		"conf":       {"\ue615", "#6d8086"},
		"properties": {"\ue615", "#6d8086"},
		"env":        {"\uf462", "#faf743"}, // oct-sliders
		"lock":       {"\ue672", "#bbbbbb"}, // seti-lock
		"txt":        {"\uf15c", "#89e051"}, // fa-file_text
		"log":        {"\uf4ed", "#dddddd"}, // oct-log
		"diff":       {"\ue728", "#41535b"}, // dev-git_compare
		"patch":      {"\ue728", "#41535b"},
		"pdf":        {"\ue67d", "#b30b00"}, // seti-pdf
		"doc":        {"\ue6a5", "#185abd"}, // seti-word
		"docx":       {"\ue6a5", "#185abd"},
		"xls":        {"\ue6a6", "#207245"}, // seti-xls
		"xlsx":       {"\ue6a6", "#207245"},
		"ppt":        {"\uf1c4", "#cb4a32"}, // fa-file_powerpoint
		"pptx":       {"\uf1c4", "#cb4a32"},
		"png":        {"\ue60d", "#a074c4"}, // seti-image
		"jpg":        {"\ue60d", "#a074c4"},
		"jpeg":       {"\ue60d", "#a074c4"},
		"gif":        {"\ue60d", "#a074c4"},
		"webp":       {"\ue60d", "#a074c4"},
		"avif":       {"\ue60d", "#a074c4"},
		"bmp":        {"\ue60d", "#a074c4"},
		"ico":        {"\ue60d", "#cbcb41"},
		"svg":        {"\ue698", "#ffb13b"}, // seti-svg
		"mp3":        {"\uf001", "#00afff"}, // fa-music
		"wav":        {"\uf001", "#00afff"},
		"flac":       {"\uf001", "#00afff"},
		"ogg":        {"\uf001", "#00afff"},
		"mp4":        {"\ue69f", "#fd971f"}, // seti-video
		"mov":        {"\ue69f", "#fd971f"},
		"mkv":        {"\ue69f", "#fd971f"},
		"webm":       {"\ue69f", "#fd971f"},
		"avi":        {"\ue69f", "#fd971f"},
		"ttf":        {"\ue659", "#ececec"}, // seti-font
		"otf":        {"\ue659", "#ececec"},
		"woff":       {"\ue659", "#ececec"},
		"woff2":      {"\ue659", "#ececec"},
		"zip":        {"\uf410", "#eca517"}, // oct-file_zip
		"tar":        {"\uf410", "#eca517"},
		"gz":         {"\uf410", "#eca517"},
		"tgz":        {"\uf410", "#eca517"},
		"bz2":        {"\uf410", "#eca517"},
		"xz":         {"\uf410", "#eca517"},
		"zst":        {"\uf410", "#eca517"},
		"7z":         {"\uf410", "#eca517"},
		"rar":        {"\uf410", "#eca517"},
		"jar":        {"\uf410", "#eca517"},
		"exe":        {"\uf471", "#9f0500"}, // oct-file_binary
		"bin":        {"\uf471", "#9f0500"},
		"o":          {"\uf471", "#9f0500"},
		"so":         {"\ueb9c", "#dcddd6"}, // cod-library
		"a":          {"\ueb9c", "#dcddd6"},
		"dll":        {"\ueb9c", "#dcddd6"},
		"dylib":      {"\ueb9c", "#dcddd6"},
		"wasm":       {"\ue6a1", "#5c4cdb"}, // seti-wasm
		"pem":        {"\uf43d", "#e3c58e"}, // oct-key
		"key":        {"\uf43d", "#e3c58e"},
		"crt":        {"\uf43d", "#e3c58e"},
		"pub":        {"\uf43d", "#e3c58e"},
		"asc":        {"\uf43d", "#e3c58e"},
		"gpg":        {"\uf43d", "#e3c58e"},
		"sig":        {"\uf43d", "#e3c58e"},
	},
}
