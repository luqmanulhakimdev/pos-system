package postgres

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/luqmanulhakimdev/pos-system/internal/application"
)

func TestCustomerStoreIntegrationCreateListAndAudit(t *testing.T) {
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
	var actorID int64
	if err := pool.QueryRow(ctx, `INSERT INTO users(email,password_hash,display_name) VALUES($1,'unused','Customer test') RETURNING id`, "customer-"+suffix+"@example.test").Scan(&actorID); err != nil {
		t.Fatal(err)
	}
	store := NewCustomerStore(pool)
	service := application.NewCustomers(store)
	customer, err := service.Create(ctx, actorID, application.Customer{Name: "Customer Test", Email: "customer-" + suffix + "@example.test", Phone: "0812345678"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM audit_logs WHERE actor_user_id=$1", actorID)
		_, _ = pool.Exec(context.Background(), "DELETE FROM customers WHERE id=$1", customer.ID)
		_, _ = pool.Exec(context.Background(), "DELETE FROM users WHERE id=$1", actorID)
	})
	items, err := service.List(ctx, application.CustomerFilter{Search: suffix, Limit: 10})
	if err != nil || len(items) != 1 || items[0].ID != customer.ID {
		t.Fatalf("customers=%#v err=%v", items, err)
	}
	var auditCount int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM audit_logs WHERE actor_user_id=$1 AND action='customer.created' AND entity_id=$2", actorID, fmt.Sprint(customer.ID)).Scan(&auditCount); err != nil {
		t.Fatal(err)
	}
	if auditCount != 1 {
		t.Fatalf("customer creation audit count=%d", auditCount)
	}
}
