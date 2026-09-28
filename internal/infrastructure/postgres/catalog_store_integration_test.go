package postgres

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/luqmanulhakimdev/pos-system/internal/application"
)

func TestCatalogStoreIntegration(t *testing.T) {
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
	store := NewCatalogStore(pool)
	category, err := store.CreateCategory(ctx, application.Category{Name: "Catalog test " + suffix, Description: "fixture"})
	if err != nil {
		t.Fatal(err)
	}
	var productID int64
	t.Cleanup(func() {
		if productID > 0 {
			_, _ = pool.Exec(context.Background(), "DELETE FROM inventory WHERE product_id=$1", productID)
			_, _ = pool.Exec(context.Background(), "DELETE FROM products WHERE id=$1", productID)
		}
		_, _ = pool.Exec(context.Background(), "DELETE FROM categories WHERE id=$1", category.ID)
	})
	product, err := store.CreateProduct(ctx, application.Product{CategoryID: &category.ID, SKU: "CAT-" + suffix, Name: "Coffee", PriceMinor: 12500, Currency: "IDR"})
	if err != nil {
		t.Fatal(err)
	}
	productID = product.ID
	var initialQuantity int64
	if err := pool.QueryRow(ctx, "SELECT quantity FROM inventory WHERE product_id=$1", productID).Scan(&initialQuantity); err != nil {
		t.Fatal(err)
	}
	if initialQuantity != 0 {
		t.Fatalf("new product inventory=%d, want 0", initialQuantity)
	}
	products, err := store.ListProducts(ctx, application.ProductFilter{Search: "CAT-" + suffix, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(products) != 1 || products[0].CategoryID == nil || *products[0].CategoryID != category.ID || products[0].PriceMinor != 12500 {
		t.Fatalf("products = %#v", products)
	}
	if _, err := store.CreateProduct(ctx, application.Product{SKU: product.SKU, Name: "Duplicate", Currency: "IDR"}); err != application.ErrDuplicateSKU {
		t.Fatalf("duplicate SKU error=%v", err)
	}
	updated, err := store.UpdateProduct(ctx, application.Product{ID: productID, CategoryID: product.CategoryID, SKU: product.SKU, Name: "Updated Coffee", PriceMinor: 13000, Currency: "IDR"})
	if err != nil || updated.Name != "Updated Coffee" || updated.PriceMinor != 13000 {
		t.Fatalf("updated product=%#v err=%v", updated, err)
	}
	if err := store.DeactivateProduct(ctx, productID); err != nil {
		t.Fatal(err)
	}
	activeProducts, err := store.ListProducts(ctx, application.ProductFilter{Search: product.SKU, Limit: 10})
	if err != nil || len(activeProducts) != 0 {
		t.Fatalf("deactivated products=%#v err=%v", activeProducts, err)
	}
}
