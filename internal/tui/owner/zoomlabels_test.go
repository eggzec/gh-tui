package owner

import "testing"

// descs lists the descriptions of the enabled bindings that the help shows.
func descs(s *Section) map[string]bool {
	got := map[string]bool{}
	for _, l := range s.KeyLayers() {
		for _, b := range l.Bindings {
			if b.Enabled() {
				got[b.Help().Desc] = true
			}
		}
	}
	return got
}

// A screen that is short forces the zoom, and Tab then flips the list and
// the README, though the user zoomed nothing.
func TestTabReadsSwitchSplitUnderForcedZoom(t *testing.T) {
	t.Parallel()
	s := newSection(t, newFake(), "octocat", 120, 40)
	s.SetNarrow(true)
	s.SetSize(120, 40)
	run(t, s, s.Update(nil))
	got := descs(s)
	if !got["switch split"] || got["next pane"] {
		t.Fatalf("forced zoom help = %v, want switch split, no next pane", got)
	}
	if got["unzoom"] {
		t.Errorf("forced zoom help = %v, want no unzoom", got)
	}
}
