package releases

import "testing"

func TestZoomKeyReadsUnzoomWhileZoomed(t *testing.T) {
	t.Parallel()
	m := newModal(&fakeService{}, 100, 30)
	run(t, m, m.Init())
	if got := enabledKeys(m); !got["zoom"] || got["unzoom"] {
		t.Fatalf("unzoomed help = %v, want zoom only", got)
	}
	press(m, "z")
	if got := enabledKeys(m); !got["unzoom"] || got["zoom"] {
		t.Errorf("zoomed help = %v, want unzoom only", got)
	}
	press(m, "z")
	if got := enabledKeys(m); !got["zoom"] || got["unzoom"] {
		t.Errorf("help after unzooming = %v, want zoom only", got)
	}
}
