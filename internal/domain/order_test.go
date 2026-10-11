package domain

import (
	"errors"
	"math"
	"testing"
)

func TestNewOrderItemSnapshotsPrice(t *testing.T) {
	item, err := NewOrderItem(3, "Coffee", "COF-1", 2, 12500, "IDR")
	if err != nil {
		t.Fatal(err)
	}
	if item.LineTotalMinor != 25000 || item.UnitPriceMinor != 12500 {
		t.Fatalf("unexpected price snapshot: %+v", item)
	}
}

func TestNewOrderRejectsMixedCurrencyAndOverflow(t *testing.T) {
	idr, _ := NewOrderItem(1, "Tea", "TEA", 1, 100, "IDR")
	usd, _ := NewOrderItem(2, "Coffee", "COF", 1, 100, "USD")
	if _, err := NewOrder([]OrderItem{idr, usd}, OrderPending); !errors.Is(err, ErrInvalidOrder) {
		t.Fatalf("mixed currencies error = %v, want %v", err, ErrInvalidOrder)
	}
	if _, err := NewOrderItem(1, "Tea", "TEA", 2, math.MaxInt64, "IDR"); !errors.Is(err, ErrInvalidOrder) {
		t.Fatalf("overflow error = %v, want %v", err, ErrInvalidOrder)
	}
}

func TestOrderTransition(t *testing.T) {
	order, err := NewOrder([]OrderItem{{ProductID: 1, ProductName: "Tea", SKUSnapshot: "TEA", Quantity: 1, UnitPriceMinor: 100, LineTotalMinor: 100, Currency: "IDR"}}, OrderPending)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := order.Transition(OrderPaid); !errors.Is(err, ErrInvalidOrderTransition) {
		t.Fatalf("pending to paid error = %v, want invalid transition", err)
	}
	confirmed, err := order.Transition(OrderConfirmed)
	if err != nil || confirmed.Status != OrderConfirmed {
		t.Fatalf("confirmed order = %+v, error = %v", confirmed, err)
	}
}
