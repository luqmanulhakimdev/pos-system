package application

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/luqmanulhakimdev/pos-system/internal/domain"
)

var ErrInvalidInventoryChange = errors.New("invalid inventory change")

type InventoryChangeRequest struct {
	ProductID int64
	ActorID   int64
	Type      domain.MovementType
	Delta     int64
	Reason    string
}

type InventoryService struct {
	store  CheckoutStore
	reader InventoryReader
}

type InventoryReader interface {
	GetInventory(context.Context, int64) (domain.Inventory, error)
}

func NewInventoryService(store CheckoutStore, reader InventoryReader) *InventoryService {
	return &InventoryService{store: store, reader: reader}
}

func (s *InventoryService) Get(ctx context.Context, productID int64) (domain.Inventory, error) {
	if s.reader == nil || productID <= 0 {
		return domain.Inventory{}, ErrInvalidInventoryChange
	}
	return s.reader.GetInventory(ctx, productID)
}

func (s *InventoryService) Change(ctx context.Context, request InventoryChangeRequest) (domain.Inventory, error) {
	if s.store == nil || request.ProductID <= 0 || request.ActorID <= 0 || strings.TrimSpace(request.Reason) == "" || request.Delta == 0 {
		return domain.Inventory{}, ErrInvalidInventoryChange
	}
	if request.Type != domain.MovementReceipt && request.Type != domain.MovementAdjustment && request.Type != domain.MovementReturn {
		return domain.Inventory{}, ErrInvalidInventoryChange
	}
	var updated domain.Inventory
	err := s.store.WithinTransaction(ctx, func(tx CheckoutTransaction) error {
		product, err := tx.LockProductAndInventory(ctx, request.ProductID)
		if err != nil {
			return err
		}
		movement := domain.StockMovement{
			ProductID: request.ProductID,
			Type:      request.Type,
			Delta:     request.Delta,
			Reason:    strings.TrimSpace(request.Reason),
		}
		updated, err = domain.ApplyMovement(domain.Inventory{ProductID: product.ID, Quantity: product.InventoryQty}, movement)
		if err != nil {
			return err
		}
		if err := tx.SetInventoryQuantity(ctx, request.ProductID, updated.Quantity); err != nil {
			return err
		}
		if err := tx.AppendStockMovement(ctx, request.ProductID, nil, request.ActorID, request.Type, request.Delta, movement.Reason); err != nil {
			return err
		}
		if err := tx.AppendAuditLog(ctx, request.ActorID, "inventory.changed", "product", fmt.Sprint(request.ProductID), map[string]any{
			"movement_type":  request.Type,
			"quantity_delta": request.Delta,
			"reason":         movement.Reason,
			"quantity_after": updated.Quantity,
		}); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return domain.Inventory{}, err
	}
	return updated, nil
}
