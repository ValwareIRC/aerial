// Package audit records who read or changed what, and why. Append-only.
package audit

import (
	"sync"
	"time"
)

type Role string

const (
	RoleService     Role = "SERVICE"
	RoleEnforcement Role = "ENFORCEMENT"
	RoleAdmin       Role = "ADMIN"
	RoleSubject     Role = "SUBJECT"
)

type Actor struct {
	ID   string `json:"id"`
	Role Role   `json:"role"`
}

type Entry struct {
	At      time.Time `json:"at"`
	Actor   Actor     `json:"actor"`
	Action  string    `json:"action"`
	Subject string    `json:"subject"`
	Purpose string    `json:"purpose"`
	Allowed bool      `json:"allowed"`
	Detail  string    `json:"detail,omitempty"`
}

type Log struct {
	mu      sync.Mutex
	now     func() time.Time
	entries []Entry
}

// New takes a clock so tests are deterministic. Pass nil for time.Now.
func New(now func() time.Time) *Log {
	if now == nil {
		now = time.Now
	}
	return &Log{now: now}
}

func (l *Log) Record(e Entry) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if e.At.IsZero() {
		e.At = l.now().UTC()
	}
	l.entries = append(l.entries, e)
}

func (l *Log) Entries() []Entry {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]Entry, len(l.entries))
	copy(out, l.entries)
	return out
}

func (l *Log) For(subject string) []Entry {
	l.mu.Lock()
	defer l.mu.Unlock()
	var out []Entry
	for _, e := range l.entries {
		if e.Subject == subject {
			out = append(out, e)
		}
	}
	return out
}

func (l *Log) ByRole(role Role) []Entry {
	l.mu.Lock()
	defer l.mu.Unlock()
	var out []Entry
	for _, e := range l.entries {
		if e.Actor.Role == role {
			out = append(out, e)
		}
	}
	return out
}

func (l *Log) Len() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.entries)
}
