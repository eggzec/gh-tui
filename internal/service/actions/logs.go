package actions

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/eggzec/gh-tui/internal/cache"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/github"
)

func logKey(repo core.RepoRef, jobID int64) string {
	return "log:" + repoID(repo) + "/" + strconv.FormatInt(jobID, 10)
}

// CachedLog returns the parsed log of job jobID of repo from memory,
// without a request. It reports false if it isn't there.
func (s *Service) CachedLog(repo core.RepoRef, jobID int64) (core.Log, bool) {
	e, st := s.logs.Get(logKey(repo, jobID))
	return e.Value, st != cache.Miss
}

// Log returns the parsed log of job jobID of repo, with the step of each
// line. A job's log is complete once the job completed, and never changes
// then, so it is read from memory, then from the store, and only then
// from GitHub, and kept for good. The store keeps the text, which the disk
// layer compresses. Memory is bounded by bytes, as logs run to megabytes.
//
// The log of a job that hasn't completed isn't there to read whole: Log
// fails with core.ErrLogPending, without asking for it, and PartialLog
// reads what there is of it. A log larger than the
// limit is read from its end, with Truncated set. One past the retention
// period fails with core.ErrLogExpired.
func (s *Service) Log(ctx context.Context, repo core.RepoRef, jobID int64) (core.Log, error) {
	key := logKey(repo, jobID)
	e, err := s.logs.Fetch(ctx, key, func(ctx context.Context, _ cache.Entry[core.Log], _ bool) (cache.Entry[core.Log], error) {
		if l, ok := s.loadLog(key); ok {
			return cache.Entry[core.Log]{Value: l}, nil
		}
		job, err := s.doneJob(ctx, repo, jobID)
		if err != nil {
			return cache.Entry[core.Log]{}, err
		}
		text, truncated, err := s.api.JobLog(ctx, repo, jobID, s.logLimit)
		if err != nil {
			return cache.Entry[core.Log]{}, err
		}
		s.saveLog(key, text, truncated, job.Steps)
		l := core.ParseLog(string(text), job.Steps)
		l.Truncated = truncated
		return cache.Entry[core.Log]{Value: l}, nil
	})
	if err != nil {
		return core.Log{}, fmt.Errorf("log of job %d of %s: %w", jobID, repo, err)
	}
	return e.Value, nil
}

// doneJob returns job jobID of repo, from the cached jobs if they have it
// completed, or else from GitHub. It fails with core.ErrLogPending if the
// job hasn't completed.
func (s *Service) doneJob(ctx context.Context, repo core.RepoRef, jobID int64) (core.Job, error) {
	job, ok := s.cachedJob(repo, jobID)
	if !ok || !job.Done() {
		var err error
		if job, _, err = s.api.GetJob(ctx, repo, jobID, github.Conditional{}); err != nil {
			return core.Job{}, err
		}
	}
	if !job.Done() {
		return core.Job{}, core.ErrLogPending
	}
	return job, nil
}

// keptLog is what the store keeps of a log before its text: the steps of
// its job, to tell the step of each line again.
type keptLog struct {
	Version   int         `json:"v"`
	Truncated bool        `json:"truncated,omitempty"`
	Steps     []core.Step `json:"steps"`
}

// logVersion is the version of keptLog. Bump it when core.Step changes
// shape, and older logs read as misses.
const logVersion = 1

// logName names the object of the log under key in the store, which takes
// hex keys.
func logName(key string) string {
	h := sha256.Sum256([]byte(key))
	return hex.EncodeToString(h[:])
}

// saveLog keeps text in the store, after a line of what else it needs.
func (s *Service) saveLog(key string, text []byte, truncated bool, steps []core.Step) {
	if s.store == nil {
		return
	}
	head, err := json.Marshal(keptLog{Version: logVersion, Truncated: truncated, Steps: steps})
	if err != nil {
		return
	}
	data := make([]byte, 0, len(head)+1+len(text))
	data = append(append(append(data, head...), '\n'), text...)
	// The store is only a shortcut, so a failure is ignored.
	_ = s.store.Put(kindLog, logName(key), data)
}

// loadLog parses the log that the store keeps under key.
func (s *Service) loadLog(key string) (core.Log, bool) {
	if s.store == nil {
		return core.Log{}, false
	}
	name := logName(key)
	data, ok := s.store.Get(kindLog, name)
	if !ok {
		return core.Log{}, false
	}
	head, text, ok := bytes.Cut(data, []byte("\n"))
	var k keptLog
	if !ok || json.Unmarshal(head, &k) != nil || k.Version != logVersion {
		s.store.Delete(kindLog, name)
		return core.Log{}, false
	}
	l := core.ParseLog(string(text), k.Steps)
	l.Truncated = k.Truncated
	return l, true
}
