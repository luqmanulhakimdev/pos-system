package domain

import (
	"errors"
	"fmt"
	"math"
	"strings"
)

var (
	ErrInvalidOrder           = errors.New("invalid order")
	ErrInvalidOrderTransition = errors.New("invalid order state transition")
)

type OrderStatus string

const (
	OrderPending   OrderStatus = "PENDING"
	OrderConfirmed OrderStatus = "CONFIRMED"
	OrderPaid      OrderStatus = "PAID"
	OrderCancelled OrderStatus = "CANCELLED"
	OrderRefunded  OrderStatus = "REFUNDED"
)

type OrderItem struct {
	ProductID      int64
	ProductName    string
	SKUSnapshot    string
	Quantity       int64
	UnitPriceMinor int64
	LineTotalMinor int64
	Currency       string
}

func NewOrderItem(productID int64, name, sku string, quantity, unitPriceMinor int64, currency string) (OrderItem, error) {
	if productID <= 0 || strings.TrimSpace(name) == "" || strings.TrimSpace(sku) == "" || quantity <= 0 || unitPriceMinor < 0 || !validCurrency(currency) {
		return OrderItem{}, ErrInvalidOrder
	}
	if unitPriceMinor > 0 && quantity > math.MaxInt64/unitPriceMinor {
		return OrderItem{}, fmt.Errorf("%w: line total overflow", ErrInvalidOrder)
	}
	return OrderItem{
		ProductID:      productID,
		ProductName:    name,
		SKUSnapshot:    sku,
		Quantity:       quantity,
		UnitPriceMinor: unitPriceMinor,
		LineTotalMinor: quantity * unitPriceMinor,
		Currency:       currency,
	}, nil
}

type Order struct {
	ID            int64       `json:"id"`
	Status        OrderStatus `json:"status"`
	Items         []OrderItem `json:"items"`
	SubtotalMinor int64       `json:"subtotal_minor"`
	Currency      string      `json:"currency"`
}

func NewOrder(items []OrderItem, status OrderStatus) (Order, error) {
	if len(items) == 0 || !validOrderStatus(status) {
		return Order{}, ErrInvalidOrder
	}
	currency := items[0].Currency
	var subtotal int64
	for _, item := range items {
		if item.ProductID <= 0 || strings.TrimSpace(item.ProductName) == "" || strings.TrimSpace(item.SKUSnapshot) == "" || item.Quantity <= 0 || item.UnitPriceMinor < 0 || !validCurrency(item.Currency) || item.Currency != currency {
			return Order{}, ErrInvalidOrder
		}
		if item.UnitPriceMinor > 0 && item.Quantity > math.MaxInt64/item.UnitPriceMinor {
			return Order{}, fmt.Errorf("%w: line total overflow", ErrInvalidOrder)
		}
		lineTotal := item.Quantity * item.UnitPriceMinor
		if lineTotal != item.LineTotalMinor || subtotal > math.MaxInt64-lineTotal {
			return Order{}, fmt.Errorf("%w: inconsistent or overflowing line total", ErrInvalidOrder)
		}
		subtotal += lineTotal
	}
	return Order{Status: status, Items: append([]OrderItem(nil), items...), SubtotalMinor: subtotal, Currency: currency}, nil
}

func (o Order) Transition(to OrderStatus) (Order, error) {
	if !validOrderStatus(o.Status) {
		return Order{}, ErrInvalidOrder
	}
	allowed := map[OrderStatus][]OrderStatus{
		OrderPending:   {OrderConfirmed, OrderCancelled},
		OrderConfirmed: {OrderPaid, OrderCancelled},
		OrderPaid:      {OrderRefunded},
	}
	for _, next := range allowed[o.Status] {
		if next == to {
			o.Status = to
			return o, nil
		}
	}
	return Order{}, fmt.Errorf("%w: %s -> %s", ErrInvalidOrderTransition, o.Status, to)
}

func validOrderStatus(status OrderStatus) bool {
	switch status {
	case OrderPending, OrderConfirmed, OrderPaid, OrderCancelled, OrderRefunded:
		return true
	default:
		return false
	}
}
