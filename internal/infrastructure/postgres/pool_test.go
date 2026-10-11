package postgres

import (
	"context"
	"errors"
	"testing"
)

func TestNewPoolValidatesURL(t *testing.T) {
	_, err := NewPool(context.Background(), "")
	if !errors.Is(err, ErrMissingDatabaseURL) {
		t.Fatalf("expected ErrMissingDatabaseURL, got %v", err)
	}

	_, err = NewPool(context.Background(), "invalid://url?%xx")
	if err == nil {
		t.Fatal("expected error parsing invalid database URL, got nil")
	}
}
