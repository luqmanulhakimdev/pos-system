package application

import (
	"context"
	"errors"
	"net/mail"
	"strings"
	"time"
)

var (
	ErrInvalidCustomer        = errors.New("invalid customer")
	ErrDuplicateCustomerEmail = errors.New("customer email already exists")
	ErrCustomerNotFound       = errors.New("customer not found")
)

type Customer struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Email     string    `json:"email,omitempty"`
	Phone     string    `json:"phone,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Active    bool      `json:"active"`
}

type CustomerFilter struct {
	Search        string
	Limit, Offset int
}

type CustomerStore interface {
	CreateCustomer(context.Context, int64, Customer) (Customer, error)
	ListCustomers(context.Context, CustomerFilter) ([]Customer, error)
	UpdateCustomer(context.Context, int64, int64, Customer) (Customer, error)
	DeactivateCustomer(context.Context, int64, int64) error
}

func (s *Customers) Update(ctx context.Context, actorID, customerID int64, customer Customer) (Customer, error) {
	customer.Name = strings.TrimSpace(customer.Name)
	customer.Email = strings.ToLower(strings.TrimSpace(customer.Email))
	customer.Phone = strings.TrimSpace(customer.Phone)
	if s == nil || s.store == nil || actorID <= 0 || customerID <= 0 || customer.Name == "" || len(customer.Name) > 120 || len(customer.Email) > 254 || len(customer.Phone) > 32 || customer.Email == "" && customer.Phone == "" {
		return Customer{}, ErrInvalidCustomer
	}
	if customer.Email != "" {
		parsed, err := mail.ParseAddress(customer.Email)
		if err != nil || parsed.Address != customer.Email {
			return Customer{}, ErrInvalidCustomer
		}
	}
	return s.store.UpdateCustomer(ctx, actorID, customerID, customer)
}

func (s *Customers) Deactivate(ctx context.Context, actorID, customerID int64) error {
	if s == nil || s.store == nil || actorID <= 0 || customerID <= 0 {
		return ErrInvalidCustomer
	}
	return s.store.DeactivateCustomer(ctx, actorID, customerID)
}

type Customers struct{ store CustomerStore }

func NewCustomers(store CustomerStore) *Customers { return &Customers{store: store} }

func (s *Customers) Create(ctx context.Context, actorID int64, customer Customer) (Customer, error) {
	customer.Name = strings.TrimSpace(customer.Name)
	customer.Email = strings.ToLower(strings.TrimSpace(customer.Email))
	customer.Phone = strings.TrimSpace(customer.Phone)
	if s == nil || s.store == nil || actorID <= 0 || customer.Name == "" || len(customer.Name) > 120 || len(customer.Email) > 254 || len(customer.Phone) > 32 || customer.Email == "" && customer.Phone == "" {
		return Customer{}, ErrInvalidCustomer
	}
	if customer.Email != "" {
		parsed, err := mail.ParseAddress(customer.Email)
		if err != nil || parsed.Address != customer.Email {
			return Customer{}, ErrInvalidCustomer
		}
	}
	return s.store.CreateCustomer(ctx, actorID, customer)
}

func (s *Customers) List(ctx context.Context, filter CustomerFilter) ([]Customer, error) {
	if s == nil || s.store == nil || filter.Limit < 1 || filter.Limit > 100 || filter.Offset < 0 || len(filter.Search) > 120 {
		return nil, ErrInvalidCustomer
	}
	filter.Search = strings.TrimSpace(filter.Search)
	return s.store.ListCustomers(ctx, filter)
}
