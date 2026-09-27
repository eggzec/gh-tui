package ui

import (
	"path"
	"slices"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
)

// glob is the icon of the files that match pattern.
func glob(pattern string) FileIcon {
	for _, g := range nerdFiles.globs {
		if g.glob == pattern {
			return g.icon
		}
	}
	panic("no glob " + pattern)
}

func file(name string) core.TreeEntry { return core.TreeEntry{Name: name, Type: core.EntryBlob} }
func dir(name string) core.TreeEntry  { return core.TreeEntry{Name: name, Type: core.EntryTree} }

// fileAt and dirAt are entries at p from the root of the repository.
func fileAt(p string) core.TreeEntry {
	return core.TreeEntry{Path: p, Name: path.Base(p), Type: core.EntryBlob}
}

func dirAt(p string) core.TreeEntry {
	return core.TreeEntry{Path: p, Name: path.Base(p), Type: core.EntryTree}
}

// at is the icon of the files named name in dir.
func at(dir, name string) FileIcon {
	fi, ok := match(nerdFiles.paths[dir], name)
	if !ok {
		panic("no icon for " + dir + "/" + name)
	}
	return fi
}

func TestFileIconsAreOneCellWide(t *testing.T) {
	f := nerdFiles
	icons := []FileIcon{f.file, f.dir, f.dirOpen, f.submodule, f.symlink}
	for _, m := range []map[string]FileIcon{f.names, f.stems, f.exts, f.dirs} {
		for _, fi := range m {
			icons = append(icons, fi)
		}
	}
	for _, g := range f.globs {
		icons = append(icons, g.icon)
	}
	for _, gs := range f.paths {
		for _, g := range gs {
			icons = append(icons, g.icon)
		}
	}
	for _, fi := range f.dirPaths {
		icons = append(icons, fi)
	}
	for _, fi := range icons {
		if w := ansi.StringWidth(fi.Glyph); w != 1 || len([]rune(fi.Glyph)) != 1 {
			t.Errorf("glyph %q is %d cells wide, want 1", fi.Glyph, w)
		}
		if _, ok := luminance(fi.Color); fi.Color != "" && !ok {
			t.Errorf("glyph %q has color %q, want a hex color", fi.Glyph, fi.Color)
		}
	}
}

func TestFileIconGlobsAreValid(t *testing.T) {
	globs := slices.Clone(nerdFiles.globs)
	for dir, gs := range nerdFiles.paths {
		if dir != strings.ToLower(dir) {
			t.Errorf("path %q should be lower case, since paths are", dir)
		}
		globs = append(globs, gs...)
	}
	for _, g := range globs {
		if _, err := path.Match(g.glob, ""); err != nil {
			t.Errorf("glob %q: %v", g.glob, err)
		}
		if g.glob != strings.ToLower(g.glob) {
			t.Errorf("glob %q should be lower case, since names are", g.glob)
		}
	}
	for p := range nerdFiles.dirPaths {
		if p != strings.ToLower(p) {
			t.Errorf("path %q should be lower case, since paths are", p)
		}
	}
}

