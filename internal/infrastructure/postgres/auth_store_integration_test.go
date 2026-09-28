package postgres

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/luqmanulhakimdev/pos-system/internal/application"
)

func TestAuthStoreIntegration(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := NewPool(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err := ApplyMigrations(ctx, pool); err != nil {
		t.Fatal(err)
	}

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	email := suffix + "@example.test"
	hash, err := HashPassword("integration-password")
	if err != nil {
		t.Fatal(err)
	}
	var userID, roleID int64
	if err := pool.QueryRow(ctx, `INSERT INTO users (email,password_hash,display_name) VALUES ($1,$2,'Auth test') RETURNING id`, email, string(hash)).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), "DELETE FROM users WHERE id = $1", userID) })
	if err := pool.QueryRow(ctx, "SELECT id FROM roles WHERE name = 'admin'").Scan(&roleID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "INSERT INTO user_roles (user_id, role_id) VALUES ($1, $2)", userID, roleID); err != nil {
		t.Fatal(err)
	}

	store := NewAuthStore(pool)
	verifiedID, err := store.VerifyCredentials(ctx, email, "integration-password")
	if err != nil || verifiedID != userID {
		t.Fatalf("VerifyCredentials() = (%d, %v), want user %d", verifiedID, err, userID)
	}
	if _, err := store.VerifyCredentials(ctx, email, "wrong-password"); err != application.ErrInvalidCredentials {
		t.Fatalf("wrong password error = %v", err)
	}
	expires := time.Now().Add(time.Minute)
	if err := store.CreateSession(ctx, userID, "integration-session-hash", expires); err != nil {
		t.Fatal(err)
	}
	user, err := store.LoadSession(ctx, "integration-session-hash")
	if err != nil || user.ID != userID || !application.HasPermission(user, "user.manage") {
		t.Fatalf("LoadSession() = (%#v, %v)", user, err)
	}
	if err := store.RevokeSession(ctx, "integration-session-hash"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.LoadSession(ctx, "integration-session-hash"); err != application.ErrInvalidSession {
		t.Fatalf("revoked session error = %v", err)
	}
}
