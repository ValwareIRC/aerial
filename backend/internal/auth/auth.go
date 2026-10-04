// Package auth holds users and sessions. Roles come from the user record and
// are never accepted from the client.
package auth

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/aerial-project/aerial/internal/audit"
)

var (
	ErrBadCredentials = errors.New("auth: username or password is not recognised")
	ErrNoSession      = errors.New("auth: no valid session")
	ErrDuplicateUser  = errors.New("auth: username already exists")
	ErrWeakPassword   = errors.New("auth: password must be at least 12 characters")
	// ErrTooManyAttempts means the account is temporarily locked after
	// repeated failures. It is returned whether or not the account exists, so
	// the lockout itself does not disclose which usernames are real.
	ErrTooManyAttempts = errors.New("auth: too many failed sign-in attempts; try again later")
)

const (
	iterations     = 210_000
	keyLength      = 32
	saltLength     = 16
	SessionTTL     = 12 * time.Hour
	MinPasswordLen = 12

	// MaxFailedAttempts failures inside FailureWindow lock an account for
	// LockoutFor. A staff console on the open internet is otherwise a
	// password-guessing target with a very short list of usernames.
	MaxFailedAttempts = 5
	FailureWindow     = 15 * time.Minute
	LockoutFor        = 15 * time.Minute
)

type User struct {
	ID          string
	Username    string
	DisplayName string
	Role        audit.Role
	salt        []byte
	hash        []byte
}

type Session struct {
	Token       string     `json:"-"`
	UserID      string     `json:"userId"`
	Username    string     `json:"username"`
	DisplayName string     `json:"displayName"`
	Role        audit.Role `json:"role"`
	Expires     time.Time  `json:"expires"`
}

func (s Session) Actor() audit.Actor {
	return audit.Actor{ID: s.Username, Role: s.Role}
}

// failures counts recent unsuccessful sign-ins for one username.
type failures struct {
	count      int
	windowFrom time.Time
	lockedTill time.Time
}

type Store struct {
	mu       sync.RWMutex
	users    map[string]*User
	sessions map[string]*Session
	failed   map[string]*failures
	now      func() time.Time
	seq      int
}

// NewStore takes a clock so tests are deterministic. Pass nil for time.Now.
func NewStore(now func() time.Time) *Store {
	if now == nil {
		now = time.Now
	}
	return &Store{
		users:    map[string]*User{},
		sessions: map[string]*Session{},
		failed:   map[string]*failures{},
		now:      now,
	}
}

func (s *Store) AddUser(username, displayName string, role audit.Role, password string) (*User, error) {
	username = strings.ToLower(strings.TrimSpace(username))
	if username == "" {
		return nil, fmt.Errorf("auth: username is required")
	}
	if len(password) < MinPasswordLen {
		return nil, ErrWeakPassword
	}
	switch role {
	case audit.RoleService, audit.RoleEnforcement, audit.RoleAdmin, audit.RoleSubject:
	default:
		return nil, fmt.Errorf("auth: unknown role %q", role)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.users[username]; exists {
		return nil, ErrDuplicateUser
	}

	salt := make([]byte, saltLength)
	if _, err := rand.Read(salt); err != nil {
		return nil, err
	}
	hash, err := derive(password, salt)
	if err != nil {
		return nil, err
	}

	s.seq++
	u := &User{
		ID:          fmt.Sprintf("U%d", s.seq),
		Username:    username,
		DisplayName: displayName,
		Role:        role,
		salt:        salt,
		hash:        hash,
	}
	s.users[username] = u
	return u, nil
}

// Authenticate verifies credentials and opens a session. It does the key
// derivation even for an unknown username, and for a locked-out one, so timing
// does not disclose which accounts exist or which are locked.
func (s *Store) Authenticate(username, password string) (*Session, error) {
	username = strings.ToLower(strings.TrimSpace(username))

	s.mu.RLock()
	u, found := s.users[username]
	s.mu.RUnlock()

	locked := s.lockedOut(username)

	salt := u.saltOrDecoy(found)
	candidate, err := derive(password, salt)
	if err != nil {
		return nil, err
	}
	correct := found && subtle.ConstantTimeCompare(candidate, u.hash) == 1

	if locked {
		return nil, ErrTooManyAttempts
	}
	if !correct {
		s.recordFailure(username)
		return nil, ErrBadCredentials
	}
	s.clearFailures(username)
	s.Sweep()

	token, err := newToken()
	if err != nil {
		return nil, err
	}
	sess := &Session{
		Token:       token,
		UserID:      u.ID,
		Username:    u.Username,
		DisplayName: u.DisplayName,
		Role:        u.Role,
		Expires:     s.now().UTC().Add(SessionTTL),
	}

	s.mu.Lock()
	s.sessions[token] = sess
	s.mu.Unlock()
	return sess, nil
}

func (s *Store) Lookup(token string) (*Session, error) {
	if token == "" {
		return nil, ErrNoSession
	}
	s.mu.RLock()
	sess, ok := s.sessions[token]
	s.mu.RUnlock()
	if !ok {
		return nil, ErrNoSession
	}
	if s.now().UTC().After(sess.Expires) {
		s.Revoke(token)
		return nil, ErrNoSession
	}
	return sess, nil
}

func (s *Store) Revoke(token string) {
	s.mu.Lock()
	delete(s.sessions, token)
	s.mu.Unlock()
}

// RevokeAllFor ends every session belonging to a user, for when an account is
// suspended or a password is changed. It returns how many were ended.
func (s *Store) RevokeAllFor(userID string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	ended := 0
	for token, sess := range s.sessions {
		if sess.UserID == userID {
			delete(s.sessions, token)
			ended++
		}
	}
	return ended
}

// Sweep discards sessions that have expired. Without it the table only ever
// grows, since an abandoned session is never signed out.
func (s *Store) Sweep() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now().UTC()
	removed := 0
	for token, sess := range s.sessions {
		if now.After(sess.Expires) {
			delete(s.sessions, token)
			removed++
		}
	}
	for username, f := range s.failed {
		if now.After(f.lockedTill) && now.Sub(f.windowFrom) > FailureWindow {
			delete(s.failed, username)
		}
	}
	return removed
}

func (s *Store) UserCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.users)
}

// SessionCount is the number of sessions currently held, expired ones
// included until the next sweep.
func (s *Store) SessionCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.sessions)
}

func (s *Store) lockedOut(username string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	f, ok := s.failed[username]
	return ok && s.now().UTC().Before(f.lockedTill)
}

func (s *Store) recordFailure(username string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now().UTC()
	f, ok := s.failed[username]
	if !ok || now.Sub(f.windowFrom) > FailureWindow {
		f = &failures{windowFrom: now}
		s.failed[username] = f
	}
	f.count++
	if f.count >= MaxFailedAttempts {
		f.lockedTill = now.Add(LockoutFor)
		f.count = 0
		f.windowFrom = now
	}
}

func (s *Store) clearFailures(username string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.failed, username)
}

var decoySalt = make([]byte, saltLength)

func (u *User) saltOrDecoy(found bool) []byte {
	if !found || u == nil {
		return decoySalt
	}
	return u.salt
}

func derive(password string, salt []byte) ([]byte, error) {
	return pbkdf2.Key(sha256.New, password, salt, iterations, keyLength)
}

func newToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
