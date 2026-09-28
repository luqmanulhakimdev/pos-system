package domain

import (
	"errors"
	"testing"
)

func TestApplyMovement(t *testing.T) {
	tests := []struct {
		name     string
		current  Inventory
		movement StockMovement
		want     int64
		wantErr  error
	}{
		{"receipt increases balance", Inventory{ProductID: 1, Quantity: 4}, StockMovement{ProductID: 1, Type: MovementReceipt, Delta: 6, Reason: "purchase"}, 10, nil},
		{"sale decreases balance", Inventory{ProductID: 1, Quantity: 4}, StockMovement{ProductID: 1, Type: MovementSale, Delta: -3, Reason: "checkout"}, 1, nil},
		{"sale cannot oversell", Inventory{ProductID: 1, Quantity: 2}, StockMovement{ProductID: 1, Type: MovementSale, Delta: -3, Reason: "checkout"}, 0, ErrInsufficientStock},
		{"adjustment may decrease balance", Inventory{ProductID: 1, Quantity: 8}, StockMovement{ProductID: 1, Type: MovementAdjustment, Delta: -2, Reason: "count"}, 6, nil},
		{"receipt cannot decrease stock", Inventory{ProductID: 1, Quantity: 4}, StockMovement{ProductID: 1, Type: MovementReceipt, Delta: -1, Reason: "purchase"}, 0, ErrInvalidMovement},
		{"movement must identify same product", Inventory{ProductID: 1, Quantity: 4}, StockMovement{ProductID: 2, Type: MovementSale, Delta: -1, Reason: "checkout"}, 0, ErrInvalidMovement},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ApplyMovement(tt.current, tt.movement)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, want %v", err, tt.wantErr)
			}
			if err == nil && got.Quantity != tt.want {
				t.Fatalf("quantity = %d, want %d", got.Quantity, tt.want)
			}
		})
	}
}
