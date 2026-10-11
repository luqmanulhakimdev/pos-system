package application

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrInvalidRefund             = errors.New("invalid refund request")
	ErrOrderNotRefundable        = errors.New("order is not refundable")
	ErrRefundInProgress          = errors.New("refund is already in progress")
	ErrRefundConflict            = errors.New("idempotency key conflicts with an existing refund")
	ErrRefundProviderUnavailable = errors.New("refund provider unavailable")
)

type Refund struct {
	ID          int64     `json:"id"`
	OrderID     int64     `json:"order_id"`
	PaymentID   int64     `json:"payment_id"`
	AmountMinor int64     `json:"amount_minor"`
	Currency    string    `json:"currency"`
	Status      string    `json:"status"`
	Reason      string    `json:"reason"`
	CreatedAt   time.Time `json:"created_at"`
}

type RefundInput struct {
	AmountMinor int64  `json:"amount_minor,omitempty"`
	Reason      string `json:"reason"`
}

type RefundStore interface {
	PrepareRefund(context.Context, int64, int64, string, [32]byte, RefundInput) (Refund, string, bool, error)
	FinalizeRefund(context.Context, Refund, string, int64) (Refund, error)
}

type Refunds struct {
	store    RefundStore
	provider RefundProvider
}

func NewRefunds(store RefundStore, provider RefundProvider) *Refunds {
	return &Refunds{store: store, provider: provider}
}

func (s *Refunds) Execute(ctx context.Context, orderID, actorID int64, key string, input RefundInput) (Refund, bool, error) {
	input.Reason = strings.TrimSpace(input.Reason)
	if s == nil || s.store == nil || s.provider == nil || orderID <= 0 || actorID <= 0 || key == "" || len(key) > 255 || strings.TrimSpace(key) != key || input.AmountMinor < 0 || len(input.Reason) > 250 {
		return Refund{}, false, ErrInvalidRefund
	}
	hash := sha256.Sum256([]byte(fmt.Sprintf("%d\x00%s", input.AmountMinor, input.Reason)))
	refund, paymentReference, replayed, err := s.store.PrepareRefund(ctx, orderID, actorID, key, hash, input)
	if err != nil {
		return Refund{}, false, err
	}
	if replayed {
		return refund, true, nil
	}
	providerKey := fmt.Sprintf("pos-refund-%d", refund.ID)
	result, err := s.provider.Refund(ctx, RefundRequest{RefundID: refund.ID, OrderID: orderID, PaymentID: refund.PaymentID, ProviderReference: paymentReference, AmountMinor: refund.AmountMinor, Currency: refund.Currency, IdempotencyKey: providerKey})
	if err != nil {
		return Refund{}, false, fmt.Errorf("%w: %v", ErrRefundProviderUnavailable, err)
	}
	if strings.TrimSpace(result.ProviderReference) == "" {
		return Refund{}, false, ErrRefundProviderUnavailable
	}
	refund, err = s.store.FinalizeRefund(ctx, refund, result.ProviderReference, actorID)
	if err != nil {
		return Refund{}, false, err
	}
	return refund, false, nil
}
