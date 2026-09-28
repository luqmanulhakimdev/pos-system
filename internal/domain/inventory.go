package domain

import (
	"errors"
	"fmt"
	"math"
	"strings"
)

var (
	ErrInvalidMovement   = errors.New("invalid stock movement")
	ErrInsufficientStock = errors.New("insufficient stock")
)

type MovementType string

const (
	MovementReceipt      MovementType = "RECEIPT"
	MovementAdjustment   MovementType = "ADJUSTMENT"
	MovementSale         MovementType = "SALE"
	MovementReturn       MovementType = "RETURN"
	MovementCancellation MovementType = "CANCELLATION"
)

type Inventory struct {
	ProductID int64
	Quantity  int64
}

type StockMovement struct {
	ProductID int64
	Type      MovementType
	Delta     int64
	Reason    string
}

// ApplyMovement validates a stock ledger entry and returns the resulting balance.
// The caller must persist the movement and balance change atomically.
func ApplyMovement(current Inventory, movement StockMovement) (Inventory, error) {
	if current.ProductID <= 0 || movement.ProductID != current.ProductID || current.Quantity < 0 || movement.Delta == 0 {
		return Inventory{}, ErrInvalidMovement
	}
	if strings.TrimSpace(movement.Reason) == "" {
		return Inventory{}, fmt.Errorf("%w: reason is required", ErrInvalidMovement)
	}

	switch movement.Type {
	case MovementReceipt, MovementReturn, MovementCancellation:
		if movement.Delta < 0 {
			return Inventory{}, fmt.Errorf("%w: %s must increase stock", ErrInvalidMovement, movement.Type)
		}
	case MovementSale:
		if movement.Delta > 0 {
			return Inventory{}, fmt.Errorf("%w: sale must decrease stock", ErrInvalidMovement)
		}
	case MovementAdjustment:
	default:
		return Inventory{}, fmt.Errorf("%w: unknown type %q", ErrInvalidMovement, movement.Type)
	}

	if movement.Delta > 0 && current.Quantity > math.MaxInt64-movement.Delta {
		return Inventory{}, fmt.Errorf("%w: quantity overflow", ErrInvalidMovement)
	}
	if movement.Delta < 0 && movement.Delta < -current.Quantity {
		return Inventory{}, ErrInsufficientStock
	}

	current.Quantity += movement.Delta
	return current, nil
}
