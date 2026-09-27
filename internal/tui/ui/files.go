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
// range. Some Devicons, such as those of Helm and Vercel, came with Nerd
// Fonts 3.3, which the icons of languages such as Jupyter already need.
// The files of tools follow the mappings of the VS Code icon themes
// Material Icon Theme, vscode-icons and Seti (MIT).
type fileIcons struct {
	// names, exts and dirs are keyed in lower case, extensions without
	// the dot. stems names files such as LICENSE by the name before the
	// extension, for a text extension or none.
	names, stems, exts, dirs map[string]FileIcon
	// globs name files by patterns such as tsconfig*.json, in order, for
	// the tools that read several names. globsByExt holds them by the
	// extension a name needs to match them, so that a name tries only a
	// few.
	globs      []globIcon
	globsByExt map[string][]globIcon
	// paths name the files that tools read at a fixed place, by the globs
	// of their names, keyed by the lower-cased directory that holds them
	// from the root of the repository, such as .github/workflows. GitHub
	// reads workflows only there, not in a directory below it.
	paths map[string][]globIcon
	// dirPaths are keyed by the lower-cased path of the directory.
	dirPaths map[string]FileIcon

	file, dir, dirOpen, submodule, symlink FileIcon
}

// Entry returns the icon of e, a directory open or not: that of its path,
// such as .github/workflows, else that of its name, such as go.mod or
// .github, else that of its extension, else a plain file or folder. Sets
// without file icons return the zero FileIcon.
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
		return f.dirIcon(e.Path, e.Name, open)
	default:
		return f.fileIcon(e.Path, e.Name)
	}
}

// globIcon is the icon of the files whose lower-cased name matches glob,
// a path.Match pattern.
type globIcon struct {
	glob string
	icon FileIcon
}

// match returns the icon of the first of gs that name matches.
func match(gs []globIcon, name string) (FileIcon, bool) {
	for _, g := range gs {
		if ok, _ := path.Match(g.glob, name); ok {
			return g.icon, true
		}
	}
	return FileIcon{}, false
}

// indexGlobs indexes the globs of f by the extension a name needs to
// match each, such as .yml for openapi*.yml. A glob that leaves the
// extension open, such as .eslintrc*, is under every extension, and alone
// under "". Each list keeps the order of the globs.
func indexGlobs(f *fileIcons) *fileIcons {
	f.globsByExt = map[string][]globIcon{"": nil}
	for _, g := range f.globs {
		f.globsByExt[globExt(g.glob)] = nil
	}
	for ext := range f.globsByExt {
		for _, g := range f.globs {
			if e := globExt(g.glob); e == "" || e == ext {
				f.globsByExt[ext] = append(f.globsByExt[ext], g)
			}
		}
	}
	return f
}