func TestEntryIconByPath(t *testing.T) {
	ic := NewIcons(config.IconsNerd)
	f := nerdFiles
	actions := at(".github/workflows", "ci.yml")
	tests := []struct {
		entry core.TreeEntry
		open  bool
		want  FileIcon
	}{
		// GitHub reads workflows at the top of .github/workflows only.
		{entry: fileAt(".github/workflows/ci.yml"), want: actions},
		{entry: fileAt(".github/workflows/release.yaml"), want: actions},
		{entry: fileAt(".GitHub/Workflows/CI.YML"), want: actions},
		{entry: fileAt(".github/workflows/sub/ci.yml"), want: lang("YAML")},
		{entry: fileAt("docs/.github/workflows/ci.yml"), want: lang("YAML")},
		{entry: fileAt(".github/workflows/README.md"), want: f.stems["readme"]},
		{entry: fileAt(".github/workflows/ci.yml.orig"), want: f.file},
		{entry: fileAt(".gitea/workflows/ci.yml"), want: at(".gitea/workflows", "x.yml")},
		{entry: fileAt(".forgejo/workflows/ci.yaml"), want: at(".gitea/workflows", "x.yml")},
		{entry: fileAt(".github/ISSUE_TEMPLATE/bug.yml"), want: at(".github/issue_template", "x")},
		{entry: fileAt(".github/ISSUE_TEMPLATE/config.yml"), want: at(".github/issue_template", "x")},
		{entry: fileAt(".github/PULL_REQUEST_TEMPLATE/feature.md"), want: at(".github/pull_request_template", "x")},
		{entry: fileAt(".github/DISCUSSION_TEMPLATE/ideas.yml"), want: at(".github/discussion_template", "x")},
		{entry: fileAt(".gitlab/issue_templates/bug.md"), want: at(".github/issue_template", "x")},
		{entry: fileAt(".gitlab/merge_request_templates/default.md"), want: at(".github/pull_request_template", "x")},
		{entry: fileAt(".github/FUNDING.yml"), want: at(".github", "funding.yml")},
		{entry: fileAt(".github/release.yml"), want: at(".github", "release.yml")},
		{entry: fileAt(".github/labeler.yml"), want: at(".github", "labeler.yml")},
		{entry: fileAt(".github/copilot-instructions.md"), want: at(".github", "copilot-instructions.md")},
		{entry: fileAt(".github/instructions/go.instructions.md"), want: at(".github", "copilot-instructions.md")},
		{entry: fileAt(".github/codeql/codeql-config.yml"), want: at(".github/codeql", "x.yml")},
		{entry: fileAt(".circleci/config.yml"), want: at(".circleci", "config.yml")},
		{entry: fileAt(".azure-pipelines/build.yaml"), want: glob("azure-pipelines*.yaml")},
		// A release.yml elsewhere is plain YAML.
		{entry: fileAt("release.yml"), want: lang("YAML")},
		{entry: fileAt("deploy/release.yml"), want: lang("YAML")},
		// The path beats the name, which beats a glob, which beats the stem,
		// which beats the extension.
		{entry: fileAt(".github/ISSUE_TEMPLATE/action.yml"), want: at(".github/issue_template", "x")},
		{entry: fileAt(".github/workflows/compose.ci.yml"), want: actions},
		{entry: fileAt("tools/action.yml"), want: f.names["action.yml"]},
		{entry: fileAt("compose.yml"), want: f.names["compose.yml"]},
		{entry: fileAt("swagger.json"), want: glob("swagger*.json")},
		{entry: fileAt(".prettierrc.md"), want: glob(".prettierrc*")},
		{entry: fileAt("docs/README.md"), want: f.stems["readme"]},
		// Directories at a known place.
		{entry: dirAt(".github/workflows"), want: actions},
		{entry: dirAt(".GitHub/Workflows"), open: true, want: actions},
		{entry: dirAt(".gitea/workflows"), want: at(".gitea/workflows", "x.yml")},
		{entry: dirAt(".forgejo/workflows/"), open: true, want: at(".gitea/workflows", "x.yml")},
		{entry: dirAt("./.github/workflows"), want: actions},
		{entry: dirAt(".github/ISSUE_TEMPLATE"), want: f.dirs[".github"]},
		{entry: dirAt("sub/.github/workflows"), want: f.dir},
		{entry: dirAt("workflows"), want: f.dir},
		{entry: dirAt(".circleci"), want: f.dirs[".circleci"]},
		{entry: dirAt(".devcontainer"), want: f.dirs[".devcontainer"]},
		{entry: fileAt(".devcontainer/go/devcontainer.json"), want: f.names["devcontainer.json"]},
	}
	for _, tt := range tests {
		if got := ic.Entry(tt.entry, tt.open); got != tt.want {
			t.Errorf("Entry(%s %q, open %v) = %q %s, want %q %s",
				tt.entry.Type, tt.entry.Path, tt.open, got.Glyph, got.Color, tt.want.Glyph, tt.want.Color)
		}
	}
}

