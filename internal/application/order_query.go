package application

import (
	"context"
	"strings"
	"time"

	"github.com/luqmanulhakimdev/pos-system/internal/domain"
)

type OrderListItem struct {
	ID           int64              `json:"id"`
	OrderNumber  string             `json:"order_number"`
	CustomerID   *int64             `json:"customer_id,omitempty"`
	CustomerName string             `json:"customer_name,omitempty"`
	CashierID    int64              `json:"cashier_id"`
	CashierName  string             `json:"cashier_name"`
	Status       domain.OrderStatus `json:"status"`
	TotalMinor   int64              `json:"total_minor"`
	Currency     string             `json:"currency"`
	CreatedAt    time.Time          `json:"created_at"`
}

type OrderDetail struct {
	OrderListItem
	Items []domain.OrderItem `json:"items"`
}

type OrderListFilter struct {
	Status        domain.OrderStatus
	Search        string
	Limit, Offset int
}

type OrderQueryStore interface {
	ListOrders(context.Context, OrderListFilter) ([]OrderListItem, error)
	GetOrder(context.Context, int64) (OrderDetail, error)
}

type OrderQueries struct{ store OrderQueryStore }

func NewOrderQueries(store OrderQueryStore) *OrderQueries { return &OrderQueries{store: store} }

func (q *OrderQueries) List(ctx context.Context, filter OrderListFilter) ([]OrderListItem, error) {
	if q == nil || q.store == nil || filter.Limit < 1 || filter.Limit > 100 || filter.Offset < 0 || len(filter.Search) > 100 {
		return nil, ErrInvalidCheckout
	}
	filter.Status = domain.OrderStatus(strings.TrimSpace(string(filter.Status)))
	switch filter.Status {
	case "", domain.OrderPending, domain.OrderConfirmed, domain.OrderPaid, domain.OrderCancelled, domain.OrderRefunded:
	default:
		return nil, ErrInvalidCheckout
	}
	filter.Search = strings.TrimSpace(filter.Search)
	return q.store.ListOrders(ctx, filter)
}

func (q *OrderQueries) Get(ctx context.Context, id int64) (OrderDetail, error) {
	if q == nil || q.store == nil || id <= 0 {
		return OrderDetail{}, domain.ErrInvalidOrder
	}
	return q.store.GetOrder(ctx, id)
}
