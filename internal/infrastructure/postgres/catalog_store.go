package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/luqmanulhakimdev/pos-system/internal/application"
	"github.com/luqmanulhakimdev/pos-system/internal/domain"
)

type CatalogStore struct{ pool *pgxpool.Pool }

func NewCatalogStore(pool *pgxpool.Pool) *CatalogStore { return &CatalogStore{pool: pool} }

func (s *CatalogStore) CreateCategory(ctx context.Context, category application.Category) (application.Category, error) {
	err := s.pool.QueryRow(ctx, `INSERT INTO categories(name,description) VALUES($1,$2) RETURNING id,name,description,active`, category.Name, category.Description).Scan(&category.ID, &category.Name, &category.Description, &category.Active)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return application.Category{}, application.ErrDuplicateCategory
		}
		return application.Category{}, fmt.Errorf("insert category: %w", err)
	}
	return category, nil
}

func (s *CatalogStore) ListCategories(ctx context.Context) ([]application.Category, error) {
	rows, err := s.pool.Query(ctx, "SELECT id,name,description,active FROM categories WHERE active=TRUE ORDER BY name,id")
	if err != nil {
		return nil, fmt.Errorf("list categories: %w", err)
	}
	defer rows.Close()
	items := make([]application.Category, 0)
	for rows.Next() {
		var item application.Category
		if err := rows.Scan(&item.ID, &item.Name, &item.Description, &item.Active); err != nil {
			return nil, fmt.Errorf("scan category: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read categories: %w", err)
	}
	return items, nil
}

func (s *CatalogStore) CreateProduct(ctx context.Context, product application.Product) (application.Product, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return application.Product{}, fmt.Errorf("begin product creation: %w", err)
	}
	defer tx.Rollback(ctx)
	var categoryID pgtype.Int8
	err = tx.QueryRow(ctx, `INSERT INTO products(category_id,sku,name,description,price_minor,currency) VALUES($1,$2,$3,$4,$5,$6)
		RETURNING id,category_id,sku,name,description,price_minor,currency,active`, product.CategoryID, product.SKU, product.Name, product.Description, product.PriceMinor, product.Currency).Scan(&product.ID, &categoryID, &product.SKU, &product.Name, &product.Description, &product.PriceMinor, &product.Currency, &product.Active)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			switch pgErr.Code {
			case "23505":
				return application.Product{}, application.ErrDuplicateSKU
			case "23503":
				return application.Product{}, application.ErrCategoryNotFound
			}
		}
		return application.Product{}, fmt.Errorf("insert product: %w", err)
	}
	if categoryID.Valid {
		product.CategoryID = &categoryID.Int64
	}
	if _, err := tx.Exec(ctx, "INSERT INTO inventory(product_id,quantity) VALUES($1,0)", product.ID); err != nil {
		return application.Product{}, fmt.Errorf("create product inventory: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return application.Product{}, fmt.Errorf("commit product creation: %w", err)
	}
	return product, nil
}

func (s *CatalogStore) ListProducts(ctx context.Context, filter application.ProductFilter) ([]application.Product, error) {
	rows, err := s.pool.Query(ctx, `SELECT id,category_id,sku,name,description,price_minor,currency,active FROM products
		WHERE active=TRUE AND ($1='' OR name ILIKE '%'||$1||'%' OR sku ILIKE '%'||$1||'%') AND ($2::bigint IS NULL OR category_id=$2)
		ORDER BY name,id LIMIT $3 OFFSET $4`, filter.Search, filter.CategoryID, filter.Limit, filter.Offset)
	if err != nil {
		return nil, fmt.Errorf("list products: %w", err)
	}
	defer rows.Close()
	products := make([]application.Product, 0)
	for rows.Next() {
		var product application.Product
		var category pgtype.Int8
		if err := rows.Scan(&product.ID, &category, &product.SKU, &product.Name, &product.Description, &product.PriceMinor, &product.Currency, &product.Active); err != nil {
			return nil, fmt.Errorf("scan product: %w", err)
		}
		if category.Valid {
			product.CategoryID = &category.Int64
		}
		products = append(products, product)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read products: %w", err)
	}
	return products, nil
}

var _ application.CatalogStore = (*CatalogStore)(nil)

func (s *CatalogStore) GetInventory(ctx context.Context, productID int64) (domain.Inventory, error) {
	var inventory domain.Inventory
	err := s.pool.QueryRow(ctx, "SELECT product_id,quantity FROM inventory WHERE product_id=$1", productID).Scan(&inventory.ProductID, &inventory.Quantity)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Inventory{}, application.ErrProductUnavailable
	}
	if err != nil {
		return domain.Inventory{}, fmt.Errorf("load inventory: %w", err)
	}
	return inventory, nil
}

var _ application.InventoryReader = (*CatalogStore)(nil)
