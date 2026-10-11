package application

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"
)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrInvalidSession     = errors.New("invalid or expired session")
)

type AuthenticatedUser struct {
	ID          int64
	Email       string
	Permissions []string
}

type LoginSession struct {
	Token     string    `json:"access_token"`
	TokenType string    `json:"token_type"`
	ExpiresAt time.Time `json:"expires_at"`
}

type AuthStore interface {
	VerifyCredentials(context.Context, string, string) (int64, error)
	CreateSession(context.Context, int64, string, time.Time) error
	LoadSession(context.Context, string) (AuthenticatedUser, error)
	RevokeSession(context.Context, string) error
}

type AuthService struct {
	store AuthStore
	ttl   time.Duration
	now   func() time.Time
}

func NewAuthService(store AuthStore, ttl time.Duration) *AuthService {
	if ttl <= 0 {
		ttl = 15 * time.Minute
	}
	return &AuthService{store: store, ttl: ttl, now: time.Now}
}

func (s *AuthService) Login(ctx context.Context, email, password string) (LoginSession, error) {
	if s.store == nil || len(password) == 0 || len(password) > 72 {
		return LoginSession{}, ErrInvalidCredentials
	}
	parsed, err := mail.ParseAddress(strings.TrimSpace(email))
	if err != nil || parsed.Address != strings.TrimSpace(email) {
		return LoginSession{}, ErrInvalidCredentials
	}
	userID, err := s.store.VerifyCredentials(ctx, strings.ToLower(parsed.Address), password)
	if err != nil {
		if errors.Is(err, ErrInvalidCredentials) {
			return LoginSession{}, ErrInvalidCredentials
		}
		return LoginSession{}, err
	}
	token, tokenHash, err := newSessionToken()
	if err != nil {
		return LoginSession{}, fmt.Errorf("generate session token: %w", err)
	}
	expiresAt := s.now().Add(s.ttl).UTC()
	if err := s.store.CreateSession(ctx, userID, tokenHash, expiresAt); err != nil {
		return LoginSession{}, err
	}
	return LoginSession{Token: token, TokenType: "Bearer", ExpiresAt: expiresAt}, nil
}

func (s *AuthService) Authenticate(ctx context.Context, token string) (AuthenticatedUser, error) {
	if s.store == nil {
		return AuthenticatedUser{}, ErrInvalidSession
	}
	hash, err := HashSessionToken(token)
	if err != nil {
		return AuthenticatedUser{}, ErrInvalidSession
	}
	user, err := s.store.LoadSession(ctx, hash)
	if err != nil {
		if errors.Is(err, ErrInvalidSession) {
			return AuthenticatedUser{}, ErrInvalidSession
		}
		return AuthenticatedUser{}, err
	}
	return user, nil
}

func (s *AuthService) Logout(ctx context.Context, token string) error {
	if s.store == nil {
		return ErrInvalidSession
	}
	hash, err := HashSessionToken(token)
	if err != nil {
		return ErrInvalidSession
	}
	return s.store.RevokeSession(ctx, hash)
}

func HasPermission(user AuthenticatedUser, required string) bool {
	for _, permission := range user.Permissions {
		if permission == required {
			return true
		}
	}
	return false
}

func newSessionToken() (string, string, error) {
	var entropy [32]byte
	if _, err := rand.Read(entropy[:]); err != nil {
		return "", "", err
	}
	token := base64.RawURLEncoding.EncodeToString(entropy[:])
	hash, err := HashSessionToken(token)
	return token, hash, err
}

func HashSessionToken(token string) (string, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(decoded) != 32 || len(token) != 43 {
		return "", ErrInvalidSession
	}
	digest := sha256.Sum256([]byte(token))
	return hex.EncodeToString(digest[:]), nil
}
