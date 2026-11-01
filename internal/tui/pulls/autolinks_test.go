package pulls

import (
	"context"
	"testing"

	"github.com/eggzec/gh-tui/internal/service/autolinks"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/markdown"
)

// syncLinks answers every reference with its url, which a test may change.
type syncLinks struct{ url string }

func (f *syncLinks) Cached(_ string, cands []autolinks.Candidate) []autolinks.Result {
	out := make([]autolinks.Result, len(cands))
	for i := range cands {
		out[i] = autolinks.Result{Known: f.url != "", Exists: f.url != "", URL: f.url}
	}
	return out
}

func (f *syncLinks) Verify(context.Context, string, []autolinks.Candidate) []autolinks.Result {
	return nil
}

// linksModal is a modal with its conversation read, whose section links with
// f and has drawn a reference.
func linksModal(tb testing.TB, f *syncLinks) *panesModal {
	tb.Helper()
	links := ui.NewAutolinks(f, "github.com")
	svc := convService()
	h := started(tb, svc, 160, 30, WithAutolinks(links))
	press(tb, h, "enter")
	m := h.modal()
	if m == nil || !m.conv.started {
		tb.Fatalf("enter opened %v, want a modal with its conversation read", m)
	}
	m.links().Func("o/r")(markdown.RefIssue, "1")
	return m
}

func TestLinksSyncedDrawsOnlyWhatChanged(t *testing.T) {
	t.Parallel()
	f := &syncLinks{}
	m := linksModal(t, f)
	msg := ui.SyncMsg{Key: autolinks.SyncKey("github.com")}

	links, version, conv := m.desc.links, m.desc.version, m.conv.version
	m.linksSynced(msg)
	if m.desc.links != links || m.desc.version != version || m.conv.version != conv {
		t.Errorf("an unchanged sync redrew: links %d->%d, version %d->%d, conversation %d->%d",
			links, m.desc.links, version, m.desc.version, conv, m.conv.version)
	}

	f.url = "https://github.com/o/r/issues/1"
	m.linksSynced(msg)
	if m.desc.links != links+1 || m.desc.version != version+1 {
		t.Errorf("a changed sync: links %d->%d, version %d->%d, want one more each",
			links, m.desc.links, version, m.desc.version)
	}
}

func BenchmarkLinksSynced(b *testing.B) {
	m := linksModal(b, &syncLinks{})
	msg := ui.SyncMsg{Key: autolinks.SyncKey("github.com")}
	b.ReportAllocs()
	for b.Loop() {
		m.linksSynced(msg)
	}
}
