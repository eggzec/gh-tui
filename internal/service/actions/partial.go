package actions

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"sync"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/github"
)

// Each read of a partial log costs a request of the rate limit, for the
// redirect to its storage, unless the client kept the signed URL; and
// GitHub publishes the log in blocks of about 2 MiB, minutes apart. So a
// poll reads a log again only once partialWaits[n] passed since the last
// read, after n reads in a row found nothing new, less partialSlack, as
// the polls come about 10 seconds apart; and never within partialFresh,
// such as right after the view that started to watch it read it.
var partialWaits = [...]time.Duration{0, 10 * time.Second, 20 * time.Second, 40 * time.Second, time.Minute}

const (
	partialSlack = 5 * time.Second
	partialFresh = 5 * time.Second
)

// partialLimit is how much of a partial log is kept at most: its end,
// which the next block of the log follows.
const partialLimit = 2 << 20

// partialOverlap is how many bytes of the end of what was read a read
// asks for again, to tell that the log goes on from them rather than
// started over.
const partialOverlap = 256

// partialLog is what was read of the log of a job in progress, while views
// watch it.
type partialLog struct {
	repo         core.RepoRef
	runID, jobID int64
	// views counts the views that watch it, and log is what they show,
	// both under the service's partialMu.
	views int
	log   core.PartialLog

	// fetch is held while the log is read, so that reads take turns. What
	// follows is only used under it.
	fetch  sync.Mutex
	parser *core.LogParser
	// lines are the lines read, from byte start of the log, and sizes the
	// bytes of each; next is the byte after them, and tail their last
	// bytes, up to partialOverlap.
	lines       []core.LogLine
	sizes       []int
	kept        int
	start, next int64
	tail        []byte
	gen         int
	seen        bool
	// readAt is when it was last read, and idle counts the reads in a row
	// that found nothing new.
	readAt time.Time
	idle   int
}

// due is how long a poll leaves the log after the last read.
func (p *partialLog) due() time.Duration {
	return max(partialWaits[min(p.idle, len(partialWaits)-1)]-partialSlack, partialFresh)
}

// WatchLog has the poll of run runID of repo read what GitHub adds to the
// log of its job jobID while the job runs, until stop is called: while a
// view shows it. Watches of the same job share what was read.
func (s *Service) WatchLog(repo core.RepoRef, runID, jobID int64) (stop func()) {
	key := logKey(repo, jobID)
	s.partialMu.Lock()
	defer s.partialMu.Unlock()
	p := s.partials[key]
	if p == nil {
		p = &partialLog{repo: repo, runID: runID, jobID: jobID}
		s.partials[key] = p
	}
	p.views++
	var once sync.Once
	return func() {
		once.Do(func() {
			s.partialMu.Lock()
			defer s.partialMu.Unlock()
			p.views--
			if p.views == 0 && s.partials[key] == p {
				delete(s.partials, key)
			}
		})
	}
}

// CachedPartialLog returns what was read of the log of job jobID of repo
// in progress while it is watched, without a request. It reports false if
// nothing was read.
func (s *Service) CachedPartialLog(repo core.RepoRef, jobID int64) (core.PartialLog, bool) {
	s.partialMu.Lock()
	defer s.partialMu.Unlock()
	p := s.partials[logKey(repo, jobID)]
	if p == nil || p.log.At.IsZero() {
		return core.PartialLog{}, false
	}
	return p.log, true
}

// PartialLog returns the log of job jobID of repo in progress, as far as
// GitHub publishes it, which is often minutes behind the job: its last
// 2 MiB at most. Of a watched job, it reads only what was added since the
// last read, and keeps it. It is never kept as the log: the job's whole
// log comes with Log once it completed.
//
// Until GitHub publishes the first block, it fails with
// core.ErrLogPending.
func (s *Service) PartialLog(ctx context.Context, repo core.RepoRef, jobID int64) (core.PartialLog, error) {
	s.partialMu.Lock()
	p := s.partials[logKey(repo, jobID)]
	s.partialMu.Unlock()
	if p == nil {
		p = &partialLog{repo: repo, jobID: jobID}
	}
	l, _, err := s.readPartial(ctx, p, false)
	if err != nil {
		return core.PartialLog{}, fmt.Errorf("partial log of job %d of %s: %w", p.jobID, repo, err)
	}
	return l, nil
}

// pollLogs reads what was added to the watched logs of the jobs of run
// runID of repo in progress, as often as they are due, and reports whether
// any changed. A log that fails to read waits for a later poll, as it is
// only a preview.
func (s *Service) pollLogs(ctx context.Context, repo core.RepoRef, runID int64) bool {
	s.partialMu.Lock()
	var watched []*partialLog
	for _, p := range s.partials {
		if p.runID == runID && repoID(p.repo) == repoID(repo) {
			watched = append(watched, p)
		}
	}
	s.partialMu.Unlock()
	changed := false
	for _, p := range watched {
		// A job that waits for a runner has no log yet.
		if j, ok := s.cachedJob(repo, p.jobID); !ok || j.Status != core.RunInProgress {
			continue
		}
		_, ch, err := s.readPartial(ctx, p, true)
		if err != nil && !errors.Is(err, core.ErrLogPending) {
			slog.WarnContext(ctx, "read partial log", "span", "service.actions", "job", p.jobID, "err", err.Error())
		}
		changed = changed || ch
	}
	return changed
}

