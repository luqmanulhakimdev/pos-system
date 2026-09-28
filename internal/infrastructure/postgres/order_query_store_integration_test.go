package postgres

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/luqmanulhakimdev/pos-system/internal/application"
	"github.com/luqmanulhakimdev/pos-system/internal/domain"
)

func TestOrderQueryStoreIntegrationListsAndLoadsPriceSnapshots(t *testing.T) {
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
	if err := pool.QueryRow(ctx, `INSERT INTO users(email,password_hash,display_name) VALUES($1,'unused','Order query test') RETURNING id`, "order-query-"+suffix+"@example.test").Scan(&actorID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO products(sku,name,price_minor,currency) VALUES($1,'Current name',900,'IDR') RETURNING id`, "ORDER-QUERY-"+suffix).Scan(&productID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO orders(order_number,cashier_id,status,subtotal_minor,total_minor,currency) VALUES($1,$2,'PENDING',1000,1000,'IDR') RETURNING id`, "ORDER-QUERY-"+suffix, actorID).Scan(&orderID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO order_items(order_id,product_id,product_name_snapshot,sku_snapshot,quantity,unit_price_minor,line_total_minor,currency) VALUES($1,$2,'Historical name','HIST-SKU',2,500,1000,'IDR')`, orderID, productID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM order_items WHERE order_id=$1", orderID)
		_, _ = pool.Exec(context.Background(), "DELETE FROM orders WHERE id=$1", orderID)
		_, _ = pool.Exec(context.Background(), "DELETE FROM products WHERE id=$1", productID)
		_, _ = pool.Exec(context.Background(), "DELETE FROM users WHERE id=$1", actorID)
	})
	queries := application.NewOrderQueries(NewOrderQueryStore(pool))
	list, err := queries.List(ctx, application.OrderListFilter{Status: domain.OrderPending, Search: suffix, Limit: 10})
	if err != nil || len(list) != 1 || list[0].OrderNumber != "ORDER-QUERY-"+suffix {
		t.Fatalf("list=%#v err=%v", list, err)
	}
	detail, err := queries.Get(ctx, orderID)
	if err != nil || len(detail.Items) != 1 || detail.Items[0].ProductName != "Historical name" || detail.Items[0].UnitPriceMinor != 500 {
		t.Fatalf("detail=%#v err=%v", detail, err)
	}
}
