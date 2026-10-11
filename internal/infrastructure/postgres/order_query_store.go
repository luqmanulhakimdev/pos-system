package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/luqmanulhakimdev/pos-system/internal/application"
	"github.com/luqmanulhakimdev/pos-system/internal/domain"
)

type OrderQueryStore struct{ pool *pgxpool.Pool }

func NewOrderQueryStore(pool *pgxpool.Pool) *OrderQueryStore { return &OrderQueryStore{pool: pool} }

func (s *OrderQueryStore) ListOrders(ctx context.Context, filter application.OrderListFilter) ([]application.OrderListItem, error) {
	rows, err := s.pool.Query(ctx, `SELECT o.id,o.order_number,o.customer_id,COALESCE(c.name,''),o.cashier_id,u.display_name,o.status,o.total_minor,o.currency,o.created_at
		FROM orders o LEFT JOIN customers c ON c.id=o.customer_id JOIN users u ON u.id=o.cashier_id
		WHERE ($1='' OR o.status=$1) AND ($2='' OR o.order_number ILIKE '%'||$2||'%')
		ORDER BY o.created_at DESC,o.id DESC LIMIT $3 OFFSET $4`, string(filter.Status), filter.Search, filter.Limit, filter.Offset)
	if err != nil {
		return nil, fmt.Errorf("query orders: %w", err)
	}
	defer rows.Close()
	items := make([]application.OrderListItem, 0)
	for rows.Next() {
		var item application.OrderListItem
		var customerID pgtype.Int8
		var status string
		if err := rows.Scan(&item.ID, &item.OrderNumber, &customerID, &item.CustomerName, &item.CashierID, &item.CashierName, &status, &item.TotalMinor, &item.Currency, &item.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan order: %w", err)
		}
		if customerID.Valid {
			item.CustomerID = &customerID.Int64
		}
		item.Status = domain.OrderStatus(status)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read orders: %w", err)
	}
	return items, nil
}

func (s *OrderQueryStore) GetOrder(ctx context.Context, id int64) (application.OrderDetail, error) {
	var detail application.OrderDetail
	var customerID pgtype.Int8
	var status string
	err := s.pool.QueryRow(ctx, `SELECT o.id,o.order_number,o.customer_id,COALESCE(c.name,''),o.cashier_id,u.display_name,o.status,o.total_minor,o.currency,o.created_at
		FROM orders o LEFT JOIN customers c ON c.id=o.customer_id JOIN users u ON u.id=o.cashier_id WHERE o.id=$1`, id).Scan(
		&detail.ID, &detail.OrderNumber, &customerID, &detail.CustomerName, &detail.CashierID, &detail.CashierName, &status, &detail.TotalMinor, &detail.Currency, &detail.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return application.OrderDetail{}, application.ErrOrderNotFound
	}
	if err != nil {
		return application.OrderDetail{}, fmt.Errorf("load order: %w", err)
	}
	if customerID.Valid {
		detail.CustomerID = &customerID.Int64
	}
	detail.Status = domain.OrderStatus(status)
	rows, err := s.pool.Query(ctx, `SELECT product_id,product_name_snapshot,sku_snapshot,quantity,unit_price_minor,line_total_minor,currency FROM order_items WHERE order_id=$1 ORDER BY id`, id)
	if err != nil {
		return application.OrderDetail{}, fmt.Errorf("query order items: %w", err)
	}
	defer rows.Close()
	detail.Items = make([]domain.OrderItem, 0)
	for rows.Next() {
		var item domain.OrderItem
		if err := rows.Scan(&item.ProductID, &item.ProductName, &item.SKUSnapshot, &item.Quantity, &item.UnitPriceMinor, &item.LineTotalMinor, &item.Currency); err != nil {
			return application.OrderDetail{}, fmt.Errorf("scan order item: %w", err)
		}
		detail.Items = append(detail.Items, item)
	}
	if err := rows.Err(); err != nil {
		return application.OrderDetail{}, fmt.Errorf("read order items: %w", err)
	}
	return detail, nil
}

var _ application.OrderQueryStore = (*OrderQueryStore)(nil)