func TestFileIconGlobIndex(t *testing.T) {
	names := make([]string, 0, len(nerdFiles.globs))
	for _, g := range nerdFiles.globs {
		names = append(names, strings.ReplaceAll(g.glob, "*", "x"))
	}
	names = append(names, "compose.ci.yml", "docker-compose.yaml", "tsconfig.base.json",
		".eslintrc", ".eslintrc.json", "eslint.config.js", ".prettierrc.yml", "openapi.json",
		"swagger.yaml", "azure-pipelines.yml", "main.go", "procfile", "x.yml", "x.json")
	for _, name := range names {
		want, wantOK := match(nerdFiles.globs, name)
		got, ok := match(nerdFiles.extGlobs(path.Ext(name)), name)
		if got != want || ok != wantOK {
			t.Errorf("%q: indexed globs give %q %v, want %q %v", name, got.Glyph, ok, want.Glyph, wantOK)
		}
	}
	if n := len(nerdFiles.extGlobs(".go")); n > 4 {
		t.Errorf("a Go file tries %d globs, want at most 4", n)
	}
}

func TestEntryIcon(t *testing.T) {
	ic := NewIcons(config.IconsNerd)
	f := nerdFiles
	tests := []struct {
		entry core.TreeEntry
		open  bool
		want  FileIcon
	}{
		// The name beats the extension, in any case.
		{entry: file("go.mod"), want: lang("Go")},
		{entry: file("Cargo.lock"), want: lang("Rust")},
		{entry: file("requirements.txt"), want: lang("Python")},
		{entry: file("Dockerfile"), want: lang("Dockerfile")},
		{entry: file("GNUmakefile"), want: lang("Makefile")},
		{entry: file(".gitignore"), want: f.names[".gitignore"]},
		{entry: file("PACKAGE.JSON"), want: f.names["package.json"]},
		{entry: file("CODEOWNERS"), want: f.names["codeowners"]},
		{entry: file("action.yml"), want: f.names["action.yml"]},
		{entry: file("Jenkinsfile"), want: f.names["jenkinsfile"]},
		{entry: file("PULL_REQUEST_TEMPLATE.md"), want: f.names["pull_request_template.md"]},
		{entry: file(".golangci.yml"), want: f.names[".golangci.yml"]},
		{entry: file("pnpm-lock.yaml"), want: f.names["pnpm-lock.yaml"]},
		{entry: file("Chart.lock"), want: f.names["chart.lock"]},
		// Then a glob of the name.
		{entry: file("compose.prod.yaml"), want: lang("Dockerfile")},
		{entry: file("docker-compose.override.yml"), want: lang("Dockerfile")},
		{entry: file("tsconfig.json"), want: glob("tsconfig*.json")},
		{entry: file("TSConfig.Build.json"), want: glob("tsconfig*.json")},
		{entry: file(".eslintrc.cjs"), want: glob(".eslintrc*")},
		{entry: file("prettier.config.mjs"), want: glob("prettier.config.*")},
		{entry: file("openapi.v2.YAML"), want: glob("openapi*.yaml")},
		{entry: file("azure-pipelines.yml"), want: glob("azure-pipelines*.yml")},
		{entry: file("compose.yml.md"), want: lang("Markdown")},
		// Documents are named by their stem, with a text extension or none.
		{entry: file("LICENSE"), want: f.stems["license"]},
		{entry: file("License.md"), want: f.stems["license"]},
		{entry: file("LICENSE.txt"), want: f.stems["license"]},
		{entry: file("README.md"), want: f.stems["readme"]},
		{entry: file("readme"), want: f.stems["readme"]},
		{entry: file("license.go"), want: lang("Go")},
		// Then the extension, in any case.
		{entry: file("main.go"), want: lang("Go")},
		{entry: file("App.TSX"), want: f.exts["tsx"]},
		{entry: file("notes.md"), want: lang("Markdown")},
		{entry: file("release.tar.gz"), want: f.exts["zip"]},
		{entry: file(".env"), want: f.exts["env"]},
		// Then a plain file.
		{entry: file("data.xyz"), want: f.file},
		{entry: file("AUTHORS"), want: f.file},
		{entry: file("archive."), want: f.file},
		// Folders open and close, unless they have their own glyph.
		{entry: dir("internal"), want: f.dir},
		{entry: dir("internal"), open: true, want: f.dirOpen},
		{entry: dir("go.mod"), want: f.dir},
		{entry: dir(".github"), want: f.dirs[".github"]},
		{entry: dir(".GitHub"), open: true, want: f.dirs[".github"]},
		{entry: dir("Tests"), want: f.dirs["tests"]},
		{entry: core.TreeEntry{Name: "vendor", Type: core.EntryCommit}, want: f.submodule},
		{entry: core.TreeEntry{Name: "main.go", Type: core.EntryBlob, Mode: core.ModeSymlink}, want: f.symlink},
	}
	for _, tt := range tests {
		if got := ic.Entry(tt.entry, tt.open); got != tt.want {
			t.Errorf("Entry(%s %q, open %v) = %q %s, want %q %s",
				tt.entry.Type, tt.entry.Name, tt.open, got.Glyph, got.Color, tt.want.Glyph, tt.want.Color)
		}
	}
	if ic.Entry(dir("a"), false) == ic.Entry(dir("a"), true) {
		t.Error("an open folder should look apart from a closed one")
	}
	if ic.Entry(file("a.go"), false) == ic.Entry(file("a.rs"), false) {
		t.Error("Go and Rust files should look apart")
	}
}

