package threads

import (
	"testing"
	"time"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
)

func TestConfigure(t *testing.T) {
	f := newReads()
	o := newOpener(t, f, 1, time.Second)
	c := config.Default()
	c.Prefetch.Notifications.Enabled = new(false)
	o.Configure(c)
	if o.ahead.On() {
		t.Error("the reads ahead didn't stop")
	}
	ns := []core.Notification{note(core.SubjectIssue, 1, 0)}
	run(o, o.ReadAhead(list(ns), 0))
	if got := f.got(); len(got) != 0 {
		t.Errorf("read %q with the reads ahead off", got)
	}
	o.Configure(config.Default())
	if !o.ahead.On() {
		t.Error("the reads ahead didn't start again")
	}
	run(o, o.ReadAhead(list(ns), 0))
	if got := f.got(); len(got) != 1 {
		t.Errorf("read %q, want the issue", got)
	}
	c = config.Default()
	c.Notifications.MarkReadOnOpen = false
	o.Configure(c)
	if o.MarksRead() {
		t.Error("the opener still marks threads read")
	}
	var none *Opener
	none.Configure(config.Default())
}

// TestConfigureWithoutServices checks that an opener with nothing to read
// with reads nothing ahead.
func TestConfigureWithoutServices(t *testing.T) {
	bare := New(t.Context())
	bare.Configure(config.Default())
	ns := []core.Notification{note(core.SubjectIssue, 1, 0), note(core.SubjectRelease, 2, 0)}
	if cmd := bare.ReadAhead(list(ns), 0); cmd != nil {
		run(bare, cmd)
	}
}
