package postgres

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/luqmanulhakimdev/pos-system/internal/application"
)

func TestPaymentStoreIntegration(t *testing.T) {
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
	var userID, orderID int64
	if err := pool.QueryRow(ctx, `INSERT INTO users(email,password_hash,display_name) VALUES($1,'unused','Payment test') RETURNING id`, suffix+"@example.test").Scan(&userID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO orders(order_number,cashier_id,status,subtotal_minor,total_minor,currency) VALUES($1,$2,'PENDING',1200,1200,'IDR') RETURNING id`, "PAY-"+suffix, userID).Scan(&orderID); err != nil {
		_, _ = pool.Exec(ctx, "DELETE FROM users WHERE id=$1", userID)
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM audit_logs WHERE entity_type='payment' AND details->>'order_id'=$1", fmt.Sprint(orderID))
		_, _ = pool.Exec(context.Background(), "DELETE FROM payments WHERE order_id=$1", orderID)
		_, _ = pool.Exec(context.Background(), "DELETE FROM orders WHERE id=$1", orderID)
		_, _ = pool.Exec(context.Background(), "DELETE FROM users WHERE id=$1", userID)
	})

	store := NewPaymentStore(pool)
	payment, err := store.PreparePayment(ctx, orderID, "mock")
	if err != nil {
		t.Fatal(err)
	}
	if payment.Status != application.PaymentPending || payment.AmountMinor != 1200 {
		t.Fatalf("prepared payment = %#v", payment)
	}
	result := application.ChargeResult{Status: application.PaymentPaid, ProviderRef: "mock-payment-" + suffix}
	paid, err := store.FinalizePayment(ctx, payment, result, userID)
	if err != nil || paid.Status != application.PaymentPaid {
		t.Fatalf("FinalizePayment() = (%#v, %v)", paid, err)
	}
	replay, err := store.FinalizePayment(ctx, payment, result, userID)
	if err != nil || replay.ProviderRef != result.ProviderRef {
		t.Fatalf("idempotent finalization = (%#v, %v)", replay, err)
	}
	var orderStatus string
	if err := pool.QueryRow(ctx, "SELECT status FROM orders WHERE id=$1", orderID).Scan(&orderStatus); err != nil {
		t.Fatal(err)
	}
	if orderStatus != "PAID" {
		t.Fatalf("order status = %s, want PAID", orderStatus)
	}
}
