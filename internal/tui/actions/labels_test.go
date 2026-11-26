package actions

import "testing"

// helpDescs lists the descriptions of the enabled bindings the help shows.
func helpDescs(m *Modal) map[string]bool {
	got := map[string]bool{}
	for _, l := range m.KeyLayers() {
		for _, b := range l.Bindings {
			if b.Enabled() {
				got[b.Help().Desc] = true
			}
		}
	}
	return got
}

func TestZoomKeyReadsUnzoomWhileZoomed(t *testing.T) {
	t.Parallel()
	m, h := newModal(t, newFake(), wideW, wideH)
	if got := helpDescs(m); !got["zoom"] {
		t.Fatalf("unzoomed help = %v, want zoom", got)
	}
	h.keys("z")
	if got := helpDescs(m); !got["unzoom"] || got["zoom"] {
		t.Errorf("zoomed help = %v, want unzoom, no zoom", got)
	}
	h.keys("z")
	if got := helpDescs(m); !got["zoom"] {
		t.Errorf("help after unzooming = %v, want zoom", got)
	}
}
