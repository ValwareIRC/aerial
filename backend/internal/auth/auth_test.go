package auth

import (
	"errors"
	"testing"
	"time"

	"github.com/aerial-project/aerial/internal/audit"
)

var clock = func() time.Time { return time.Date(2026, time.August, 28, 9, 0, 0, 0, time.UTC) }

func newStoreWithUser(t *testing.T) *Store {
	t.Helper()
	s := NewStore(clock)
	if _, err := s.AddUser("j.okafor", "J. Okafor", audit.RoleEnforcement, "correct-horse-battery"); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestAuthenticateOpensASession(t *testing.T) {
	s := newStoreWithUser(t)
	sess, err := s.Authenticate("j.okafor", "correct-horse-battery")
	if err != nil {
		t.Fatal(err)
	}
	if sess.Role != audit.RoleEnforcement {
		t.Errorf("role = %s, want ENFORCEMENT", sess.Role)
	}
	if sess.Token == "" {
		t.Error("session has no token")
	}
	if got, err := s.Lookup(sess.Token); err != nil || got.Username != "j.okafor" {
		t.Errorf("lookup failed: %v", err)
	}
}

func TestRoleComesFromTheRecordNotTheRequest(t *testing.T) {
	s := newStoreWithUser(t)
	sess, err := s.Authenticate("j.okafor", "correct-horse-battery")
	if err != nil {
		t.Fatal(err)
	}
	if sess.Actor().Role != audit.RoleEnforcement {
		t.Error("the actor role must come from the stored user")
	}
}

func TestBadCredentialsAreRejected(t *testing.T) {
	s := newStoreWithUser(t)
	if _, err := s.Authenticate("j.okafor", "wrong-password-here"); !errors.Is(err, ErrBadCredentials) {
		t.Errorf("wrong password = %v, want ErrBadCredentials", err)
	}
	if _, err := s.Authenticate("nobody", "correct-horse-battery"); !errors.Is(err, ErrBadCredentials) {
		t.Errorf("unknown user = %v, want ErrBadCredentials", err)
	}
}

func TestUsernameIsCaseInsensitive(t *testing.T) {
	s := newStoreWithUser(t)
	if _, err := s.Authenticate("J.Okafor", "correct-horse-battery"); err != nil {
		t.Errorf("username should be case insensitive: %v", err)
	}
}

func TestSessionExpires(t *testing.T) {
	now := clock()
	s := NewStore(func() time.Time { return now })
	if _, err := s.AddUser("a.user", "A. User", audit.RoleService, "a-long-enough-password"); err != nil {
		t.Fatal(err)
	}
	sess, err := s.Authenticate("a.user", "a-long-enough-password")
	if err != nil {
		t.Fatal(err)
	}

	now = now.Add(SessionTTL + time.Minute)
	if _, err := s.Lookup(sess.Token); !errors.Is(err, ErrNoSession) {
		t.Errorf("expired session = %v, want ErrNoSession", err)
	}
}

func TestRevokeEndsTheSession(t *testing.T) {
	s := newStoreWithUser(t)
	sess, err := s.Authenticate("j.okafor", "correct-horse-battery")
	if err != nil {
		t.Fatal(err)
	}
	s.Revoke(sess.Token)
	if _, err := s.Lookup(sess.Token); !errors.Is(err, ErrNoSession) {
		t.Errorf("revoked session = %v, want ErrNoSession", err)
	}
}

func TestUnknownTokenIsRejected(t *testing.T) {
	s := newStoreWithUser(t)
	for _, token := range []string{"", "nonsense", "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"} {
		if _, err := s.Lookup(token); !errors.Is(err, ErrNoSession) {
			t.Errorf("token %q = %v, want ErrNoSession", token, err)
		}
	}
}

func TestAddUserValidation(t *testing.T) {
	s := NewStore(clock)
	if _, err := s.AddUser("short", "Short", audit.RoleService, "tooshort"); !errors.Is(err, ErrWeakPassword) {
		t.Errorf("short password = %v, want ErrWeakPassword", err)
	}
	if _, err := s.AddUser("", "No name", audit.RoleService, "a-long-enough-password"); err == nil {
		t.Error("empty username should be rejected")
	}
	if _, err := s.AddUser("x.user", "X", "SUPERVISOR", "a-long-enough-password"); err == nil {
		t.Error("unknown role should be rejected")
	}
	if _, err := s.AddUser("dupe", "Dupe", audit.RoleService, "a-long-enough-password"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddUser("dupe", "Dupe", audit.RoleService, "a-long-enough-password"); !errors.Is(err, ErrDuplicateUser) {
		t.Errorf("duplicate username = %v, want ErrDuplicateUser", err)
	}
}

func TestPasswordIsNotRecoverableFromTheStore(t *testing.T) {
	s := newStoreWithUser(t)
	s.mu.RLock()
	u := s.users["j.okafor"]
	s.mu.RUnlock()

	if string(u.hash) == "correct-horse-battery" {
		t.Fatal("password stored in clear")
	}
	if len(u.salt) != saltLength || len(u.hash) != keyLength {
		t.Errorf("unexpected salt/hash lengths %d/%d", len(u.salt), len(u.hash))
	}
}

func TestRepeatedFailuresLockTheAccount(t *testing.T) {
	now := clock()
	s := NewStore(func() time.Time { return now })
	if _, err := s.AddUser("j.okafor", "J. Okafor", audit.RoleEnforcement, "correct-horse-battery"); err != nil {
		t.Fatal(err)
	}

	for i := 0; i < MaxFailedAttempts; i++ {
		if _, err := s.Authenticate("j.okafor", "wrong-password-here"); !errors.Is(err, ErrBadCredentials) {
			t.Fatalf("attempt %d = %v, want ErrBadCredentials", i+1, err)
		}
	}

	// The right password is now refused too: guessing has to stop, not merely
	// slow down.
	if _, err := s.Authenticate("j.okafor", "correct-horse-battery"); !errors.Is(err, ErrTooManyAttempts) {
		t.Errorf("after %d failures = %v, want ErrTooManyAttempts", MaxFailedAttempts, err)
	}

	now = now.Add(LockoutFor + time.Minute)
	if _, err := s.Authenticate("j.okafor", "correct-horse-battery"); err != nil {
		t.Errorf("after the lockout expires: %v", err)
	}
}

func TestLockoutDoesNotDiscloseWhetherAnAccountExists(t *testing.T) {
	s := newStoreWithUser(t)
	for i := 0; i < MaxFailedAttempts; i++ {
		if _, err := s.Authenticate("nobody.here", "wrong-password-here"); !errors.Is(err, ErrBadCredentials) {
			t.Fatalf("attempt %d = %v, want ErrBadCredentials", i+1, err)
		}
	}
	if _, err := s.Authenticate("nobody.here", "wrong-password-here"); !errors.Is(err, ErrTooManyAttempts) {
		t.Errorf("an unknown username should lock out the same way, got %v", err)
	}
}

func TestOneGoodSignInClearsTheFailureCount(t *testing.T) {
	s := newStoreWithUser(t)
	for i := 0; i < MaxFailedAttempts-1; i++ {
		_, _ = s.Authenticate("j.okafor", "wrong-password-here")
	}
	if _, err := s.Authenticate("j.okafor", "correct-horse-battery"); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < MaxFailedAttempts-1; i++ {
		if _, err := s.Authenticate("j.okafor", "wrong-password-here"); !errors.Is(err, ErrBadCredentials) {
			t.Fatalf("the count should have restarted, got %v", err)
		}
	}
}

func TestExpiredSessionsAreSweptAway(t *testing.T) {
	now := clock()
	s := NewStore(func() time.Time { return now })
	if _, err := s.AddUser("j.okafor", "J. Okafor", audit.RoleEnforcement, "correct-horse-battery"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Authenticate("j.okafor", "correct-horse-battery"); err != nil {
		t.Fatal(err)
	}
	if s.SessionCount() != 1 {
		t.Fatalf("sessions = %d, want 1", s.SessionCount())
	}

	now = now.Add(SessionTTL + time.Hour)
	if removed := s.Sweep(); removed != 1 {
		t.Errorf("swept %d sessions, want 1", removed)
	}
	if s.SessionCount() != 0 {
		t.Errorf("sessions = %d after sweeping, want 0", s.SessionCount())
	}
}

func TestRevokeAllForEndsEverySessionOfOneUser(t *testing.T) {
	s := newStoreWithUser(t)
	first, err := s.Authenticate("j.okafor", "correct-horse-battery")
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.Authenticate("j.okafor", "correct-horse-battery")
	if err != nil {
		t.Fatal(err)
	}
	if ended := s.RevokeAllFor(first.UserID); ended != 2 {
		t.Errorf("ended %d sessions, want 2", ended)
	}
	for _, sess := range []*Session{first, second} {
		if _, err := s.Lookup(sess.Token); !errors.Is(err, ErrNoSession) {
			t.Errorf("session should be gone, got %v", err)
		}
	}
}
