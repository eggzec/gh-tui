package tray

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"slices"
	"sync"
	"time"
)

// Put adds d to the drafts under key, or replaces the draft that has its ID.
// A draft with no ID gets one, and one with no time is stamped now. It returns
// the draft as kept. A damaged file is replaced, as Save may.
func (s *Store) Put(key string, d Draft) (Draft, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	drafts, err := s.load(key)
	if err != nil && !errors.Is(err, ErrDamaged) {
		return Draft{}, err
	}
	d, drafts = put(d, drafts)
	return d, s.save(key, drafts)
}

// Remove deletes the draft with id from those under key. A draft that is not
// there is no error.
func (s *Store) Remove(key, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	drafts, err := s.load(key)
	if err != nil {
		return err
	}
	return s.save(key, without(drafts, id))
}

// put returns d, stamped, and drafts with d added or put in place of the
// draft of its ID. It never changes drafts itself.
func put(d Draft, drafts []Draft) (stamped Draft, all []Draft) {
	if d.ID == "" {
		d.ID = newID()
	}
	if d.Created.IsZero() {
		d.Created = time.Now()
	}
	drafts = slices.Clone(drafts)
	if i := slices.IndexFunc(drafts, func(x Draft) bool { return x.ID == d.ID }); i >= 0 {
		drafts[i] = d
	} else {
		drafts = append(drafts, d)
	}
	return d, drafts
}

// without returns drafts but the one with id.
func without(drafts []Draft, id string) []Draft {
	return slices.DeleteFunc(slices.Clone(drafts), func(x Draft) bool { return x.ID == id })
}

// newID returns a random name for a draft.
func newID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// Memory is drafts kept in memory only, for a session with nowhere to keep
// them on disk. It has the calls of a Store that the interface needs.
type Memory struct {
	mu sync.Mutex
	by map[string][]Draft
}

// Load returns the drafts under key.
func (m *Memory) Load(key string) ([]Draft, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return slices.Clone(m.by[key]), nil
}

// Put adds or replaces a draft, as [Store.Put] does.
func (m *Memory) Put(key string, d Draft) (Draft, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.by == nil {
		m.by = map[string][]Draft{}
	}
	d, m.by[key] = put(d, m.by[key])
	return d, nil
}

// Remove deletes the draft with id.
func (m *Memory) Remove(key, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.by == nil {
		return nil
	}
	m.by[key] = without(m.by[key], id)
	return nil
}
