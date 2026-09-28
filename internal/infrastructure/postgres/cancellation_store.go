package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/luqmanulhakimdev/pos-system/internal/application"
	"github.com/luqmanulhakimdev/pos-system/internal/domain"
)

type CancellationStore struct{ pool *pgxpool.Pool }

func NewCancellationStore(pool *pgxpool.Pool) *CancellationStore {
	return &CancellationStore{pool: pool}
}

func (s *CancellationStore) WithinCancellationTransaction(ctx context.Context, operation func(application.CancellationTransaction) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin order cancellation: %w", err)
	}
	defer tx.Rollback(context.Background())
	if err := operation(&cancellationTransaction{tx: tx}); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit order cancellation: %w", err)
	}
	return nil
}

type cancellationTransaction struct{ tx pgx.Tx }

func (tx *cancellationTransaction) LockOrderForCancellation(ctx context.Context, orderID int64) (application.OrderForCancellation, error) {
	var order application.OrderForCancellation
	var status string
	err := tx.tx.QueryRow(ctx, `SELECT o.id,o.status,EXISTS(SELECT 1 FROM payments p WHERE p.order_id=o.id AND p.status IN ('PENDING','AUTHORIZED','PAID'))
		FROM orders o WHERE o.id=$1 FOR UPDATE OF o`, orderID).Scan(&order.ID, &status, &order.PaymentBlocksCancellation)
	if errors.Is(err, pgx.ErrNoRows) {
		return application.OrderForCancellation{}, application.ErrOrderNotFound
	}
	if err != nil {
		return application.OrderForCancellation{}, fmt.Errorf("lock order for cancellation: %w", err)
	}
	order.Status = domain.OrderStatus(status)
	return order, nil
}

func (tx *cancellationTransaction) RestoreOrderInventory(ctx context.Context, orderID, actorID int64) error {
	rows, err := tx.tx.Query(ctx, "SELECT product_id,quantity FROM order_items WHERE order_id=$1 ORDER BY product_id", orderID)
	if err != nil {
		return fmt.Errorf("load order items for cancellation: %w", err)
	}
	type item struct{ productID, quantity int64 }
	items := make([]item, 0)
	for rows.Next() {
		var current item
		if err := rows.Scan(&current.productID, &current.quantity); err != nil {
			rows.Close()
			return fmt.Errorf("scan order item: %w", err)
		}
		items = append(items, current)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("read order items: %w", err)
	}
	rows.Close()
	if len(items) == 0 {
		return domain.ErrInvalidOrder
	}
	for _, current := range items {
		var quantity int64
		if err := tx.tx.QueryRow(ctx, "SELECT quantity FROM inventory WHERE product_id=$1 FOR UPDATE", current.productID).Scan(&quantity); errors.Is(err, pgx.ErrNoRows) {
			return application.ErrProductUnavailable
		} else if err != nil {
			return fmt.Errorf("lock inventory for cancellation: %w", err)
		}
		updated, err := domain.ApplyMovement(domain.Inventory{ProductID: current.productID, Quantity: quantity}, domain.StockMovement{ProductID: current.productID, Type: domain.MovementCancellation, Delta: current.quantity, Reason: "order cancellation"})
		if err != nil {
			return err
		}
		tag, err := tx.tx.Exec(ctx, "UPDATE inventory SET quantity=$2,updated_at=now() WHERE product_id=$1", current.productID, updated.Quantity)
		if err != nil {
			return fmt.Errorf("restore inventory for product %d: %w", current.productID, err)
		}
		if tag.RowsAffected() != 1 {
			return application.ErrProductUnavailable
		}
		if _, err := tx.tx.Exec(ctx, `INSERT INTO stock_movements(product_id,order_id,actor_user_id,movement_type,quantity_delta,reason) VALUES($1,$2,$3,$4,$5,'order cancellation')`, current.productID, orderID, actorID, domain.MovementCancellation, current.quantity); err != nil {
			return fmt.Errorf("record cancellation stock movement: %w", err)
		}
	}
	return nil
}

func (tx *cancellationTransaction) MarkOrderCancelled(ctx context.Context, orderID, actorID int64) error {
	tag, err := tx.tx.Exec(ctx, `UPDATE orders SET status='CANCELLED',updated_at=now() WHERE id=$1 AND status IN ('PENDING','CONFIRMED')`, orderID)
	if err != nil {
		return fmt.Errorf("mark order cancelled: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return domain.ErrInvalidOrderTransition
	}
	if _, err := tx.tx.Exec(ctx, `INSERT INTO audit_logs(actor_user_id,action,entity_type,entity_id,details) VALUES($1,'order.cancelled','order',$2,jsonb_build_object('order_id',$2::bigint))`, actorID, fmt.Sprint(orderID)); err != nil {
		return fmt.Errorf("audit order cancellation: %w", err)
	}
	return nil
}

var _ application.CancellationStore = (*CancellationStore)(nil)
var _ application.CancellationTransaction = (*cancellationTransaction)(nil)
