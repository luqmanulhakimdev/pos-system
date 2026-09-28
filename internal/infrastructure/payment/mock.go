package payment

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"

	"github.com/luqmanulhakimdev/pos-system/internal/application"
)

// MockProvider is a deterministic, in-memory provider for local development.
// It never accepts or stores card data and replays the same result for retries.
type MockProvider struct {
	mu      sync.Mutex
	results map[string]application.ChargeResult
}

func NewMockProvider() *MockProvider {
	return &MockProvider{results: make(map[string]application.ChargeResult)}
}
func (p *MockProvider) Name() string { return "mock" }

func (p *MockProvider) Charge(_ context.Context, request application.ChargeRequest) (application.ChargeResult, error) {
	if request.PaymentID <= 0 || request.OrderID <= 0 || request.AmountMinor < 0 || request.Currency == "" || request.IdempotencyKey == "" {
		return application.ChargeResult{}, errors.New("invalid mock charge request")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if result, exists := p.results[request.IdempotencyKey]; exists {
		return result, nil
	}
	digest := sha256.Sum256([]byte(request.IdempotencyKey))
	result := application.ChargeResult{Status: application.PaymentPaid, ProviderRef: fmt.Sprintf("mock_%s", hex.EncodeToString(digest[:12]))}
	p.results[request.IdempotencyKey] = result
	return result, nil
}
