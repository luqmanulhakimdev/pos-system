package application

import (
	"context"
	"testing"
	"time"
)

type memoryAuthStore struct {
	storedHash string
	user       AuthenticatedUser
	revoked    bool
}

func (s *memoryAuthStore) VerifyCredentials(_ context.Context, email, password string) (int64, error) {
	if email != "admin@example.com" || password != "a-strong-password" {
		return 0, ErrInvalidCredentials
	}
	return 7, nil
}
func (s *memoryAuthStore) CreateSession(_ context.Context, _ int64, tokenHash string, _ time.Time) error {
	s.storedHash = tokenHash
	return nil
}
func (s *memoryAuthStore) LoadSession(_ context.Context, tokenHash string) (AuthenticatedUser, error) {
	if tokenHash != s.storedHash || s.revoked {
		return AuthenticatedUser{}, ErrInvalidSession
	}
	return s.user, nil
}
func (s *memoryAuthStore) RevokeSession(_ context.Context, tokenHash string) error {
	if tokenHash != s.storedHash || s.revoked {
		return ErrInvalidSession
	}
	s.revoked = true
	return nil
}

func TestLoginStoresOnlyHashAndLogoutRevokesSession(t *testing.T) {
	store := &memoryAuthStore{user: AuthenticatedUser{ID: 7, Email: "admin@example.com", Permissions: []string{"orders.read"}}}
	service := NewAuthService(store, time.Hour)
	session, err := service.Login(context.Background(), "admin@example.com", "a-strong-password")
	if err != nil {
		t.Fatal(err)
	}
	if session.TokenType != "Bearer" || len(session.Token) != 43 || store.storedHash == session.Token {
		t.Fatalf("unexpected token/session storage: %#v", session)
	}
	user, err := service.Authenticate(context.Background(), session.Token)
	if err != nil || !HasPermission(user, "orders.read") {
		t.Fatalf("Authenticate() = (%#v, %v), want authorized user", user, err)
	}
	if err := service.Logout(context.Background(), session.Token); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Authenticate(context.Background(), session.Token); err != ErrInvalidSession {
		t.Fatalf("Authenticate() after logout error = %v, want ErrInvalidSession", err)
	}
}

func TestLoginRejectsInvalidCredentialsAndMalformedToken(t *testing.T) {
	service := NewAuthService(&memoryAuthStore{}, time.Hour)
	if _, err := service.Login(context.Background(), "admin@example.com", "wrong"); err != ErrInvalidCredentials {
		t.Fatalf("Login() error = %v, want ErrInvalidCredentials", err)
	}
	if _, err := service.Authenticate(context.Background(), "not-a-token"); err != ErrInvalidSession {
		t.Fatalf("Authenticate() error = %v, want ErrInvalidSession", err)
	}
}
