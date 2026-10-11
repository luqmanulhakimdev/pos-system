package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/luqmanulhakimdev/pos-system/internal/application"
	"golang.org/x/crypto/bcrypt"
)

type AuthStore struct {
	pool *pgxpool.Pool
}

func NewAuthStore(pool *pgxpool.Pool) *AuthStore {
	return &AuthStore{pool: pool}
}

func (s *AuthStore) VerifyCredentials(ctx context.Context, email, password string) (int64, error) {
	var userID int64
	var passwordHash string
	var active bool
	err := s.pool.QueryRow(ctx, "SELECT id, password_hash, active FROM users WHERE lower(email) = $1", email).Scan(&userID, &passwordHash, &active)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, application.ErrInvalidCredentials
	}
	if err != nil {
		return 0, fmt.Errorf("load user credentials: %w", err)
	}
	if !active || bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(password)) != nil {
		return 0, application.ErrInvalidCredentials
	}
	return userID, nil
}

func (s *AuthStore) CreateSession(ctx context.Context, userID int64, tokenHash string, expiresAt time.Time) error {
	if _, err := s.pool.Exec(ctx, "INSERT INTO user_sessions (user_id, token_hash, expires_at) VALUES ($1, $2, $3)", userID, tokenHash, expiresAt); err != nil {
		return fmt.Errorf("persist session: %w", err)
	}
	return nil
}

func (s *AuthStore) LoadSession(ctx context.Context, tokenHash string) (application.AuthenticatedUser, error) {
	var user application.AuthenticatedUser
	err := s.pool.QueryRow(ctx, `SELECT u.id, u.email FROM user_sessions s JOIN users u ON u.id = s.user_id
		WHERE s.token_hash = $1 AND s.revoked_at IS NULL AND s.expires_at > now() AND u.active = TRUE`, tokenHash).Scan(&user.ID, &user.Email)
	if errors.Is(err, pgx.ErrNoRows) {
		return application.AuthenticatedUser{}, application.ErrInvalidSession
	}
	if err != nil {
		return application.AuthenticatedUser{}, fmt.Errorf("load session: %w", err)
	}
	rows, err := s.pool.Query(ctx, `SELECT DISTINCT p.name FROM user_roles ur
		JOIN role_permissions rp ON rp.role_id = ur.role_id
		JOIN permissions p ON p.id = rp.permission_id
		WHERE ur.user_id = $1 ORDER BY p.name`, user.ID)
	if err != nil {
		return application.AuthenticatedUser{}, fmt.Errorf("load user permissions: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var permission string
		if err := rows.Scan(&permission); err != nil {
			return application.AuthenticatedUser{}, fmt.Errorf("scan user permission: %w", err)
		}
		user.Permissions = append(user.Permissions, permission)
	}
	if err := rows.Err(); err != nil {
		return application.AuthenticatedUser{}, fmt.Errorf("read user permissions: %w", err)
	}
	return user, nil
}

func (s *AuthStore) RevokeSession(ctx context.Context, tokenHash string) error {
	tag, err := s.pool.Exec(ctx, `UPDATE user_sessions SET revoked_at = now()
		WHERE token_hash = $1 AND revoked_at IS NULL AND expires_at > now()`, tokenHash)
	if err != nil {
		return fmt.Errorf("revoke session: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return application.ErrInvalidSession
	}
	return nil
}

func HashPassword(password string) ([]byte, error) {
	if len(password) < 12 || len(password) > 72 {
		return nil, fmt.Errorf("password must be between 12 and 72 bytes")
	}
	return bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
}

var _ application.AuthStore = (*AuthStore)(nil)