// globExt returns the extension of every name glob matches, or "" when
// glob leaves it open.
func globExt(glob string) string {
	ext := path.Ext(glob)
	if strings.ContainsAny(ext, `*?[]\`) {
		return ""
	}
	return ext
}

// extGlobs returns the globs that a name with extension ext may match.
func (f *fileIcons) extGlobs(ext string) []globIcon {
	if gs, ok := f.globsByExt[ext]; ok {
		return gs
	}
	return f.globsByExt[""]
}

func (f *fileIcons) fileIcon(p, name string) FileIcon {
	name = strings.ToLower(name)
	if fi, ok := match(f.paths[strings.ToLower(path.Dir(p))], name); ok {
		return fi
	}
	if fi, ok := f.names[name]; ok {
		return fi
	}
	ext := path.Ext(name)
	if fi, ok := match(f.extGlobs(ext), name); ok {
		return fi
	}
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

func (f *fileIcons) dirIcon(p, name string, open bool) FileIcon {
	if fi, ok := f.dirPaths[path.Clean(strings.ToLower(p))]; ok {
		return fi
	}
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
	folderColor  = "#519aba"
	gitColor     = "#f54d27"
	npmColor     = "#e8274b"
	actionsColor = "#2088ff"
	githubColor  = "#3fb950"
)

// Icons of the files and directories GitHub and other tools read at a
// fixed place.
var (
	workflowIcon  = FileIcon{"\ue7e9", actionsColor} // dev-githubactions
	giteaIcon     = FileIcon{"\uf339", "#609926"}    // linux-gitea
	issueIcon     = FileIcon{"\uf41b", githubColor}  // oct-issue_opened
	pullIcon      = FileIcon{"\uf407", githubColor}  // oct-git_pull_request
	templatesIcon = FileIcon{"\ue5fd", folderColor}  // custom-folder_github
	codescanIcon  = FileIcon{"\uf4b1", githubColor}  // oct-codescan
	azureIcon     = FileIcon{"\ue756", "#0078d7"}    // dev-azuredevops
)

var nerdFiles = indexGlobs(&fileIcons{
	file:      FileIcon{"\uf4a5", ""},          // oct-file
	dir:       FileIcon{"\ue5ff", folderColor}, // custom-folder
	dirOpen:   FileIcon{"\ue5fe", folderColor}, // custom-folder_open
	submodule: FileIcon{"\uf414", gitColor},    // oct-file_submodule
	symlink:   FileIcon{"\uf481", ""},          // oct-file_symlink_file
	dirs: map[string]FileIcon{
		".github":       {"\ue5fd", folderColor}, // custom-folder_github
		".config":       {"\ue5fc", folderColor}, // custom-folder_config
		"config":        {"\ue5fc", folderColor},
		"node_modules":  {"\ue5fa", npmColor},    // custom-folder_npm
		".vscode":       {"\ue70c", "#007acc"},   // dev-visualstudio
		"docs":          {"\uf02d", folderColor}, // fa-book
		"doc":           {"\uf02d", folderColor},
		"test":          {"\uf499", folderColor}, // oct-beaker
		"tests":         {"\uf499", folderColor},
		"testdata":      {"\uf499", folderColor},
		"__tests__":     {"\uf499", folderColor},
		".circleci":     {"\ue78c", folderColor}, // dev-circleci
		".devcontainer": {"\uf4b7", "#2496ed"},   // oct-container
	},
	names: map[string]FileIcon{
		"dockerfile":        lang("Dockerfile"),
		"containerfile":     lang("Dockerfile"),
		".dockerignore":     lang("Dockerfile"),
		"compose.yml":       lang("Dockerfile"),
		"compose.yaml":      lang("Dockerfile"),
		"go.mod":            lang("Go"),
		"go.sum":            lang("Go"),
		"go.work":           lang("Go"),
		"go.work.sum":       lang("Go"),
		"makefile":          lang("Makefile"),
		"gnumakefile":       lang("Makefile"),
		"cmakelists.txt":    lang("CMake"),
		"justfile":          {"\uf0ad", "#6d8086"}, // fa-wrench
		".gitignore":        {"\ue702", gitColor},  // dev-git
		".gitattributes":    {"\ue702", gitColor},
		".gitmodules":       {"\ue702", gitColor},
		".gitkeep":          {"\ue702", gitColor},
		".mailmap":          {"\ue702", gitColor},
		"package.json":      {"\ue71e", npmColor}, // dev-npm
		"package-lock.json": {"\ue71e", npmColor},
		".npmrc":            {"\ue71e", npmColor},
		".npmignore":        {"\ue71e", npmColor},
		"yarn.lock":         {"\ue6a7", "#2c8ebb"}, // seti-yarn
		"cargo.toml":        lang("Rust"),
		"cargo.lock":        lang("Rust"),
		"gemfile":           lang("Ruby"),
		"gemfile.lock":      lang("Ruby"),
		"rakefile":          lang("Ruby"),
		"pyproject.toml":    lang("Python"),
		"requirements.txt":  lang("Python"),
		"pipfile":           lang("Python"),
		"pipfile.lock":      lang("Python"),
		"flake.nix":         lang("Nix"),
		"flake.lock":        lang("Nix"),
		".editorconfig":     {"\ue652", "#fff2f2"}, // seti-editorconfig
		".envrc":            {"\uf462", "#faf743"}, // oct-sliders
		// GitHub, and the tools that run on a repository.
		"codeowners":               {"\uf4fd", "#afb42b"},    // oct-people
		"action.yml":               {"\ueaff", actionsColor}, // cod-github_action
		"action.yaml":              {"\ueaff", actionsColor},
		"dependabot.yml":           {"\uf4be", "#0366d6"}, // oct-dependabot
		"dependabot.yaml":          {"\uf4be", "#0366d6"},
		"pull_request_template.md": pullIcon,
		"issue_template.md":        issueIcon,
		"renovate.json":            {"\uf4f8", "#1a7fa0"}, // oct-package_dependencies
		"renovate.json5":           {"\uf4f8", "#1a7fa0"},
		".renovaterc":              {"\uf4f8", "#1a7fa0"},
		".renovaterc.json":         {"\uf4f8", "#1a7fa0"},
		".renovaterc.json5":        {"\uf4f8", "#1a7fa0"},
		".pre-commit-config.yaml":  {"\uf417", "#f8b424"}, // oct-git_commit
		".pre-commit-config.yml":   {"\uf417", "#f8b424"},
		".pre-commit-hooks.yaml":   {"\uf417", "#f8b424"},
		"devcontainer.json":        {"\uf4b7", "#2496ed"}, // oct-container
		".devcontainer.json":       {"\uf4b7", "#2496ed"},
		".golangci.yml":            {"\uf4b1", "#00add8"}, // oct-codescan
		".golangci.yaml":           {"\uf4b1", "#00add8"},
		".golangci.toml":           {"\uf4b1", "#00add8"},
		".golangci.json":           {"\uf4b1", "#00add8"},
		".goreleaser.yml":          {"\uf427", "#00add8"}, // oct-rocket
		".goreleaser.yaml":         {"\uf427", "#00add8"},
		"goreleaser.yml":           {"\uf427", "#00add8"},
		"goreleaser.yaml":          {"\uf427", "#00add8"},
		"mkdocs.yml":               {"\uf02d", "#526cfe"}, // fa-book
		"mkdocs.yaml":              {"\uf02d", "#526cfe"},
		".readthedocs.yml":         {"\ue889", "#8ca1af"}, // dev-readthedocs
		".readthedocs.yaml":        {"\ue889", "#8ca1af"},
		// Continuous integration.
		".gitlab-ci.yml":           {"\ue65c", "#e24329"}, // seti-gitlab
		".travis.yml":              {"\ue77e", "#cb3349"}, // dev-travis
		"bitbucket-pipelines.yml":  {"\ue703", "#2684ff"}, // dev-bitbucket
		"jenkinsfile":              {"\ue767", "#d24939"}, // dev-jenkins
		".gitpod.yml":              {"\ue7ec", "#ffae33"}, // dev-gitpod
		"codecov.yml":              {"\ue797", "#f01f7a"}, // dev-codecov
		".codecov.yml":             {"\ue797", "#f01f7a"},
		"sonar-project.properties": {"\ue8a8", "#4e9bcd"}, // dev-sonarqube
		// Deployment and infrastructure.
		"chart.yaml":          {"\ue7fb", "#0f1689"}, // dev-helm
		"chart.lock":          {"\ue7fb", "#0f1689"},
		".helmignore":         {"\ue7fb", "#0f1689"},
		"helmfile.yaml":       {"\ue7fb", "#0f1689"},
		"kustomization.yaml":  {"\ue81d", "#326ce5"}, // dev-kubernetes
		"kustomization.yml":   {"\ue81d", "#326ce5"},
		"ansible.cfg":         {"\ue723", "#ee0000"}, // dev-ansible
		".ansible-lint":       {"\ue723", "#ee0000"},
		".terraform.lock.hcl": {"\ue8bd", "#844fba"}, // dev-terraform
		"procfile":            {"\ue77b", "#6567a5"}, // dev-heroku
		"vagrantfile":         {"\ue8d0", "#1868f2"}, // dev-vagrant
		"netlify.toml":        {"\ue83c", "#05bdba"}, // dev-netlify
		"vercel.json":         {"\ue8d3", ""},        // dev-vercel
		"wrangler.toml":       {"\ue792", "#f38020"}, // dev-cloudflare
		"wrangler.json":       {"\ue792", "#f38020"},
		"wrangler.jsonc":      {"\ue792", "#f38020"},
		"firebase.json":       {"\ue787", "#ffca28"}, // dev-firebase
		".firebaserc":         {"\ue787", "#ffca28"},
		// Package managers and JavaScript tools.
		"pnpm-lock.yaml":      {"\ue865", "#f9ad00"}, // dev-pnpm
		"pnpm-workspace.yaml": {"\ue865", "#f9ad00"},
		"bun.lock":            {"\ue76f", "#fbf0df"}, // dev-bun
		"bun.lockb":           {"\ue76f", "#fbf0df"},
		"bunfig.toml":         {"\ue76f", "#fbf0df"},
		"deno.json":           {"\ue7c0", "#70ffaf"}, // dev-denojs
		"deno.jsonc":          {"\ue7c0", "#70ffaf"},
		"deno.lock":           {"\ue7c0", "#70ffaf"},
		"biome.json":          {"\ue8fb", "#60a5fa"}, // dev-biome
		"biome.jsonc":         {"\ue8fb", "#60a5fa"},
		"turbo.json":          {"\ue94d", "#ff1e56"}, // dev-turbo
		"poetry.lock":         {"\ue867", "#60a5fa"}, // dev-poetry
	},
	globs: []globIcon{
		{"compose.*.yml", lang("Dockerfile")},
		{"compose.*.yaml", lang("Dockerfile")},
		{"*docker*compose*.yml", lang("Dockerfile")},
		{"*docker*compose*.yaml", lang("Dockerfile")},
		{"tsconfig*.json", FileIcon{"\ue69d", "#3178c6"}}, // seti-tsconfig
		{"jsconfig*.json", FileIcon{"\ue69d", "#3178c6"}},
		{".eslintrc*", FileIcon{"\ue655", "#4b32c3"}}, // seti-eslint
		{"eslint.config.*", FileIcon{"\ue655", "#4b32c3"}},
		{".prettierrc*", FileIcon{"\ue6b4", "#56b3b4"}}, // custom-prettier
		{"prettier.config.*", FileIcon{"\ue6b4", "#56b3b4"}},
		{"openapi*.yml", FileIcon{"\ue852", "#6ba539"}}, // dev-openapi
		{"openapi*.yaml", FileIcon{"\ue852", "#6ba539"}},
		{"openapi*.json", FileIcon{"\ue852", "#6ba539"}},
		{"swagger*.yml", FileIcon{"\ue8b8", "#85ea2d"}}, // dev-swagger
		{"swagger*.yaml", FileIcon{"\ue8b8", "#85ea2d"}},
		{"swagger*.json", FileIcon{"\ue8b8", "#85ea2d"}},
		{"azure-pipelines*.yml", azureIcon},
		{"azure-pipelines*.yaml", azureIcon},
	},
	paths: map[string][]globIcon{
		".github/workflows":  {{"*.yml", workflowIcon}, {"*.yaml", workflowIcon}},
		".gitea/workflows":   {{"*.yml", giteaIcon}, {"*.yaml", giteaIcon}},
		".forgejo/workflows": {{"*.yml", giteaIcon}, {"*.yaml", giteaIcon}},
		".github": {
			{"funding.yml", FileIcon{"\uf4e1", "#db61a2"}}, // oct-heart_fill
			{"funding.yaml", FileIcon{"\uf4e1", "#db61a2"}},
			{"release.yml", FileIcon{"\uf412", githubColor}}, // oct-tag
			{"release.yaml", FileIcon{"\uf412", githubColor}},
			{"labeler.yml", FileIcon{"\uf412", "#ffb300"}},
			{"labeler.yaml", FileIcon{"\uf412", "#ffb300"}},
			{"labels.yml", FileIcon{"\uf412", "#ffb300"}},
			{"labels.yaml", FileIcon{"\uf412", "#ffb300"}},
			{"copilot-instructions.md", FileIcon{"\uf4b8", "#8957e5"}}, // oct-copilot
		},
		".github/instructions":            {{"*.instructions.md", FileIcon{"\uf4b8", "#8957e5"}}},
		".github/codeql":                  {{"*.yml", codescanIcon}, {"*.yaml", codescanIcon}},
		".github/issue_template":          {{"*", issueIcon}},
		".github/pull_request_template":   {{"*", pullIcon}},
		".github/discussion_template":     {{"*", FileIcon{"\uf442", githubColor}}}, // oct-comment_discussion
		".gitlab/issue_templates":         {{"*", issueIcon}},
		".gitlab/merge_request_templates": {{"*", pullIcon}},
		".circleci": {
			{"config.yml", FileIcon{"\ue78c", "#343434"}}, // dev-circleci
			{"config.yaml", FileIcon{"\ue78c", "#343434"}},
		},
		".azure-pipelines": {{"*.yml", azureIcon}, {"*.yaml", azureIcon}},
	},
	dirPaths: map[string]FileIcon{
		".github/workflows":             workflowIcon,
		".gitea/workflows":              giteaIcon,
		".forgejo/workflows":            giteaIcon,
		".github/issue_template":        templatesIcon,
		".github/pull_request_template": templatesIcon,
		".github/discussion_template":   templatesIcon,
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
})
