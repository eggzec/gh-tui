package releases

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// The commit and contributor rows draw only once the extra part arrived, and
// the header gets taller for it.
func TestHeaderRowsAfterExtra(t *testing.T) {
	t.Parallel()
	svc := &fakeService{cached: true}
	m := newModal(svc, 100, 30)
	if v := ansi.Strip(m.header()); strings.Contains(v, "since") || strings.Contains(v, "@alice") {
		t.Fatalf("header before the extra part read:\n%s", v)
	}
	before := m.headerRows()
	m.update(extraMsg{id: m.id, extra: extra3})
	v := ansi.Strip(m.header())
	for _, want := range []string{"0123456", "3 commits to main since", "@alice", "@bob", "downloads", "released this"} {
		if !strings.Contains(v, want) {
			t.Errorf("header lacks %q:\n%s", want, v)
		}
	}
	if m.headerRows() <= before {
		t.Errorf("header rows = %d, want more than %d", m.headerRows(), before)
	}
	if strings.Contains(v, "target") || strings.Contains(v, "files ") {
		t.Errorf("header still has a target or files row:\n%s", v)
	}
}

// An extra part of another modal, or one that failed, changes nothing.
func TestExtraMsgDroppedByID(t *testing.T) {
	t.Parallel()
	m := newModal(&fakeService{cached: true}, 100, 30)
	m.update(extraMsg{id: m.id + 1, extra: extra3})
	if m.extraDone {
		t.Error("an extra part of another modal was taken")
	}
	m.update(extraMsg{id: m.id, err: errors.New("boom")})
	if m.extraDone {
		t.Error("a failed read counts as done")
	}
}

// An extra part that got no answer is read again once GitHub answers.
func TestExtraRetriedWhenOnline(t *testing.T) {
	t.Parallel()
	svc := &fakeService{cached: true, extraErr: core.ErrOffline}
	m := newModal(svc, 100, 30)
	feed := func(cmd tea.Cmd) {
		for _, msg := range run(t, m, cmd) {
			m.Update(msg)
		}
	}
	feed(m.Init())
	if m.extraDone || svc.extras != 1 {
		t.Fatalf("after the failed read: done = %v, reads = %d", m.extraDone, svc.extras)
	}
	svc.mu.Lock()
	svc.extraErr = nil
	svc.mu.Unlock()
	feed(m.Update(ui.OnlineMsg{}))
	if svc.extras != 2 {
		t.Errorf("extra reads = %d, want 2", svc.extras)
	}
	if v := ansi.Strip(m.header()); !strings.Contains(v, "@alice") {
		t.Errorf("header lacks the extra rows after the retry:\n%s", v)
	}
}

// When the compare failed, the commit shows without the text of the count.
func TestHeaderCompareHidden(t *testing.T) {
	t.Parallel()
	m := newModal(&fakeService{cached: true}, 100, 30)
	x := extra3
	x.AheadBy = -1
	m.update(extraMsg{id: m.id, extra: x})
	v := ansi.Strip(m.header())
	if !strings.Contains(v, "0123456") || strings.Contains(v, "since") {
		t.Errorf("header with an unknown count:\n%s", v)
	}
}

// With no default branch known, the count names none.
func TestHeaderNoDefault(t *testing.T) {
	t.Parallel()
	m := newModal(&fakeService{cached: true}, 100, 30)
	x := extra3
	x.Default = ""
	m.update(extraMsg{id: m.id, extra: x})
	if v := ansi.Strip(m.header()); !strings.Contains(v, "3 commits since") {
		t.Errorf("header without a default branch:\n%s", v)
	}
}

// Where avatars are not drawn the contributors show as @handles, and the
// ones that don't fit are counted.
func TestHeaderHandlesAndOthers(t *testing.T) {
	t.Parallel()
	m := newModal(&fakeService{cached: true}, 100, 30)
	m.update(extraMsg{id: m.id, extra: extra3})
	v := ansi.Strip(m.header())
	for _, want := range []string{"@github-actions[bot]", "@alice", "@bob", "@octocat"} {
		if !strings.Contains(v, want) {
			t.Errorf("wide header lacks %q:\n%s", want, v)
		}
	}
	if strings.Contains(v, "others") {
		t.Errorf("wide header counts others:\n%s", v)
	}

	n := newModal(&fakeService{cached: true}, 40, 30)
	n.update(extraMsg{id: n.id, extra: extra3})
	v = ansi.Strip(n.header())
	if !strings.Contains(v, "and 3 others") || strings.Contains(v, "@octocat") {
		t.Errorf("narrow header:\n%s", v)
	}
}

// A cached extra part shows at once on opening.
func TestCachedExtraShowsAtOnce(t *testing.T) {
	t.Parallel()
	m := newModal(&fakeService{cached: true, extra: core.ReleaseExtra{TagSHA: "abcdef0123", AheadBy: 1}, extraCached: true}, 100, 30)
	if v := ansi.Strip(m.header()); !strings.Contains(v, "abcdef0") || !strings.Contains(v, "1 commit since") {
		t.Errorf("cached header:\n%s", v)
	}
}
