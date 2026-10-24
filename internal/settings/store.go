package settings

import "sync"

// Store is the process-wide owner of the persisted preferences. Callers
// mutate fields through Update so a stale copy cannot overwrite unrelated
// changes made by another part of the app.
type Store struct {
	mu      sync.Mutex
	dir     string
	current Settings
}

// NewStore returns a store containing initial. The caller is responsible for
// loading initial from disk before constructing the store.
func NewStore(dir string, initial Settings) *Store {
	return &Store{dir: dir, current: initial}
}

// Snapshot returns the current settings. Settings contains only value fields,
// so the returned copy cannot mutate the store.
func (s *Store) Snapshot() Settings {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.current
}

// Update applies change to a copy, persists it atomically, and only then
// publishes it. A failed save leaves the in-memory settings unchanged.
func (s *Store) Update(change func(*Settings)) (Settings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if change == nil {
		return s.current, nil
	}
	next := s.current
	change(&next)
	if err := Save(s.dir, next); err != nil {
		return s.current, err
	}
	s.current = next
	return next, nil
}
