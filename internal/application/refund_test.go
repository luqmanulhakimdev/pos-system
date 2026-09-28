package application

import (
	"context"
	"errors"
	"testing"
	"time"
)

type refundStoreStub struct {
	refund           Refund
	paymentReference string
	replay           bool
	finalizeCalls    int
	gotHash          [32]byte
	err              error
}

func (s *refundStoreStub) PrepareRefund(_ context.Context, orderID, actorID int64, key string, hash [32]byte, input RefundInput) (Refund, string, bool, error) {
	s.gotHash = hash
	if s.err != nil {
		return Refund{}, "", false, s.err
	}
	return s.refund, s.paymentReference, s.replay, nil
}
func (s *refundStoreStub) FinalizeRefund(_ context.Context, refund Refund, providerReference string, _ int64) (Refund, error) {
	s.finalizeCalls++
	refund.Status = "SUCCEEDED"
	return refund, nil
}

type refundProviderStub struct {
	got    RefundRequest
	result RefundResult
	err    error
	calls  int
}

func (s *refundProviderStub) Refund(_ context.Context, request RefundRequest) (RefundResult, error) {
	s.calls++
	s.got = request
	return s.result, s.err
}

func TestRefundUsesStableProviderKeyAndFinalizes(t *testing.T) {
	store := &refundStoreStub{refund: Refund{ID: 7, OrderID: 4, PaymentID: 3, AmountMinor: 900, Currency: "IDR", Status: "PENDING", CreatedAt: time.Now()}, paymentReference: "charge-ref"}
	provider := &refundProviderStub{result: RefundResult{ProviderReference: "refund-ref"}}
	got, replayed, err := NewRefunds(store, provider).Execute(context.Background(), 4, 9, "refund-key", RefundInput{Reason: " returned "})
	if err != nil || replayed || got.Status != "SUCCEEDED" || provider.got.IdempotencyKey != "pos-refund-7" || provider.got.ProviderReference != "charge-ref" || provider.calls != 1 || store.finalizeCalls != 1 {
		t.Fatalf("refund=(%#v,%v,%v) provider=%#v store=%#v", got, replayed, err, provider, store)
	}
	if store.gotHash == ([32]byte{}) {
		t.Fatal("refund reason was not hashed for idempotency")
	}
}

func TestRefundReplaysCompletedAndKeepsProviderTimeoutRetryable(t *testing.T) {
	store := &refundStoreStub{refund: Refund{ID: 5, Status: "SUCCEEDED"}, replay: true}
	provider := &refundProviderStub{}
	if _, replayed, err := NewRefunds(store, provider).Execute(context.Background(), 3, 8, "key", RefundInput{}); err != nil || !replayed || provider.calls != 0 {
		t.Fatalf("replay=%v err=%v provider calls=%d", replayed, err, provider.calls)
	}
	store = &refundStoreStub{refund: Refund{ID: 5, Status: "PENDING"}, paymentReference: "charge"}
	provider = &refundProviderStub{err: errors.New("timeout")}
	if _, _, err := NewRefunds(store, provider).Execute(context.Background(), 3, 8, "key", RefundInput{}); !errors.Is(err, ErrRefundProviderUnavailable) || store.finalizeCalls != 0 {
		t.Fatalf("timeout error=%v finalized=%d", err, store.finalizeCalls)
	}
}
