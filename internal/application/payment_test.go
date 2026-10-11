package application

import (
	"context"
	"errors"
	"testing"
)

type paymentStoreStub struct {
	payment   Payment
	result    ChargeResult
	err       error
	prepared  int
	finalized int
}

func (s *paymentStoreStub) PreparePayment(context.Context, int64, string) (Payment, error) {
	s.prepared++
	return s.payment, s.err
}
func (s *paymentStoreStub) FinalizePayment(_ context.Context, p Payment, result ChargeResult, _ int64) (Payment, error) {
	s.finalized++
	p.Status = result.Status
	p.ProviderRef = result.ProviderRef
	return p, s.err
}

type paymentProviderStub struct {
	result ChargeResult
	err    error
	got    ChargeRequest
	calls  int
}

func (p *paymentProviderStub) Name() string { return "test-provider" }
func (p *paymentProviderStub) Charge(_ context.Context, request ChargeRequest) (ChargeResult, error) {
	p.calls++
	p.got = request
	return p.result, p.err
}

func TestChargeOrderUsesStableProviderIdempotencyKey(t *testing.T) {
	store := &paymentStoreStub{payment: Payment{ID: 17, OrderID: 8, Status: PaymentPending, AmountMinor: 2400, Currency: "IDR", Provider: "test-provider"}}
	provider := &paymentProviderStub{result: ChargeResult{Status: PaymentPaid, ProviderRef: "ref-17"}}
	service := NewPayments(store, provider)
	got, err := service.ChargeOrder(context.Background(), 8, 3)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != PaymentPaid || got.ProviderRef != "ref-17" || store.finalized != 1 {
		t.Fatalf("result=%#v finalized=%d", got, store.finalized)
	}
	if provider.got.IdempotencyKey != "pos-payment-17" || provider.got.AmountMinor != 2400 || provider.got.Currency != "IDR" {
		t.Fatalf("provider request = %#v", provider.got)
	}
}

func TestChargeOrderDoesNotFinalizeProviderFailure(t *testing.T) {
	store := &paymentStoreStub{payment: Payment{ID: 2, OrderID: 5, Status: PaymentPending, AmountMinor: 10, Currency: "IDR"}}
	provider := &paymentProviderStub{err: errors.New("timeout")}
	_, err := NewPayments(store, provider).ChargeOrder(context.Background(), 5, 3)
	if err == nil || store.finalized != 0 {
		t.Fatalf("err=%v finalized=%d, want provider error and pending retry", err, store.finalized)
	}
}

func TestChargeOrderReplaysPaidPaymentWithoutProviderCall(t *testing.T) {
	store := &paymentStoreStub{payment: Payment{ID: 2, OrderID: 5, Status: PaymentPaid, ProviderRef: "existing"}}
	provider := &paymentProviderStub{}
	got, err := NewPayments(store, provider).ChargeOrder(context.Background(), 5, 3)
	if err != nil || got.ProviderRef != "existing" || provider.calls != 0 {
		t.Fatalf("result=%#v err=%v provider calls=%d", got, err, provider.calls)
	}
}
