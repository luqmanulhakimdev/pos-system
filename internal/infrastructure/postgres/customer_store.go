package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/luqmanulhakimdev/pos-system/internal/application"
)

type CustomerStore struct{ pool *pgxpool.Pool }

func NewCustomerStore(pool *pgxpool.Pool) *CustomerStore { return &CustomerStore{pool: pool} }

func (s *CustomerStore) CreateCustomer(ctx context.Context, actorID int64, customer application.Customer) (application.Customer, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return application.Customer{}, fmt.Errorf("begin customer creation: %w", err)
	}
	defer tx.Rollback(ctx)
	err = tx.QueryRow(ctx, `INSERT INTO customers(name,email,phone) VALUES($1,NULLIF($2,''),NULLIF($3,'')) RETURNING id,name,COALESCE(email,''),COALESCE(phone,''),created_at,updated_at,active`, customer.Name, customer.Email, customer.Phone).Scan(&customer.ID, &customer.Name, &customer.Email, &customer.Phone, &customer.CreatedAt, &customer.UpdatedAt, &customer.Active)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return application.Customer{}, application.ErrDuplicateCustomerEmail
		}
		return application.Customer{}, fmt.Errorf("insert customer: %w", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO audit_logs(actor_user_id,action,entity_type,entity_id,details) VALUES($1,'customer.created','customer',$2,jsonb_build_object('customer_id',$2::bigint))`, actorID, customer.ID); err != nil {
		return application.Customer{}, fmt.Errorf("audit customer creation: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return application.Customer{}, fmt.Errorf("commit customer creation: %w", err)
	}
	return customer, nil
}

func (s *CustomerStore) ListCustomers(ctx context.Context, filter application.CustomerFilter) ([]application.Customer, error) {
	rows, err := s.pool.Query(ctx, `SELECT id,name,COALESCE(email,''),COALESCE(phone,''),created_at,updated_at,active
		FROM customers WHERE active=TRUE AND ($1='' OR name ILIKE '%'||$1||'%' OR email ILIKE '%'||$1||'%' OR phone ILIKE '%'||$1||'%')
		ORDER BY name,id LIMIT $2 OFFSET $3`, filter.Search, filter.Limit, filter.Offset)
	if err != nil {
		return nil, fmt.Errorf("list customers: %w", err)
	}
	defer rows.Close()
	customers := make([]application.Customer, 0)
	for rows.Next() {
		var customer application.Customer
		if err := rows.Scan(&customer.ID, &customer.Name, &customer.Email, &customer.Phone, &customer.CreatedAt, &customer.UpdatedAt, &customer.Active); err != nil {
			return nil, fmt.Errorf("scan customer: %w", err)
		}
		customers = append(customers, customer)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read customers: %w", err)
	}
	return customers, nil
}

func (s *CustomerStore) UpdateCustomer(ctx context.Context, actorID, customerID int64, customer application.Customer) (application.Customer, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return application.Customer{}, fmt.Errorf("begin customer update: %w", err)
	}
	defer tx.Rollback(ctx)
	err = tx.QueryRow(ctx, `UPDATE customers SET name=$2,email=NULLIF($3,''),phone=NULLIF($4,''),updated_at=now() WHERE id=$1 AND active=TRUE
		RETURNING id,name,COALESCE(email,''),COALESCE(phone,''),created_at,updated_at,active`, customerID, customer.Name, customer.Email, customer.Phone).
		Scan(&customer.ID, &customer.Name, &customer.Email, &customer.Phone, &customer.CreatedAt, &customer.UpdatedAt, &customer.Active)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return application.Customer{}, application.ErrCustomerNotFound
		}
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return application.Customer{}, application.ErrDuplicateCustomerEmail
		}
		return application.Customer{}, fmt.Errorf("update customer: %w", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO audit_logs(actor_user_id,action,entity_type,entity_id,details) VALUES($1,'customer.updated','customer',$2,jsonb_build_object('customer_id',$2::bigint))`, actorID, customerID); err != nil {
		return application.Customer{}, fmt.Errorf("audit customer update: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return application.Customer{}, fmt.Errorf("commit customer update: %w", err)
	}
	return customer, nil
}

func (s *CustomerStore) DeactivateCustomer(ctx context.Context, actorID, customerID int64) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin customer deactivation: %w", err)
	}
	defer tx.Rollback(ctx)
	tag, err := tx.Exec(ctx, "UPDATE customers SET active=FALSE,updated_at=now() WHERE id=$1 AND active=TRUE", customerID)
	if err != nil {
		return fmt.Errorf("deactivate customer: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return application.ErrCustomerNotFound
	}
	if _, err := tx.Exec(ctx, `INSERT INTO audit_logs(actor_user_id,action,entity_type,entity_id,details) VALUES($1,'customer.deactivated','customer',$2,jsonb_build_object('customer_id',$2::bigint))`, actorID, customerID); err != nil {
		return fmt.Errorf("audit customer deactivation: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit customer deactivation: %w", err)
	}
	return nil
}

var _ application.CustomerStore = (*CustomerStore)(nil)
