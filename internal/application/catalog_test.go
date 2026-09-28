package application

import (
	"context"
	"testing"
)

type catalogStoreStub struct {
	product  Product
	category Category
}

func (s *catalogStoreStub) CreateCategory(_ context.Context, c Category) (Category, error) {
	c.ID = 1
	c.Active = true
	s.category = c
	return c, nil
}
func (s *catalogStoreStub) ListCategories(context.Context) ([]Category, error) {
	return []Category{s.category}, nil
}
func (s *catalogStoreStub) CreateProduct(_ context.Context, p Product) (Product, error) {
	p.ID = 2
	p.Active = true
	s.product = p
	return p, nil
}
func (s *catalogStoreStub) ListProducts(context.Context, ProductFilter) ([]Product, error) {
	return []Product{s.product}, nil
}

func TestCatalogNormalizesAndValidatesProducts(t *testing.T) {
	store := &catalogStoreStub{}
	catalog := NewCatalog(store)
	product, err := catalog.CreateProduct(context.Background(), Product{SKU: " SKU-1 ", Name: " Item ", PriceMinor: 0})
	if err != nil {
		t.Fatal(err)
	}
	if product.SKU != "SKU-1" || product.Name != "Item" || product.Currency != "IDR" {
		t.Fatalf("normalized product=%#v", product)
	}
	if _, err := catalog.CreateProduct(context.Background(), Product{SKU: "", Name: "Missing SKU"}); err != ErrInvalidCatalogItem {
		t.Fatalf("invalid product error=%v", err)
	}
}

func TestCatalogRejectsInvalidFiltersAndCreatesCategory(t *testing.T) {
	catalog := NewCatalog(&catalogStoreStub{})
	category, err := catalog.CreateCategory(context.Background(), Category{Name: " Grocery "})
	if err != nil || category.Name != "Grocery" {
		t.Fatalf("category=%#v err=%v", category, err)
	}
	if _, err := catalog.ListProducts(context.Background(), ProductFilter{Limit: 101}); err != ErrInvalidCatalogItem {
		t.Fatalf("filter error=%v", err)
	}
}
