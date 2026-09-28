package postgres

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/luqmanulhakimdev/pos-system/internal/application"
	"github.com/luqmanulhakimdev/pos-system/internal/infrastructure/payment"
)

func TestRefundStoreIntegrationIdempotentPaidOrderRefund(t *testing.T) {
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
	var actorID, orderID, paymentID int64
	if err := pool.QueryRow(ctx, `INSERT INTO users(email,password_hash,display_name) VALUES($1,'unused','Refund test') RETURNING id`, "refund-"+suffix+"@example.test").Scan(&actorID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO orders(order_number,cashier_id,status,subtotal_minor,total_minor,currency) VALUES($1,$2,'PAID',1999,1999,'IDR') RETURNING id`, "REFUND-ORDER-"+suffix, actorID).Scan(&orderID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO payments(order_id,provider,provider_reference,status,amount_minor,currency) VALUES($1,'mock',$2,'PAID',1999,'IDR') RETURNING id`, orderID, "charge-"+suffix).Scan(&paymentID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM audit_logs WHERE actor_user_id=$1", actorID)
		_, _ = pool.Exec(context.Background(), "DELETE FROM refunds WHERE order_id=$1", orderID)
		_, _ = pool.Exec(context.Background(), "DELETE FROM payments WHERE order_id=$1", orderID)
		_, _ = pool.Exec(context.Background(), "DELETE FROM orders WHERE id=$1", orderID)
		_, _ = pool.Exec(context.Background(), "DELETE FROM users WHERE id=$1", actorID)
	})
	service := application.NewRefunds(NewRefundStore(pool), payment.NewMockProvider())
	refund, replayed, err := service.Execute(ctx, orderID, actorID, "refund-1", application.RefundInput{Reason: "customer request"})
	if err != nil || replayed || refund.Status != "SUCCEEDED" || refund.PaymentID != paymentID {
		t.Fatalf("refund=%#v replay=%v err=%v", refund, replayed, err)
	}
	second, replayed, err := service.Execute(ctx, orderID, actorID, "refund-1", application.RefundInput{Reason: "customer request"})
	if err != nil || !replayed || second.ID != refund.ID {
		t.Fatalf("replay=%#v replay=%v err=%v", second, replayed, err)
	}
	var orderStatus, paymentStatus string
	var refundCount int
	if err := pool.QueryRow(ctx, "SELECT status FROM orders WHERE id=$1", orderID).Scan(&orderStatus); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, "SELECT status FROM payments WHERE id=$1", paymentID).Scan(&paymentStatus); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM refunds WHERE order_id=$1", orderID).Scan(&refundCount); err != nil {
		t.Fatal(err)
	}
	if orderStatus != "REFUNDED" || paymentStatus != "REFUNDED" || refundCount != 1 {
		t.Fatalf("order=%s payment=%s refunds=%d", orderStatus, paymentStatus, refundCount)
	}
}
