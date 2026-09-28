package application_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/luqmanulhakimdev/pos-system/internal/application"
	"github.com/luqmanulhakimdev/pos-system/internal/domain"
	"github.com/luqmanulhakimdev/pos-system/internal/infrastructure/postgres"
)

func TestCheckoutIntegrationPreventsConcurrentOversell(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := postgres.NewPool(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err := postgres.ApplyMigrations(ctx, pool); err != nil {
		t.Fatal(err)
	}

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	var userID, productID int64
	if err := pool.QueryRow(ctx, `INSERT INTO users (email, password_hash, display_name)
		VALUES ($1, 'test-hash', 'Checkout integration') RETURNING id`, suffix+"@example.test").Scan(&userID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO products (sku, name, price_minor, currency)
		VALUES ($1, 'Integration item', 125, 'IDR') RETURNING id`, "TEST-"+suffix).Scan(&productID); err != nil {
		t.Cleanup(func() { _, _ = pool.Exec(context.Background(), "DELETE FROM users WHERE id = $1", userID) })
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "INSERT INTO inventory (product_id, quantity) VALUES ($1, 5)", productID); err != nil {
		t.Cleanup(func() {
			_, _ = pool.Exec(context.Background(), "DELETE FROM products WHERE id = $1", productID)
			_, _ = pool.Exec(context.Background(), "DELETE FROM users WHERE id = $1", userID)
		})
		t.Fatal(err)
	}
	t.Cleanup(func() { cleanupCheckoutFixture(pool, userID, productID) })

	checkout := application.NewCheckout(postgres.NewCheckoutStore(pool))
	start := make(chan struct{})
	errorsFound := make(chan error, 2)
	var wait sync.WaitGroup
	for range 2 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			_, err := checkout.Execute(ctx, application.CheckoutRequest{
				CashierID: userID,
				Items:     []application.RequestedItem{{ProductID: productID, Quantity: 4}},
			})
			errorsFound <- err
		}()
	}
	close(start)
	wait.Wait()
	close(errorsFound)

	var succeeded, insufficient int
	for err := range errorsFound {
		switch {
		case err == nil:
			succeeded++
		case errors.Is(err, domain.ErrInsufficientStock):
			insufficient++
		default:
			t.Fatalf("unexpected checkout error: %v", err)
		}
	}
	if succeeded != 1 || insufficient != 1 {
		t.Fatalf("successful checkouts = %d, insufficient stock = %d; want one each", succeeded, insufficient)
	}

	var remaining, movementCount, orderCount, auditCount int
	if err := pool.QueryRow(ctx, "SELECT quantity FROM inventory WHERE product_id = $1", productID).Scan(&remaining); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM stock_movements WHERE product_id = $1", productID).Scan(&movementCount); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM orders WHERE cashier_id = $1", userID).Scan(&orderCount); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM audit_logs WHERE actor_user_id = $1", userID).Scan(&auditCount); err != nil {
		t.Fatal(err)
	}
	if remaining != 1 || movementCount != 1 || orderCount != 1 || auditCount != 1 {
		t.Fatalf("remaining=%d movements=%d orders=%d audits=%d; want 1,1,1,1", remaining, movementCount, orderCount, auditCount)
	}
}

func cleanupCheckoutFixture(pool *pgxpool.Pool, userID, productID int64) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	tx, err := pool.Begin(ctx)
	if err != nil {
		return
	}
	defer tx.Rollback(ctx)
	_, _ = tx.Exec(ctx, `DELETE FROM audit_logs WHERE actor_user_id = $1`, userID)
	_, _ = tx.Exec(ctx, `DELETE FROM stock_movements WHERE product_id = $1`, productID)
	_, _ = tx.Exec(ctx, `DELETE FROM order_items WHERE order_id IN (SELECT id FROM orders WHERE cashier_id = $1)`, userID)
	_, _ = tx.Exec(ctx, "DELETE FROM orders WHERE cashier_id = $1", userID)
	_, _ = tx.Exec(ctx, "DELETE FROM inventory WHERE product_id = $1", productID)
	_, _ = tx.Exec(ctx, "DELETE FROM products WHERE id = $1", productID)
	_, _ = tx.Exec(ctx, "DELETE FROM users WHERE id = $1", userID)
	_ = tx.Commit(ctx)
}
