package payment

import (
	"context"
	"testing"

	"github.com/luqmanulhakimdev/pos-system/internal/application"
)

func TestMockProviderReplaysSameCharge(t *testing.T) {
	provider := NewMockProvider()
	request := application.ChargeRequest{PaymentID: 9, OrderID: 4, AmountMinor: 500, Currency: "IDR", IdempotencyKey: "stable-9"}
	first, err := provider.Charge(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	second, err := provider.Charge(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if first != second || first.Status != application.PaymentPaid {
		t.Fatalf("first=%#v second=%#v", first, second)
	}
}
