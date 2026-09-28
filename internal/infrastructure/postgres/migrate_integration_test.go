package postgres

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestMigrationsIntegration(t *testing.T) {
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
		t.Fatalf("apply migrations: %v", err)
	}
	if err := ApplyMigrations(ctx, pool); err != nil {
		t.Fatalf("reapply migrations: %v", err)
	}

	var tableCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM information_schema.tables
		WHERE table_schema = 'public' AND table_name IN
		('users','roles','permissions','products','inventory','stock_movements','orders','order_items','payments','audit_logs')`).Scan(&tableCount); err != nil {
		t.Fatal(err)
	}
	if tableCount != 10 {
		t.Fatalf("core table count = %d, want 10", tableCount)
	}

	var seededRoles int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM roles WHERE name = ANY($1)", []string{"admin", "manager", "cashier", "inventory_staff"}).Scan(&seededRoles); err != nil {
		t.Fatal(err)
	}
	if seededRoles != 4 {
		t.Fatalf("seeded roles = %d, want 4", seededRoles)
	}
}
