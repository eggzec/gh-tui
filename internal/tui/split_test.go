package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/exp/golden"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// splitFake is a section with a preview box, which records its size.
type splitFake struct {
	fakeSection
	sizes [][2]int
	cmd   tea.Cmd // handed on once by PreviewCmd
}

type previewCmdMsg struct{}

func (s *splitFake) SetPreviewSize(w, h int) { s.sizes = append(s.sizes, [2]int{w, h}) }
func (s *splitFake) PreviewTitle() string    { return "Preview" }
func (s *splitFake) PreviewView() string     { return "preview content" }
func (s *splitFake) PreviewCmd() tea.Cmd {
	cmd := s.cmd
	s.cmd = nil
	return cmd
}

var _ ui.Splitter = (*splitFake)(nil)

func newSplitApp(t *testing.T, w, h int) (*Model, *splitFake) {
	t.Helper()
	files := &splitFake{}
	files.title = "Files"
	others := []*fakeSection{{title: "Pull requests"}, {title: "Issues"}, {title: "Notifications"}}
	layout := Layout{Files: files, Pulls: others[0], Issues: others[1], Notifications: others[2]}
	m := New(t.Context(), config.Default(), layout, WithRepo(testRepo))
	rankFakes(m)
	m.toast.SetDuration(0)
	m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	return m, files
}

func TestSplitZoomed(t *testing.T) {
	t.Parallel()
	m, files := newSplitApp(t, 120, 40)
	run(m, m.key(press("z")))
	if got, want := files.sizes[len(files.sizes)-1], [2]int{120 - 46 - 2, m.contentHeight() - 2}; got != want {
		t.Fatalf("preview size = %v, want %v", got, want)
	}
	if files.width != 44 {
		t.Errorf("list width = %d, want 44 inside the frame", files.width)
	}
	if s := onScreen(m); !strings.Contains(s, "preview content") || !strings.Contains(s, "Preview") {
		t.Errorf("screen has no preview box:\n%s", s)
	}
	golden.RequireEqual(t, m.View().Content)
}

// The command the preview queues is returned by Update, once.
func TestSplitPreviewCmd(t *testing.T) {
	t.Parallel()
	m, files := newSplitApp(t, 120, 40)
	run(m, m.key(press("z")))
	files.cmd = func() tea.Msg { return previewCmdMsg{} }
	_, cmd := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	if cmd == nil {
		t.Fatal("Update returned no command")
	}
	var found func(tea.Msg) bool
	found = func(msg tea.Msg) bool {
		if b, ok := msg.(tea.BatchMsg); ok {
			for _, c := range b {
				if c != nil && found(c()) {
					return true
				}
			}
			return false
		}
		_, ok := msg.(previewCmdMsg)
		return ok
	}
	if !found(cmd()) {
		t.Error("the preview's command is not in what Update returned")
	}
	if _, cmd := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40}); cmd != nil && found(cmd()) {
		t.Error("the preview's command was handed on twice")
	}
}

func TestSplitNarrow(t *testing.T) {
	t.Parallel()
	m, files := newSplitApp(t, 80, 24)
	run(m, m.key(press("z")))
	if s := onScreen(m); strings.Contains(s, "preview content") {
		t.Errorf("a narrow screen has a preview box:\n%s", s)
	}
	for _, size := range files.sizes {
		if size != [2]int{} {
			t.Errorf("SetPreviewSize%v on a narrow screen", size)
		}
	}
}

func TestSplitUnzoomed(t *testing.T) {
	t.Parallel()
	m, files := newSplitApp(t, 120, 40)
	if len(files.sizes) != 0 {
		t.Errorf("SetPreviewSize called %v while unzoomed", files.sizes)
	}
	run(m, m.key(press("z")))
	run(m, m.key(press("z")))
	if last := files.sizes[len(files.sizes)-1]; last != [2]int{} {
		t.Errorf("preview size after unzoom = %v, want 0x0", last)
	}
}
