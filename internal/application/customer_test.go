package application

import (
	"context"
	"testing"
)

type customerStoreStub struct {
	created            Customer
	updated            Customer
	listed             CustomerFilter
	deactivatedActorID int64
	deactivatedID      int64
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
func (s *customerStoreStub) UpdateCustomer(_ context.Context, _ int64, id int64, customer Customer) (Customer, error) {
	customer.ID = id
	s.updated = customer
	return customer, nil
}
func (s *customerStoreStub) DeactivateCustomer(_ context.Context, actorID, id int64) error {
	s.deactivatedActorID = actorID
	s.deactivatedID = id
	return nil
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

func TestCustomerUpdatesAndDeactivatesCustomer(t *testing.T) {
	store := &customerStoreStub{}
	service := NewCustomers(store)
	updated, err := service.Update(context.Background(), 4, 12, Customer{Name: " Updated Ana ", Email: " ANA@EXAMPLE.COM ", Phone: " 0812 "})
	if err != nil || updated.Name != "Updated Ana" || updated.Email != "ana@example.com" || updated.Phone != "0812" || updated.ID != 12 || store.updated.ID != 12 {
		t.Fatalf("updated=%#v store=%#v err=%v", updated, store.updated, err)
	}
	if err := service.Deactivate(context.Background(), 4, 12); err != nil || store.deactivatedActorID != 4 || store.deactivatedID != 12 {
		t.Fatalf("deactivate err=%v actor=%d id=%d", err, store.deactivatedActorID, store.deactivatedID)
	}
	for _, invalid := range []Customer{{}, {Name: "No contact"}, {Name: "Bad email", Email: "not-an-email"}} {
		if _, err := service.Update(context.Background(), 4, 12, invalid); err != ErrInvalidCustomer {
			t.Fatalf("invalid update input %#v error=%v", invalid, err)
		}
	}
	if _, err := service.Update(context.Background(), 0, 12, Customer{Name: "Ana", Phone: "0812"}); err != ErrInvalidCustomer {
		t.Fatalf("invalid actor error=%v", err)
	}
	if _, err := service.Update(context.Background(), 4, 0, Customer{Name: "Ana", Phone: "0812"}); err != ErrInvalidCustomer {
		t.Fatalf("invalid customer id error=%v", err)
	}
	if err := service.Deactivate(context.Background(), 0, 12); err != ErrInvalidCustomer {
		t.Fatalf("invalid deactivate actor error=%v", err)
	}
	if err := service.Deactivate(context.Background(), 4, 0); err != ErrInvalidCustomer {
		t.Fatalf("invalid deactivate customer id error=%v", err)
	}
}
