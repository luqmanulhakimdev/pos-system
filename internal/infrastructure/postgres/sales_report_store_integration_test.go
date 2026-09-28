package postgres

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/luqmanulhakimdev/pos-system/internal/application"
)

func TestSalesReportStoreIntegrationGroupsGrossAndRefundedAmounts(t *testing.T) {
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
	var actorID, paidOrderID, refundedOrderID, paymentID int64
	if err := pool.QueryRow(ctx, `INSERT INTO users(email,password_hash,display_name) VALUES($1,'unused','Sales report test') RETURNING id`, "sales-report-"+suffix+"@example.test").Scan(&actorID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO orders(order_number,cashier_id,status,subtotal_minor,total_minor,currency) VALUES($1,$2,'PAID',1000,1000,'IDR') RETURNING id`, "SALES-PAID-"+suffix, actorID).Scan(&paidOrderID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO orders(order_number,cashier_id,status,subtotal_minor,total_minor,currency) VALUES($1,$2,'REFUNDED',400,400,'IDR') RETURNING id`, "SALES-REFUNDED-"+suffix, actorID).Scan(&refundedOrderID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO payments(order_id,provider,status,amount_minor,currency) VALUES($1,'mock','REFUNDED',400,'IDR') RETURNING id`, refundedOrderID).Scan(&paymentID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO refunds(order_id,payment_id,idempotency_key,request_hash,amount_minor,currency,status) VALUES($1,$2,$3,$4,400,'IDR','SUCCEEDED')`, refundedOrderID, paymentID, "report-"+suffix, make([]byte, 32)); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM refunds WHERE order_id IN ($1,$2)", paidOrderID, refundedOrderID)
		_, _ = pool.Exec(context.Background(), "DELETE FROM payments WHERE order_id IN ($1,$2)", paidOrderID, refundedOrderID)
		_, _ = pool.Exec(context.Background(), "DELETE FROM orders WHERE id IN ($1,$2)", paidOrderID, refundedOrderID)
		_, _ = pool.Exec(context.Background(), "DELETE FROM users WHERE id=$1", actorID)
	})
	today := time.Now().UTC().Format("2006-01-02")
	report, err := application.NewSalesReports(NewSalesReportStore(pool)).Execute(ctx, today, today)
	if err != nil || len(report.Currencies) != 1 {
		t.Fatalf("report=%#v err=%v", report, err)
	}
	got := report.Currencies[0]
	if got.Currency != "IDR" || got.PaidOrderCount != 2 || got.GrossMinor != 1400 || got.RefundedMinor != 400 || got.NetMinor != 1000 {
		t.Fatalf("summary=%#v", got)
	}
}
