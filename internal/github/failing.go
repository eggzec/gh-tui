package github

import "time"

const (
	// failingAfter is how long server errors must keep coming before the
	// status says GitHub fails. A 502 that a retry mends a moment later
	// isn't an outage, and telling of it would only flash the bar.
	failingAfter = 5 * time.Second
	// failingClear is how long GitHub must go without a server error, and
	// answer well meanwhile, before failing ends. Without it a resource
	// that fails now and then, such as GraphQL 502s every other poll,
	// would flap the bar and wake the polls each time.
	failingClear = 30 * time.Second
	// failingResume is how long after its last error a streak that
	// lapsed may be taken up again, keeping its start. Polls back off to
	// ten minutes, longer while unfocused, so a resource polled that
	// seldom still keeps the start of its outage, while an error hours
	// later starts a streak of its own.
	failingResume = time.Hour
)

// failStreak is a run of server errors in the answers of one resource.
// start is when its first error came, which the status tells, and from
// when the errors it was last taken up with began, which whether it
// shows goes by; last is when the latest came. firstOK and ok are when
// GitHub first and last answered a request of the resource with
// anything else since from, and mendOK first since last. showed is
// whether it showed before it was last taken up.
type failStreak struct {
	start, from, last time.Time
	firstOK, ok       time.Time
	mendOK            time.Time
	showed            bool
}

// quiet reports whether failingClear passed by now since the latest
// error of the streak, and GitHub answered well since, as healthy, when
// it last answered any request well, says.
func (s *failStreak) quiet(now, healthy time.Time) bool {
	return healthy.After(s.last) && !now.Before(s.last.Add(failingClear))
}

// mended reports whether the streak ended by now with its own resource
// answering well: it is quiet, and the resource answered well since its
// latest error.
func (s *failStreak) mended(now, healthy time.Time) bool {
	return s.ok.After(s.last) && s.quiet(now, healthy)
}

// lapsed reports whether the streak is quiet at now only because other
// resources answered well, as for a search that failed and isn't run
// again, or GraphQL failing at each of its polls while REST answers in
// between. It hides from the status, but nothing failed mended: the
// streak is kept, and an error of its resource within failingResume
// takes it up again.
func (s *failStreak) lapsed(now, healthy time.Time) bool {
	return !s.ok.After(s.last) && s.quiet(now, healthy)
}

// shown reports whether the streak tells of an outage by now, if it is
// neither mended nor lapsed: the errors since from kept coming for
// failingAfter, or no good answer of its resource came in the
// failingAfter since from. So a blip that takes up a streak doesn't show
// any more than one that starts it.
func (s *failStreak) shown(now time.Time) bool {
	if s.last.Sub(s.from) >= failingAfter {
		return true
	}
	noAnswer := s.firstOK.IsZero() || s.firstOK.Sub(s.from) >= failingAfter
	return noAnswer && now.Sub(s.from) >= failingAfter
}

// next returns when the streak shows or goes quiet with no answer, after
// now, given healthy as quiet takes it, or zero if neither is coming.
func (s *failStreak) next(now, healthy time.Time) time.Time {
	if s.firstOK.IsZero() {
		if t := s.from.Add(failingAfter); t.After(now) {
			return t
		}
	}
	if healthy.After(s.last) {
		if t := s.last.Add(failingClear); t.After(now) {
			return t
		}
	}
	return time.Time{}
}

// takeUp counts an error at now in the streak that lapsed: it shows
// again by the same rules as a new one, from now, and keeps telling
// since when it began.
func (s *failStreak) takeUp(now time.Time) {
	s.showed = s.showed || s.shown(s.last.Add(failingClear))
	s.from, s.firstOK = now, time.Time{}
}

// noteAnswer counts an answer to a request of resource: a server error,
// whether GitHub or a proxy on the way sent it, starts the resource's
// streak, or takes it up again if it lapsed after showing within
// failingResume, and GitHub answering otherwise counts toward ending it,
// and every other streak's showing. Streaks are kept per resource, so
// that GraphQL failing while REST answers well still fails. b.mu must be
// held.
func (b *budget) noteAnswer(resource string, serverError, github bool, now time.Time) {
	b.dropMended(now)
	switch {
	case serverError:
		s := b.failing[resource]
		switch {
		case s == nil:
			s = &failStreak{start: now, from: now}
		case s.lapsed(now, b.healthy):
			s.takeUp(now)
			if !s.showed || now.Sub(s.last) > failingResume {
				s = &failStreak{start: now, from: now}
			}
		}
		b.failing[resource] = s
		s.last = now
	case github:
		b.healthy = now
		if s := b.failing[resource]; s != nil {
			if s.firstOK.IsZero() {
				s.firstOK = now
			}
			if !s.ok.After(s.last) {
				s.mendOK = now
			}
			s.ok = now
			b.dropMended(now)
		}
	}
}

// dropMended forgets the streaks that mended by now, and notes when the
// resource of one that showed first answered well again. b.mu must be
// held.
func (b *budget) dropMended(now time.Time) {
	for resource, s := range b.failing {
		if !s.mended(now, b.healthy) {
			continue
		}
		if s.shown(now) && s.mendOK.After(b.mended) {
			b.mended = s.mendOK
		}
		delete(b.failing, resource)
	}
}

// failingSince returns since when GitHub fails at now: the start of the
// earliest streak shown, or zero if none is. b.mu must be held.
func (b *budget) failingSince(now time.Time) time.Time {
	b.dropMended(now)
	var since time.Time
	for _, s := range b.failing {
		if !s.lapsed(now, b.healthy) && s.shown(now) && (since.IsZero() || s.start.Before(since)) {
			since = s.start
		}
	}
	return since
}
