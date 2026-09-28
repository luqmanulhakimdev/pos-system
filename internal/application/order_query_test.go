package application

import (
	"context"
	"testing"

	"github.com/luqmanulhakimdev/pos-system/internal/domain"
)

type orderQueryStoreStub struct {
	filter OrderListFilter
	id     int64
	list   []OrderListItem
	detail OrderDetail
}

func (s *orderQueryStoreStub) ListOrders(_ context.Context, filter OrderListFilter) ([]OrderListItem, error) {
	s.filter = filter
	return s.list, nil
}
func (s *orderQueryStoreStub) GetOrder(_ context.Context, id int64) (OrderDetail, error) {
	s.id = id
	return s.detail, nil
}

func TestOrderQueriesValidateFilterAndPreserveSnapshots(t *testing.T) {
	store := &orderQueryStoreStub{list: []OrderListItem{{ID: 2, Status: domain.OrderPaid}}}
	queries := NewOrderQueries(store)
	items, err := queries.List(context.Background(), OrderListFilter{Status: "PAID", Search: " POS- ", Limit: 20, Offset: 4})
	if err != nil || len(items) != 1 || store.filter.Status != domain.OrderPaid || store.filter.Search != "POS-" {
		t.Fatalf("items=%#v filter=%#v err=%v", items, store.filter, err)
	}
	detail := OrderDetail{OrderListItem: OrderListItem{ID: 2, Status: domain.OrderPaid}, Items: []domain.OrderItem{{ProductID: 8, ProductName: "Old name", SKUSnapshot: "SKU-OLD", UnitPriceMinor: 500}}}
	store.detail = detail
	got, err := queries.Get(context.Background(), 2)
	if err != nil || got.Items[0].ProductName != "Old name" || got.Items[0].SKUSnapshot != "SKU-OLD" || store.id != 2 {
		t.Fatalf("detail=%#v err=%v", got, err)
	}
	if _, err := queries.List(context.Background(), OrderListFilter{Status: "UNKNOWN", Limit: 10}); err != ErrInvalidCheckout {
		t.Fatalf("invalid status error=%v", err)
	}
}
