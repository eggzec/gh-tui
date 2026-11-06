package issues

import (
	"context"
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/autolinks"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/markdown"
)

// previewLinks answers every reference as known once known is set.
type previewLinks struct{ known bool }

func (f *previewLinks) Cached(_ string, cands []autolinks.Candidate) []autolinks.Result {
	out := make([]autolinks.Result, len(cands))
	for i := range cands {
		out[i] = autolinks.Result{Known: f.known, Exists: f.known, URL: "https://github.com/o/r/issues/1"}
	}
	return out
}

func (f *previewLinks) Verify(context.Context, string, []autolinks.Candidate) []autolinks.Result {
	return nil
}

func TestLinksSyncDrawsPreviewAgain(t *testing.T) {
	t.Parallel()
	f := &previewLinks{}
	svc := blockedSvc()
	svc.addComments(999, core.Comment{ID: "IC_p", Author: core.User{Login: "ann"}, Body: "Peeked at."})
	h := started(t, svc, 110, 30, WithAutolinks(ui.NewAutolinks(f, "github.com")))
	press(t, h, "down", "enter")
	m := h.modal()
	if m == nil {
		t.Fatal("enter didn't open the issue")
	}
	press(t, h, "3")
	m.links.Func(testRepo.String())(markdown.RefIssue, "1")
	m.Update(strayMsg{})
	m.View()
	ver := m.peekVer
	f.known = true
	m.Update(ui.SyncMsg{Key: autolinks.SyncKey("github.com")})
	if m.peekVer == ver {
		t.Error("the links changed and the preview was not drawn again")
	}
}
