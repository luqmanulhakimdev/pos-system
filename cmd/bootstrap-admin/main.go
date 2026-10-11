package main

import (
	"context"
	"fmt"
	"log"
	"net/mail"
	"os"
	"strings"
	"time"

	"github.com/luqmanulhakimdev/pos-system/internal/infrastructure/postgres"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	email := strings.ToLower(strings.TrimSpace(os.Getenv("ADMIN_EMAIL")))
	password := os.Getenv("ADMIN_PASSWORD")
	parsed, err := mail.ParseAddress(email)
	if err != nil || parsed.Address != email {
		return fmt.Errorf("ADMIN_EMAIL must be a valid email address")
	}
	passwordHash, err := postgres.HashPassword(password)
	if err != nil {
		return fmt.Errorf("ADMIN_PASSWORD must be 12 to 72 bytes")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	pool, err := postgres.NewPool(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		return err
	}
	defer pool.Close()
	if err := postgres.ApplyMigrations(ctx, pool); err != nil {
		return err
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin admin bootstrap: %w", err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", int64(746219532)); err != nil {
		return fmt.Errorf("lock admin bootstrap: %w", err)
	}
	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (
		SELECT 1 FROM user_roles ur JOIN roles r ON r.id = ur.role_id
		JOIN users u ON u.id = ur.user_id WHERE r.name = 'admin' AND u.active = TRUE
	)`).Scan(&exists); err != nil {
		return fmt.Errorf("check existing admin: %w", err)
	}
	if exists {
		return fmt.Errorf("an active admin already exists; refusing to create another")
	}
	var userID, roleID int64
	if err := tx.QueryRow(ctx, `INSERT INTO users (email, password_hash, display_name)
		VALUES ($1, $2, $1) RETURNING id`, email, string(passwordHash)).Scan(&userID); err != nil {
		return fmt.Errorf("create admin user: %w", err)
	}
	if err := tx.QueryRow(ctx, "SELECT id FROM roles WHERE name = 'admin'").Scan(&roleID); err != nil {
		return fmt.Errorf("load admin role: %w", err)
	}
	if _, err := tx.Exec(ctx, "INSERT INTO user_roles (user_id, role_id) VALUES ($1, $2)", userID, roleID); err != nil {
		return fmt.Errorf("assign admin role: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit admin bootstrap: %w", err)
	}
	log.Printf("created initial administrator account for %s", email)
	return nil
}
