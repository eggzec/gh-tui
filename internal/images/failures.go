package images

import (
	"container/list"
	"time"
)

// maxFailed is how many images that failed the fetcher remembers at
// most. Past it, the oldest failure is forgotten first: it is the one
// soonest due to be tried again anyway, and the views remember what
// failed for what they draw, so a failure forgotten early costs a fetch
// only of an image drawn again.
const maxFailed = 1024

// failures remembers which images failed, until failedFor has passed, and
// at most maxFailed of them. It is not safe for concurrent use.
type failures struct {
	order  *list.List // of *failure, oldest first
	byAddr map[string]*list.Element
}

type failure struct {
	addr  string
	err   error
	until time.Time
}

func newFailures() *failures {
	return &failures{order: list.New(), byAddr: make(map[string]*list.Element)}
}

// get returns the error the image at addr failed with, if it failed and
// isn't due to be tried again at now, or nil.
func (fs *failures) get(addr string, now time.Time) error {
	e, ok := fs.byAddr[addr]
	if !ok {
		return nil
	}
	if f := e.Value.(*failure); now.Before(f.until) {
		return f.err
	}
	return nil
}

// add remembers that the image at addr failed with err at now, and
// forgets the failures due to be tried again, then the oldest past
// maxFailed. Every failure lasts as long, so the oldest is due first.
func (fs *failures) add(addr string, err error, now time.Time) {
	fs.forget(addr)
	fs.byAddr[addr] = fs.order.PushBack(&failure{addr: addr, err: err, until: now.Add(failedFor)})
	for e := fs.order.Front(); e != nil; e = fs.order.Front() {
		if f := e.Value.(*failure); now.Before(f.until) && fs.order.Len() <= maxFailed {
			break
		}
		fs.remove(e)
	}
}

// forget forgets that the image at addr failed.
func (fs *failures) forget(addr string) {
	if e, ok := fs.byAddr[addr]; ok {
		fs.remove(e)
	}
}

func (fs *failures) remove(e *list.Element) {
	fs.order.Remove(e)
	delete(fs.byAddr, e.Value.(*failure).addr)
}

// len returns how many failures are remembered.
func (fs *failures) len() int { return fs.order.Len() }
