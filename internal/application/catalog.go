package application

import (
	"context"
	"errors"
	"strings"
)

var (
	ErrInvalidCatalogItem = errors.New("invalid catalog item")
	ErrDuplicateSKU       = errors.New("product SKU already exists")
	ErrDuplicateCategory  = errors.New("category name already exists")
	ErrCategoryNotFound   = errors.New("category not found")
)

type Category struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Active      bool   `json:"active"`
}

type Product struct {
	ID          int64  `json:"id"`
	CategoryID  *int64 `json:"category_id,omitempty"`
	SKU         string `json:"sku"`
	Name        string `json:"name"`
	Description string `json:"description"`
	PriceMinor  int64  `json:"price_minor"`
	Currency    string `json:"currency"`
	Active      bool   `json:"active"`
}

type ProductFilter struct {
	Search     string
	CategoryID *int64
	Limit      int
	Offset     int
}

type CatalogStore interface {
	CreateCategory(context.Context, Category) (Category, error)
	ListCategories(context.Context) ([]Category, error)
	CreateProduct(context.Context, Product) (Product, error)
	ListProducts(context.Context, ProductFilter) ([]Product, error)
}

type Catalog struct{ store CatalogStore }

func NewCatalog(store CatalogStore) *Catalog { return &Catalog{store: store} }

func (c *Catalog) CreateCategory(ctx context.Context, category Category) (Category, error) {
	category.Name = strings.TrimSpace(category.Name)
	category.Description = strings.TrimSpace(category.Description)
	if c.store == nil || category.Name == "" || len(category.Name) > 120 || len(category.Description) > 1000 {
		return Category{}, ErrInvalidCatalogItem
	}
	return c.store.CreateCategory(ctx, category)
}
func (c *Catalog) ListCategories(ctx context.Context) ([]Category, error) {
	if c.store == nil {
		return nil, ErrInvalidCatalogItem
	}
	return c.store.ListCategories(ctx)
}
func (c *Catalog) CreateProduct(ctx context.Context, product Product) (Product, error) {
	product.SKU = strings.TrimSpace(product.SKU)
	product.Name = strings.TrimSpace(product.Name)
	product.Description = strings.TrimSpace(product.Description)
	if product.Currency == "" {
		product.Currency = "IDR"
	}
	if c.store == nil || product.SKU == "" || len(product.SKU) > 64 || product.Name == "" || len(product.Name) > 200 || len(product.Description) > 2000 || product.PriceMinor < 0 || product.CategoryID != nil && *product.CategoryID <= 0 || !validCatalogCurrency(product.Currency) {
		return Product{}, ErrInvalidCatalogItem
	}
	return c.store.CreateProduct(ctx, product)
}
func (c *Catalog) ListProducts(ctx context.Context, filter ProductFilter) ([]Product, error) {
	if c.store == nil || filter.Limit < 1 || filter.Limit > 100 || filter.Offset < 0 || filter.CategoryID != nil && *filter.CategoryID <= 0 || len(filter.Search) > 200 {
		return nil, ErrInvalidCatalogItem
	}
	filter.Search = strings.TrimSpace(filter.Search)
	return c.store.ListProducts(ctx, filter)
}
func validCatalogCurrency(currency string) bool {
	if len(currency) != 3 {
		return false
	}
	for _, char := range currency {
		if char < 'A' || char > 'Z' {
			return false
		}
	}
	return true
}
