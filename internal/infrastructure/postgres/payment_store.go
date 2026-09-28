package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/luqmanulhakimdev/pos-system/internal/application"
)

type PaymentStore struct{ pool *pgxpool.Pool }

func NewPaymentStore(pool *pgxpool.Pool) *PaymentStore { return &PaymentStore{pool: pool} }

func (s *PaymentStore) PreparePayment(ctx context.Context, orderID int64, provider string) (application.Payment, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return application.Payment{}, fmt.Errorf("begin payment preparation: %w", err)
	}
	defer tx.Rollback(ctx)
	var status string
	var amount int64
	var currency string
	if err := tx.QueryRow(ctx, "SELECT status,total_minor,currency FROM orders WHERE id=$1 FOR UPDATE", orderID).Scan(&status, &amount, &currency); errors.Is(err, pgx.ErrNoRows) {
		return application.Payment{}, application.ErrOrderNotPayable
	} else if err != nil {
		return application.Payment{}, fmt.Errorf("load order for payment: %w", err)
	}
	if status == "PAID" {
		payment, err := scanPayment(tx.QueryRow(ctx, `SELECT id,order_id,status,amount_minor,currency,provider,COALESCE(provider_reference,'') FROM payments WHERE order_id=$1 AND status='PAID' ORDER BY id DESC LIMIT 1`, orderID))
		if errors.Is(err, pgx.ErrNoRows) {
			return application.Payment{}, application.ErrOrderNotPayable
		}
		if err != nil {
			return application.Payment{}, fmt.Errorf("load completed payment: %w", err)
		}
		if err := tx.Commit(ctx); err != nil {
			return application.Payment{}, fmt.Errorf("commit payment lookup: %w", err)
		}
		return payment, nil
	}
	if status != "PENDING" && status != "CONFIRMED" {
		return application.Payment{}, application.ErrOrderNotPayable
	}
	payment, err := scanPayment(tx.QueryRow(ctx, `SELECT id,order_id,status,amount_minor,currency,provider,COALESCE(provider_reference,'') FROM payments WHERE order_id=$1 AND status='PENDING' ORDER BY id DESC LIMIT 1`, orderID))
	if errors.Is(err, pgx.ErrNoRows) {
		payment.Provider = provider
		payment.OrderID = orderID
		payment.Status = application.PaymentPending
		payment.AmountMinor = amount
		payment.Currency = currency
		err = tx.QueryRow(ctx, `INSERT INTO payments(order_id,provider,status,amount_minor,currency) VALUES($1,$2,'PENDING',$3,$4) RETURNING id`, orderID, provider, amount, currency).Scan(&payment.ID)
	}
	if err != nil {
		return application.Payment{}, fmt.Errorf("prepare payment attempt: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return application.Payment{}, fmt.Errorf("commit payment preparation: %w", err)
	}
	return payment, nil
}

func (s *PaymentStore) FinalizePayment(ctx context.Context, payment application.Payment, result application.ChargeResult, actorID int64) (application.Payment, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return application.Payment{}, fmt.Errorf("begin payment finalization: %w", err)
	}
	defer tx.Rollback(ctx)
	var current string
	if err := tx.QueryRow(ctx, "SELECT status FROM payments WHERE id=$1 FOR UPDATE", payment.ID).Scan(&current); errors.Is(err, pgx.ErrNoRows) {
		return application.Payment{}, application.ErrInvalidPayment
	} else if err != nil {
		return application.Payment{}, fmt.Errorf("lock payment attempt: %w", err)
	}
	if current != string(application.PaymentPending) {
		if current == string(result.Status) {
			stored, err := scanPayment(tx.QueryRow(ctx, `SELECT id,order_id,status,amount_minor,currency,provider,COALESCE(provider_reference,'') FROM payments WHERE id=$1`, payment.ID))
			if err != nil {
				return application.Payment{}, fmt.Errorf("load finalized payment: %w", err)
			}
			if stored.ProviderRef != result.ProviderRef {
				return application.Payment{}, application.ErrInvalidPayment
			}
			if err := tx.Commit(ctx); err != nil {
				return application.Payment{}, fmt.Errorf("commit payment replay: %w", err)
			}
			return stored, nil
		}
		return application.Payment{}, application.ErrInvalidPayment
	}
	if result.Status != application.PaymentPaid && result.Status != application.PaymentFailed {
		return application.Payment{}, application.ErrInvalidPayment
	}
	if _, err := tx.Exec(ctx, `UPDATE payments SET status=$2,provider_reference=NULLIF($3,''),updated_at=now() WHERE id=$1`, payment.ID, string(result.Status), result.ProviderRef); err != nil {
		return application.Payment{}, fmt.Errorf("update payment: %w", err)
	}
	if result.Status == application.PaymentPaid {
		tag, err := tx.Exec(ctx, `UPDATE orders SET status='PAID',updated_at=now() WHERE id=$1 AND status IN ('PENDING','CONFIRMED')`, payment.OrderID)
		if err != nil {
			return application.Payment{}, fmt.Errorf("mark order paid: %w", err)
		}
		if tag.RowsAffected() != 1 {
			return application.Payment{}, application.ErrOrderNotPayable
		}
	}
	if _, err := tx.Exec(ctx, `INSERT INTO audit_logs(actor_user_id,action,entity_type,entity_id,details)
		VALUES($1,$2,'payment',$3,jsonb_build_object('order_id',$4::bigint,'status',$5::text,'amount_minor',$6::bigint,'currency',$7::text))`, actorID, "payment."+string(result.Status), fmt.Sprint(payment.ID), payment.OrderID, string(result.Status), payment.AmountMinor, payment.Currency); err != nil {
		return application.Payment{}, fmt.Errorf("audit payment: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return application.Payment{}, fmt.Errorf("commit payment: %w", err)
	}
	payment.Status = result.Status
	payment.ProviderRef = result.ProviderRef
	return payment, nil
}

type rowScanner interface{ Scan(...any) error }

func scanPayment(row rowScanner) (application.Payment, error) {
	var payment application.Payment
	var status string
	err := row.Scan(&payment.ID, &payment.OrderID, &status, &payment.AmountMinor, &payment.Currency, &payment.Provider, &payment.ProviderRef)
	payment.Status = application.PaymentStatus(status)
	return payment, err
}

var _ application.PaymentStore = (*PaymentStore)(nil)
