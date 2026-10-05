package files

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

func TestMarkdownFile(t *testing.T) {
	for name, want := range map[string]bool{
		"README.md":          true,
		"docs/Guide.MD":      true,
		"notes.markdown":     true,
		"README":             true,
		"sub/readme":         true,
		"README.txt":         false,
		"READMEFIRST":        false,
		"main.go":            false,
		"markdown":           false,
		"docs/md":            false,
		"CHANGELOG.mdx":      false,
		".md":                true,
		"archive.md.tar.gz":  false,
		"dir.md/inside.json": false,
	} {
		if got := markdownFile(name); got != want {
			t.Errorf("markdownFile(%q) = %v, want %v", name, got, want)
		}
	}
}

// sourced returns the top modal of h as a ui.Sourced.
func sourced(t *testing.T, h *host) ui.Sourced {
	t.Helper()
	s, ok := h.top().(ui.Sourced)
	if !ok {
		t.Fatalf("the top modal %T shows no source", h.top())
	}
	return s
}

func TestPreviewRendersMarkdown(t *testing.T) {
	h := openRow(t, sampleFake(), rowAgents)
	v := h.modal()
	if strings.Contains(v, "# AGENTS.md") || !strings.Contains(v, "AGENTS.md") || !strings.Contains(v, "Guidance for anyone.") {
		t.Errorf("preview isn't rendered:\n%s", v)
	}
	if raw, ok := sourced(t, h).Raw(); raw || !ok {
		t.Errorf("Raw() = %v, %v, want false, true", raw, ok)
	}
	// The status line counts the rendered lines.
	if !strings.Contains(v, "line 1/3") {
		t.Errorf("status line doesn't count the rendered lines:\n%s", v)
	}
}

func TestPreviewRawOnAndOff(t *testing.T) {
	h := openRow(t, sampleFake(), rowAgents)
	s := sourced(t, h)
	h.run(s.SetRaw(true))
	if v := h.modal(); !strings.Contains(v, "# AGENTS.md") {
		t.Errorf("raw on doesn't show the source:\n%s", v)
	}
	if raw, ok := s.Raw(); !raw || !ok {
		t.Errorf("Raw() after raw on = %v, %v, want true, true", raw, ok)
	}
	// Again changes nothing.
	if cmd := s.SetRaw(true); cmd != nil {
		t.Error("raw on twice did something")
	}
	h.run(s.SetRaw(false))
	if v := h.modal(); strings.Contains(v, "# AGENTS.md") {
		t.Errorf("raw off doesn't render:\n%s", v)
	}
}

func TestPreviewRawKeepsTheSearch(t *testing.T) {
	h := openRow(t, sampleFake(), rowAgents)
	h.keys(strings.Split("/anyone", "")...)
	h.keys("enter")
	p := h.top().(*preview)
	if p.pager.Matches() != 1 {
		t.Fatalf("%d matches of anyone in the rendered file, want 1", p.pager.Matches())
	}
	h.run(p.SetRaw(true))
	if p.pager.Query() != "anyone" || p.pager.Matches() != 1 {
		t.Errorf("after raw on the search is %q with %d matches", p.pager.Query(), p.pager.Matches())
	}
}

// Search matches what the rendered file shows, not its markup.
func TestPreviewSearchesTheRenderedText(t *testing.T) {
	f := sampleFake()
	f.addBlob(file("AGENTS.md", 0), "# Title\n\nSome **bold** words.\n")
	h := openRow(t, f, rowAgents)
	p := h.top().(*preview)
	h.keys(strings.Split("/Some bold words", "")...)
	h.keys("enter")
	if p.pager.Matches() != 1 {
		t.Errorf("%d matches of the rendered text, want 1:\n%s", p.pager.Matches(), h.modal())
	}
	h.keys("esc")
	h.keys(strings.Split("/**bold**", "")...)
	h.keys("enter")
	if p.pager.Matches() != 0 {
		t.Errorf("%d matches of the markup in the rendered file, want 0", p.pager.Matches())
	}
}

func TestPreviewRawByConfig(t *testing.T) {
	f := sampleFake()
	h := newHost(loaded(t, f, 40, 12, WithMarkdown(config.MarkdownRaw)))
	h.keys("down", "down", "down", "down", "enter")
	if v := h.modal(); !strings.Contains(v, "# AGENTS.md") {
		t.Errorf("files.markdown=raw doesn't show the source:\n%s", v)
	}
	if raw, ok := sourced(t, h).Raw(); !raw || !ok {
		t.Errorf("Raw() = %v, %v, want true, true", raw, ok)
	}
	// The setting changed for the session applies to the next file.
	cfg := config.Default()
	cfg.Files.Markdown = config.MarkdownRendered
	h.run(func() tea.Msg { return ui.SettingsMsg{Config: cfg} })
	h.keys("q")
	h.keys("enter")
	if v := h.modal(); strings.Contains(v, "# AGENTS.md") {
		t.Errorf("files.markdown=rendered set later doesn't render the next file:\n%s", v)
	}
}

// A file opened on a line or a match shows as its source, which they are
// of.
func TestPreviewOpenedOnALineIsRaw(t *testing.T) {
	f := sampleFake()
	h := newHost(loaded(t, f, 40, 12))
	h.run(func() tea.Msg {
		return ui.OpenFileMsg{Repo: ghTUI, Path: "AGENTS.md", SHA: file("AGENTS.md", 0).SHA, Line: 3}
	})
	if raw, _ := sourced(t, h).Raw(); !raw {
		t.Errorf("a file opened on a line is rendered:\n%s", h.modal())
	}
	h.run(func() tea.Msg { return ui.OpenFileMsg{Repo: ghTUI, Path: "AGENTS.md", SHA: file("AGENTS.md", 0).SHA} })
	if raw, _ := sourced(t, h).Raw(); raw {
		t.Errorf("a file opened at its top shows as its source:\n%s", h.modal())
	}
}

func TestPreviewRawOnlyForMarkdown(t *testing.T) {
	for _, row := range []int{rowGoMod, rowLink, rowGitignore, rowReadme} {
		h := openRow(t, sampleFake(), row)
		s := sourced(t, h)
		if _, ok := s.Raw(); ok {
			t.Errorf("%s: Raw() says it renders", h.top().Title())
		}
		if cmd := s.SetRaw(true); cmd != nil {
			t.Errorf("%s: raw on did something", h.top().Title())
		}
	}
}

func TestViewPreviewRaw(t *testing.T) {
	h := newHost(loaded(t, sampleFake(), 40, 12, WithMarkdown(config.MarkdownRaw)))
	h.keys("down", "down", "down", "down", "enter")
	v := h.top().View()
	assertFits(t, v, h.width, h.height)
	golden.RequireEqual(t, v)
}

func TestFinderPreviewRendersMarkdown(t *testing.T) {
	for mode, source := range map[string]bool{config.MarkdownRendered: false, config.MarkdownRaw: true} {
		h := newHost(loaded(t, sampleFake(), 40, 12, fast, WithMarkdown(mode)))
		h.width, h.height = 120, 16
		f := findIn(t, h)
		h.keys(strings.Split("agents", "")...)
		v := ansi.Strip(f.View())
		if !strings.Contains(v, "Guidance for anyone.") {
			t.Fatalf("%s: the finder doesn't preview AGENTS.md:\n%s", mode, v)
		}
		if got := strings.Contains(v, "# AGENTS.md"); got != source {
			t.Errorf("%s: the finder shows the source: %v, want %v:\n%s", mode, got, source, v)
		}
	}
}