// readPartial reads what was added to p since the last read, unless a
// poll reads it before it is due, and reports whether what its views show
// changed: its lines, or the minute it was read at.
func (s *Service) readPartial(ctx context.Context, p *partialLog, poll bool) (core.PartialLog, bool, error) {
	p.fetch.Lock()
	defer p.fetch.Unlock()
	now := s.now()
	if poll && now.Sub(p.readAt) < p.due() {
		return core.PartialLog{}, false, nil
	}
	p.readAt = now
	limit := min(s.logLimit, partialLimit)
	from := p.next - int64(len(p.tail))
	part, err := s.api.JobLogFrom(ctx, p.repo, p.jobID, from, limit)
	if errors.Is(err, core.ErrNotFound) {
		// GitHub has nothing of the log of a job in progress yet.
		err = fmt.Errorf("%w: %w", core.ErrLogPending, err)
	}
	if err != nil {
		p.idle++
		return core.PartialLog{}, false, err
	}

	added, goesOn := p.after(part)
	if !goesOn {
		job, _ := s.cachedJob(p.repo, p.jobID)
		p.parser, p.lines, p.sizes, p.kept = core.NewLogParser(job.Steps), nil, nil, 0
		p.start, p.next, p.tail = part.Start, part.Start, nil
		p.gen++
		added = part.Text
	}
	p.seen = true
	// Only whole lines are kept: the rest is read again with what follows.
	whole := added[:bytes.LastIndexByte(added, '\n')+1]
	if goesOn && len(whole) == 0 {
		p.idle++
		return s.checked(p, now)
	}
	p.idle = 0
	p.add(whole, s.stepsOf(p), limit)
	l := core.PartialLog{Lines: p.lines, Truncated: p.start > 0, At: now, Gen: p.gen}
	s.partialMu.Lock()
	p.log = l
	s.partialMu.Unlock()
	return l, true, nil
}

// after returns what part adds to what was read of p, and reports whether
// it goes on from it: whether it holds the tail of what was read, where it
// was read, rather than a log that started over.
func (p *partialLog) after(part github.LogPart) ([]byte, bool) {
	if !p.seen {
		return nil, false
	}
	at := p.next - int64(len(p.tail)) - part.Start
	end := at + int64(len(p.tail))
	if at < 0 || end > int64(len(part.Text)) || !bytes.Equal(part.Text[at:end], p.tail) {
		return nil, false
	}
	return part.Text[end:], true
}

// add parses whole, the whole lines that follow those read, and keeps the
// last limit bytes of them at most.
func (p *partialLog) add(whole []byte, steps []core.Step, limit int64) {
	p.lines = append(p.lines, p.parser.Parse(string(whole), steps)...)
	for rest := whole; len(rest) > 0; {
		n := bytes.IndexByte(rest, '\n') + 1
		p.sizes = append(p.sizes, n)
		rest = rest[n:]
	}
	p.kept += len(whole)
	p.next += int64(len(whole))
	p.tail = bytes.Clone(append(p.tail, whole[max(len(whole)-partialOverlap, 0):]...))
	p.tail = p.tail[max(len(p.tail)-partialOverlap, 0):]
	if int64(p.kept) <= limit {
		return
	}
	// Drop the first lines, into new slices, which the views don't share
	// and which free what was dropped. The lines move, so the views show
	// them anew: down to 3/4 of the limit, so that the blocks after this
	// one don't each move them again.
	drop, n := 0, 0
	for int64(p.kept-drop) > limit*3/4 {
		drop += p.sizes[n]
		n++
	}
	p.lines, p.sizes = slices.Clone(p.lines[n:]), slices.Clone(p.sizes[n:])
	p.kept -= drop
	p.start += int64(drop)
	p.gen++
}

// stepsOf returns the steps of the job of p as the cache has them now.
func (s *Service) stepsOf(p *partialLog) []core.Step {
	job, _ := s.cachedJob(p.repo, p.jobID)
	return job.Steps
}

// checked marks p read at now, with nothing new, and reports a change
// only when the minute it shows moves on, rather than at every read.
func (s *Service) checked(p *partialLog, now time.Time) (core.PartialLog, bool, error) {
	s.partialMu.Lock()
	defer s.partialMu.Unlock()
	moved := !p.log.At.IsZero() && !p.log.At.Truncate(time.Minute).Equal(now.Truncate(time.Minute))
	if !p.log.At.IsZero() {
		p.log.At = now
	}
	return p.log, moved, nil
}
