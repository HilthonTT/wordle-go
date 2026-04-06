package store

import (
	"fmt"
	"sync"
	"time"

	"github.com/hilthontt/wordle-go/internal/game"
)

// Session wraps a game instance with metadata.
type Session struct {
	ID          string
	Game        game.Wordle
	CreatedAt   time.Time
	UpdatedAt   time.Time
	WordLength  int
	MaxAttempts int
}

// Store is a thread-safe, in-memory game session registry.
type Store struct {
	mu       sync.RWMutex
	sessions map[string]*Session
}

// New returns an empty Store.
func New() *Store {
	return &Store{sessions: make(map[string]*Session)}
}

// Create stores a new session and returns it.
func (s *Store) Create(id string, g game.Wordle, wordLength, maxAttempts int) *Session {
	now := time.Now()
	sess := &Session{
		ID:          id,
		Game:        g,
		CreatedAt:   now,
		UpdatedAt:   now,
		WordLength:  wordLength,
		MaxAttempts: maxAttempts,
	}
	s.mu.Lock()
	s.sessions[id] = sess
	s.mu.Unlock()
	return sess
}

// Get retrieves a session by ID.
func (s *Store) Get(id string) (*Session, error) {
	s.mu.RLock()
	sess, ok := s.sessions[id]
	s.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("game %q not found", id)
	}
	return sess, nil
}

// Touch updates the UpdatedAt timestamp.
func (s *Store) Touch(id string) {
	s.mu.Lock()
	if sess, ok := s.sessions[id]; ok {
		sess.UpdatedAt = time.Now()
	}
	s.mu.Unlock()
}

// Delete removes a session.
func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.sessions[id]; !ok {
		return fmt.Errorf("game %q not found", id)
	}
	delete(s.sessions, id)
	return nil
}

// List returns a snapshot of all active sessions.
func (s *Store) List() []*Session {
	s.mu.RLock()
	out := make([]*Session, 0, len(s.sessions))
	for _, sess := range s.sessions {
		out = append(out, sess)
	}
	s.mu.RUnlock()
	return out
}
