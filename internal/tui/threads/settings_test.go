package threads

import (
	"testing"
	"time"

	"github.com/eggzec/gh-tui/internal/config"
)

func TestConfigure(t *testing.T) {
	o := newOpener(t, newReads(), 2, time.Second)
	c := config.Default()
	c.Details.Prefetch.Rows, c.Details.Prefetch.HoverDelay = 4, 2*time.Second
	o.Configure(c)
	if o.ahead == nil || o.rows != 4 || o.delay != 2*time.Second {
		t.Errorf("ahead %v, rows %d, delay %v; want 4 rows after 2s", o.ahead != nil, o.rows, o.delay)
	}
	c.Details.Prefetch.Enabled = false
	o.Configure(c)
	if o.ahead != nil {
		t.Error("the reads ahead didn't stop")
	}
	o.Configure(config.Default())
	if o.ahead == nil {
		t.Error("the reads ahead didn't start again")
	}
	bare := New(t.Context())
	bare.Configure(config.Default())
	if bare.ahead != nil {
		t.Error("an opener with nothing to read with reads ahead")
	}
	var none *Opener
	none.Configure(config.Default())
}
