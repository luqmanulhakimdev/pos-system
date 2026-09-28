package application

import (
	"context"
	"errors"

	"github.com/luqmanulhakimdev/pos-system/internal/domain"
)

var (
	ErrOrderNotFound                  = errors.New("order not found")
	ErrOrderPaymentBlocksCancellation = errors.New("order payment must be resolved before cancellation")
)

type OrderForCancellation struct {
	ID                        int64
	Status                    domain.OrderStatus
	PaymentBlocksCancellation bool
}

type CancellationTransaction interface {
	LockOrderForCancellation(context.Context, int64) (OrderForCancellation, error)
	RestoreOrderInventory(context.Context, int64, int64) error
	MarkOrderCancelled(context.Context, int64, int64) error
}

type CancellationStore interface {
	WithinCancellationTransaction(context.Context, func(CancellationTransaction) error) error
}

type CancelOrder struct{ store CancellationStore }

func NewCancelOrder(store CancellationStore) *CancelOrder { return &CancelOrder{store: store} }

func (c *CancelOrder) Execute(ctx context.Context, orderID, actorID int64) error {
	if c == nil || c.store == nil || orderID <= 0 || actorID <= 0 {
		return domain.ErrInvalidOrder
	}
	return c.store.WithinCancellationTransaction(ctx, func(tx CancellationTransaction) error {
		order, err := tx.LockOrderForCancellation(ctx, orderID)
		if err != nil {
			return err
		}
		if order.PaymentBlocksCancellation {
			return ErrOrderPaymentBlocksCancellation
		}
		if _, err := (domain.Order{ID: order.ID, Status: order.Status}).Transition(domain.OrderCancelled); err != nil {
			return err
		}
		if err := tx.RestoreOrderInventory(ctx, orderID, actorID); err != nil {
			return err
		}
		return tx.MarkOrderCancelled(ctx, orderID, actorID)
	})
}
