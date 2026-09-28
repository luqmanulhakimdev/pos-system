package postgres

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/luqmanulhakimdev/pos-system/internal/application"
	"github.com/luqmanulhakimdev/pos-system/internal/domain"
)

func TestCancellationStoreIntegrationRestoresStockAtomically(t *testing.T) {
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
	var actorID, productID, orderID int64
	if err := pool.QueryRow(ctx, `INSERT INTO users(email,password_hash,display_name) VALUES($1,'unused','Cancellation test') RETURNING id`, "cancel-"+suffix+"@example.test").Scan(&actorID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO products(sku,name,price_minor,currency) VALUES($1,'Cancellation item',100,'IDR') RETURNING id`, "CANCEL-"+suffix).Scan(&productID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "INSERT INTO inventory(product_id,quantity) VALUES($1,3)", productID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO orders(order_number,cashier_id,status,subtotal_minor,total_minor,currency) VALUES($1,$2,'PENDING',200,200,'IDR') RETURNING id`, "CANCEL-ORDER-"+suffix, actorID).Scan(&orderID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO order_items(order_id,product_id,product_name_snapshot,sku_snapshot,quantity,unit_price_minor,line_total_minor,currency) VALUES($1,$2,'Cancellation item',$3,2,100,200,'IDR')`, orderID, productID, "CANCEL-"+suffix); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM audit_logs WHERE actor_user_id=$1", actorID)
		_, _ = pool.Exec(context.Background(), "DELETE FROM stock_movements WHERE order_id=$1", orderID)
		_, _ = pool.Exec(context.Background(), "DELETE FROM order_items WHERE order_id=$1", orderID)
		_, _ = pool.Exec(context.Background(), "DELETE FROM orders WHERE id=$1", orderID)
		_, _ = pool.Exec(context.Background(), "DELETE FROM inventory WHERE product_id=$1", productID)
		_, _ = pool.Exec(context.Background(), "DELETE FROM products WHERE id=$1", productID)
		_, _ = pool.Exec(context.Background(), "DELETE FROM users WHERE id=$1", actorID)
	})
	if err := application.NewCancelOrder(NewCancellationStore(pool)).Execute(ctx, orderID, actorID); err != nil {
		t.Fatal(err)
	}
	var status string
	var quantity int64
	var movements int
	if err := pool.QueryRow(ctx, "SELECT status FROM orders WHERE id=$1", orderID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, "SELECT quantity FROM inventory WHERE product_id=$1", productID).Scan(&quantity); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM stock_movements WHERE order_id=$1 AND movement_type='CANCELLATION' AND quantity_delta=2", orderID).Scan(&movements); err != nil {
		t.Fatal(err)
	}
	if status != "CANCELLED" || quantity != 5 || movements != 1 {
		t.Fatalf("status=%s quantity=%d cancellation movements=%d", status, quantity, movements)
	}
	if err := application.NewCancelOrder(NewCancellationStore(pool)).Execute(ctx, orderID, actorID); !errors.Is(err, domain.ErrInvalidOrderTransition) {
		t.Fatalf("second cancellation error=%v", err)
	}
}
