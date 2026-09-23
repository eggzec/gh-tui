package tui

import (
	"bytes"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/exp/teatest/v2"
)

func newTestModel(t *testing.T) *teatest.TestModel {
	t.Helper()
	return teatest.NewTestModel(t, New(), teatest.WithInitialTermSize(80, 24))
}

func TestRendersTitleAndHelp(t *testing.T) {
	tm := newTestModel(t)
	teatest.WaitFor(t, tm.Output(), func(out []byte) bool {
		return bytes.Contains(out, []byte("gh-tui")) && bytes.Contains(out, []byte("quit"))
	}, teatest.WithDuration(time.Second))
	if err := tm.Quit(); err != nil {
		t.Fatal(err)
	}
}

func TestQuitKeys(t *testing.T) {
	for _, k := range []tea.KeyPressMsg{
		{Code: 'q', Text: "q"},
		{Code: 'c', Mod: tea.ModCtrl},
	} {
		t.Run(k.String(), func(t *testing.T) {
			tm := newTestModel(t)
			tm.Send(k)
			tm.WaitFinished(t, teatest.WithFinalTimeout(time.Second))
		})
	}
}

func TestTracksWindowSize(t *testing.T) {
	tm := newTestModel(t)
	tm.Send(tea.WindowSizeMsg{Width: 120, Height: 40})
	if err := tm.Quit(); err != nil {
		t.Fatal(err)
	}
	m := tm.FinalModel(t, teatest.WithFinalTimeout(time.Second)).(*Model)
	if m.width != 120 || m.height != 40 {
		t.Errorf("size = %dx%d, want 120x40", m.width, m.height)
	}
}

func BenchmarkView(b *testing.B) {
	m := New()
	b.ReportAllocs()
	for b.Loop() {
		_ = m.View()
	}
}

func BenchmarkUpdate(b *testing.B) {
	var m tea.Model = New()
	msg := tea.WindowSizeMsg{Width: 120, Height: 40}
	b.ReportAllocs()
	for b.Loop() {
		m, _ = m.Update(msg)
	}
}
