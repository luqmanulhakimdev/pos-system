package application

import (
	"context"
	"errors"
	"testing"

	"github.com/luqmanulhakimdev/pos-system/internal/domain"
)

type cancellationStoreStub struct {
	order     OrderForCancellation
	lockErr   error
	restored  bool
	cancelled bool
}
type cancellationTxStub struct{ store *cancellationStoreStub }

func (s *cancellationStoreStub) WithinCancellationTransaction(_ context.Context, fn func(CancellationTransaction) error) error {
	return fn(cancellationTxStub{s})
}
func (tx cancellationTxStub) LockOrderForCancellation(context.Context, int64) (OrderForCancellation, error) {
	return tx.store.order, tx.store.lockErr
}
func (tx cancellationTxStub) RestoreOrderInventory(context.Context, int64, int64) error {
	tx.store.restored = true
	return nil
}
func (tx cancellationTxStub) MarkOrderCancelled(context.Context, int64, int64) error {
	tx.store.cancelled = true
	return nil
}

func TestCancelOrderRestoresOnlyCancelableOrders(t *testing.T) {
	store := &cancellationStoreStub{order: OrderForCancellation{ID: 4, Status: domain.OrderPending}}
	if err := NewCancelOrder(store).Execute(context.Background(), 4, 8); err != nil || !store.restored || !store.cancelled {
		t.Fatalf("cancel err=%v restored=%v cancelled=%v", err, store.restored, store.cancelled)
	}
	store = &cancellationStoreStub{order: OrderForCancellation{ID: 4, Status: domain.OrderPaid}}
	if err := NewCancelOrder(store).Execute(context.Background(), 4, 8); !errors.Is(err, domain.ErrInvalidOrderTransition) || store.restored {
		t.Fatalf("paid cancel err=%v restored=%v", err, store.restored)
	}
	store = &cancellationStoreStub{order: OrderForCancellation{ID: 4, Status: domain.OrderPending, PaymentBlocksCancellation: true}}
	if err := NewCancelOrder(store).Execute(context.Background(), 4, 8); !errors.Is(err, ErrOrderPaymentBlocksCancellation) || store.restored {
		t.Fatalf("pending payment err=%v restored=%v", err, store.restored)
	}
}
