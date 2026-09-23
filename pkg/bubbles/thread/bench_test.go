package thread

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// longThread is an issue with a long body and 500 comments in chunks of 50,
// all loaded, at a common terminal size.
func longThread(b *testing.B) (Model[comment], *renders) {
	b.Helper()
	src := newSource(10, 50)
	r := &renders{}
	m := newTest(src, r, 120, 40)
	body := strings.Repeat(testBody+"\n\n", 20)
	m = drain(b, m, m.SetDocument(testHeader, body))
	for !m.done() {
		m = press(b, m, "G")
	}
	return m, r
}

func BenchmarkView(b *testing.B) {
	m, r := longThread(b)
	runs, items := m.mdRuns, r.count()
	lines := m.TotalLines() - m.Height()
	b.ReportAllocs()
	i := 0
	for b.Loop() {
		// Scroll through the whole thread, a line per frame.
		m.vp.SetYOffset(i % lines)
		_ = m.View()
		i++
	}
	if m.mdRuns != runs || r.count() != items {
		b.Fatal("View rendered markdown or comments")
	}
}

func BenchmarkUpdate(b *testing.B) {
	m, r := longThread(b)
	runs, items := m.mdRuns, r.count()
	m.vp.SetYOffset(0)
	s := scroller{m: m, msg: keyMsg("j")}
	b.ReportAllocs()
	for b.Loop() {
		s.step()
	}
	if s.m.mdRuns != runs || r.count() != items {
		b.Fatal("scrolling rendered markdown or comments")
	}
}

// scroller scrolls down to the bottom and back up, a line per key. It lives
// outside the b.Loop body because go1.27.1 crashes compiling a call to
// Model.AtBottom inlined there.
type scroller struct {
	m   Model[comment]
	msg tea.KeyPressMsg
}

func (s *scroller) step() {
	switch {
	case s.m.AtBottom():
		s.msg = keyMsg("k")
	case s.m.YOffset() == 0:
		s.msg = keyMsg("j")
	}
	s.m, _ = s.m.Update(s.msg)
}
