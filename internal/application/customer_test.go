package application

import (
	"context"
	"testing"
)

type customerStoreStub struct {
	created Customer
	listed  CustomerFilter
}

func (s *customerStoreStub) CreateCustomer(_ context.Context, _ int64, customer Customer) (Customer, error) {
	customer.ID = 12
	s.created = customer
	return customer, nil
}

func (s *customerStoreStub) ListCustomers(_ context.Context, filter CustomerFilter) ([]Customer, error) {
	s.listed = filter
	return []Customer{}, nil
}

func TestCustomersNormalizeAndValidateInput(t *testing.T) {
	store := &customerStoreStub{}
	service := NewCustomers(store)
	customer, err := service.Create(context.Background(), 4, Customer{Name: " Ana ", Email: " ANA@EXAMPLE.COM ", Phone: " 0812 "})
	if err != nil || customer.Name != "Ana" || customer.Email != "ana@example.com" || customer.Phone != "0812" || customer.ID != 12 {
		t.Fatalf("customer=%#v err=%v", customer, err)
	}
	for _, invalid := range []Customer{{}, {Name: "No contact"}, {Name: "Bad email", Email: "not-an-email"}} {
		if _, err := service.Create(context.Background(), 4, invalid); err != ErrInvalidCustomer {
			t.Fatalf("input %#v error=%v", invalid, err)
		}
	}
}

func TestCustomerListValidatesPaginationAndTrimsSearch(t *testing.T) {
	store := &customerStoreStub{}
	service := NewCustomers(store)
	if _, err := service.List(context.Background(), CustomerFilter{Search: " Ana ", Limit: 20, Offset: 5}); err != nil {
		t.Fatal(err)
	}
	if store.listed.Search != "Ana" || store.listed.Limit != 20 || store.listed.Offset != 5 {
		t.Fatalf("filter=%#v", store.listed)
	}
	if _, err := service.List(context.Background(), CustomerFilter{Limit: 101}); err != ErrInvalidCustomer {
		t.Fatalf("invalid pagination error=%v", err)
	}
}