func TestEntryIconOtherSets(t *testing.T) {
	for _, set := range []string{config.IconsUnicode, config.IconsASCII} {
		ic := NewIcons(set)
		for _, e := range []core.TreeEntry{file("go.mod"), file("main.go"), file("x"), dir("docs"), dir(".github")} {
			for _, open := range []bool{false, true} {
				if got := ic.Entry(e, open); got != (FileIcon{}) {
					t.Errorf("%s: Entry(%q, %v) = %q, want none", set, e.Name, open, got.Glyph)
				}
			}
		}
	}
	if NewIcons("bogus").Entry(file("main.go"), false) != NewIcons(config.IconsNerd).Entry(file("main.go"), false) {
		t.Error("an unknown set should get the Nerd Font glyphs")
	}
}

func TestThemeFileIcon(t *testing.T) {
	p, err := config.Default().Palette(true)
	if err != nil {
		t.Fatal(err)
	}
	dark := NewTheme(p, true)
	if p, err = config.Default().Palette(false); err != nil {
		t.Fatal(err)
	}
	light := NewTheme(p, false)
	tests := []struct {
		name  string
		th    Theme
		color string
		want  any
	}{
		{"go on dark", dark, "#00ADD8", lipgloss.Color("#00ADD8")},
		{"no color", dark, "", dark.Muted.GetForeground()},
		// Pale grey vanishes on a light background, and navy on a dark one.
		{"grey on light", light, "#dddddd", light.Muted.GetForeground()},
		{"grey on dark", dark, "#dddddd", lipgloss.Color("#dddddd")},
		{"navy on dark", dark, "#000080", dark.Muted.GetForeground()},
	}
	for _, tt := range tests {
		if got := tt.th.FileIcon(FileIcon{Glyph: "x", Color: tt.color}).GetForeground(); got != tt.want {
			t.Errorf("%s: foreground %v, want %v", tt.name, got, tt.want)
		}
	}
}
