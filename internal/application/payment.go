package application

import (
	"context"
	"errors"
	"fmt"
)

var (
	ErrInvalidPayment  = errors.New("invalid payment request")
	ErrOrderNotPayable = errors.New("order is not payable")
)

type PaymentStatus string

const (
	PaymentPending PaymentStatus = "PENDING"
	PaymentPaid    PaymentStatus = "PAID"
	PaymentFailed  PaymentStatus = "FAILED"
)

type Payment struct {
	ID          int64         `json:"id"`
	OrderID     int64         `json:"order_id"`
	Status      PaymentStatus `json:"status"`
	AmountMinor int64         `json:"amount_minor"`
	Currency    string        `json:"currency"`
	Provider    string        `json:"provider"`
	ProviderRef string        `json:"provider_reference,omitempty"`
}

type ChargeRequest struct {
	PaymentID      int64
	OrderID        int64
	AmountMinor    int64
	Currency       string
	IdempotencyKey string
}

type ChargeResult struct {
	Status      PaymentStatus
	ProviderRef string
}

// PaymentProvider is the outbound port. Implementations must honor the stable key
// so retries after a timeout cannot create duplicate charges.
type PaymentProvider interface {
	Name() string
	Charge(context.Context, ChargeRequest) (ChargeResult, error)
}

type RefundRequest struct {
	RefundID          int64
	OrderID           int64
	PaymentID         int64
	ProviderReference string
	AmountMinor       int64
	Currency          string
	IdempotencyKey    string
}

type RefundResult struct{ ProviderReference string }

type RefundProvider interface {
	Refund(context.Context, RefundRequest) (RefundResult, error)
}

type PaymentStore interface {
	PreparePayment(context.Context, int64, string) (Payment, error)
	FinalizePayment(context.Context, Payment, ChargeResult, int64) (Payment, error)
}

type Payments struct {
	store    PaymentStore
	provider PaymentProvider
}

func NewPayments(store PaymentStore, provider PaymentProvider) *Payments {
	return &Payments{store: store, provider: provider}
}

func (p *Payments) ChargeOrder(ctx context.Context, orderID, actorID int64) (Payment, error) {
	if p.store == nil || p.provider == nil || orderID <= 0 || actorID <= 0 {
		return Payment{}, ErrInvalidPayment
	}
	providerName := p.provider.Name()
	if providerName == "" {
		return Payment{}, ErrInvalidPayment
	}
	payment, err := p.store.PreparePayment(ctx, orderID, providerName)
	if err != nil {
		return Payment{}, err
	}
	if payment.Status == PaymentPaid {
		return payment, nil
	}
	if payment.Status != PaymentPending || payment.ID <= 0 || payment.AmountMinor < 0 || payment.Currency == "" {
		return Payment{}, ErrInvalidPayment
	}
	result, err := p.provider.Charge(ctx, ChargeRequest{
		PaymentID: payment.ID, OrderID: payment.OrderID, AmountMinor: payment.AmountMinor,
		Currency: payment.Currency, IdempotencyKey: fmt.Sprintf("pos-payment-%d", payment.ID),
	})
	if err != nil {
		return Payment{}, err
	}
	if result.Status != PaymentPaid && result.Status != PaymentFailed {
		return Payment{}, ErrInvalidPayment
	}
	if result.Status == PaymentPaid && result.ProviderRef == "" {
		return Payment{}, ErrInvalidPayment
	}
	return p.store.FinalizePayment(ctx, payment, result, actorID)
}
