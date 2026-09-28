package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/luqmanulhakimdev/pos-system/internal/application"
	"github.com/luqmanulhakimdev/pos-system/internal/domain"
)

type CheckoutStore struct {
	pool *pgxpool.Pool
}

func NewCheckoutStore(pool *pgxpool.Pool) *CheckoutStore {
	return &CheckoutStore{pool: pool}
}

func (s *CheckoutStore) WithinTransaction(ctx context.Context, operation func(application.CheckoutTransaction) error) error {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return fmt.Errorf("begin checkout transaction: %w", err)
	}
	defer tx.Rollback(context.Background())
	if err := operation(&checkoutTransaction{tx: tx}); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit checkout transaction: %w", err)
	}
	return nil
}

type checkoutTransaction struct {
	tx pgx.Tx
}

func (tx *checkoutTransaction) LockProductAndInventory(ctx context.Context, productID int64) (application.ProductSnapshot, error) {
	var product application.ProductSnapshot
	err := tx.tx.QueryRow(ctx, `SELECT p.id, p.name, p.sku, p.price_minor, p.currency, i.quantity
		FROM products p JOIN inventory i ON i.product_id = p.id
		WHERE p.id = $1 AND p.active = TRUE
		FOR UPDATE OF p, i`, productID).Scan(
		&product.ID, &product.Name, &product.SKU, &product.PriceMinor, &product.Currency, &product.InventoryQty,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return application.ProductSnapshot{}, application.ErrProductUnavailable
	}
	if err != nil {
		return application.ProductSnapshot{}, fmt.Errorf("lock product inventory: %w", err)
	}
	return product, nil
}

func (tx *checkoutTransaction) CreateOrder(ctx context.Context, order application.OrderRecord) (int64, error) {
	var id int64
	err := tx.tx.QueryRow(ctx, `INSERT INTO orders
		(order_number, customer_id, cashier_id, status, subtotal_minor, total_minor, currency)
		VALUES ($1, $2, $3, $4, $5, $5, $6) RETURNING id`,
		order.OrderNumber, order.CustomerID, order.CashierID, domain.OrderPending, order.SubtotalMinor, order.Currency,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("insert order: %w", err)
	}
	return id, nil
}

func (tx *checkoutTransaction) CreateOrderItem(ctx context.Context, orderID int64, item domain.OrderItem) error {
	_, err := tx.tx.Exec(ctx, `INSERT INTO order_items
		(order_id, product_id, product_name_snapshot, sku_snapshot, quantity, unit_price_minor, line_total_minor, currency)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		orderID, item.ProductID, item.ProductName, item.SKUSnapshot, item.Quantity, item.UnitPriceMinor, item.LineTotalMinor, item.Currency,
	)
	if err != nil {
		return fmt.Errorf("insert order item: %w", err)
	}
	return nil
}

func (tx *checkoutTransaction) SetInventoryQuantity(ctx context.Context, productID, quantity int64) error {
	tag, err := tx.tx.Exec(ctx, "UPDATE inventory SET quantity = $2, updated_at = now() WHERE product_id = $1", productID, quantity)
	if err != nil {
		return fmt.Errorf("update inventory: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return application.ErrProductUnavailable
	}
	return nil
}

func (tx *checkoutTransaction) AppendStockMovement(ctx context.Context, productID, orderID, actorID int64, movementType domain.MovementType, delta int64, reason string) error {
	_, err := tx.tx.Exec(ctx, `INSERT INTO stock_movements
		(product_id, order_id, actor_user_id, movement_type, quantity_delta, reason)
		VALUES ($1, $2, $3, $4, $5, $6)`, productID, orderID, actorID, movementType, delta, reason)
	if err != nil {
		return fmt.Errorf("insert stock movement: %w", err)
	}
	return nil
}

func (tx *checkoutTransaction) AppendAuditLog(ctx context.Context, actorID int64, action, entityType, entityID string, details map[string]any) error {
	payload, err := json.Marshal(details)
	if err != nil {
		return fmt.Errorf("encode audit details: %w", err)
	}
	_, err = tx.tx.Exec(ctx, `INSERT INTO audit_logs
		(actor_user_id, action, entity_type, entity_id, details)
		VALUES ($1, $2, $3, $4, $5::jsonb)`, actorID, action, entityType, entityID, string(payload))
	if err != nil {
		return fmt.Errorf("insert audit log: %w", err)
	}
	return nil
}

var _ application.CheckoutStore = (*CheckoutStore)(nil)
var _ application.CheckoutTransaction = (*checkoutTransaction)(nil)
