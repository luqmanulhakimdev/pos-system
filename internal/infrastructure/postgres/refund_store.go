package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/luqmanulhakimdev/pos-system/internal/application"
)

type RefundStore struct{ pool *pgxpool.Pool }

func NewRefundStore(pool *pgxpool.Pool) *RefundStore { return &RefundStore{pool: pool} }

func (s *RefundStore) PrepareRefund(ctx context.Context, orderID, actorID int64, key string, requestHash [32]byte, input application.RefundInput) (application.Refund, string, bool, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return application.Refund{}, "", false, fmt.Errorf("begin refund reservation: %w", err)
	}
	defer tx.Rollback(context.Background())
	var status string
	if err := tx.QueryRow(ctx, "SELECT status FROM orders WHERE id=$1 FOR UPDATE", orderID).Scan(&status); errors.Is(err, pgx.ErrNoRows) {
		return application.Refund{}, "", false, application.ErrOrderNotFound
	} else if err != nil {
		return application.Refund{}, "", false, fmt.Errorf("lock order for refund: %w", err)
	}
	var refund application.Refund
	var storedHash []byte
	err = tx.QueryRow(ctx, `SELECT id,order_id,payment_id,amount_minor,currency,status,reason,created_at,request_hash FROM refunds WHERE order_id=$1 AND idempotency_key=$2`, orderID, key).Scan(&refund.ID, &refund.OrderID, &refund.PaymentID, &refund.AmountMinor, &refund.Currency, &refund.Status, &refund.Reason, &refund.CreatedAt, &storedHash)
	if err == nil {
		if !refundEqualHash(storedHash, requestHash[:]) {
			return application.Refund{}, "", false, application.ErrRefundConflict
		}
		if refund.Status == "SUCCEEDED" {
			if err := tx.Commit(ctx); err != nil {
				return application.Refund{}, "", false, fmt.Errorf("commit refund replay: %w", err)
			}
			return refund, "", true, nil
		}
		var paymentReference string
		if err := tx.QueryRow(ctx, "SELECT provider_reference FROM payments WHERE id=$1 AND status='PAID'", refund.PaymentID).Scan(&paymentReference); err != nil {
			return application.Refund{}, "", false, fmt.Errorf("load payment reference for refund retry: %w", err)
		}
		if err := tx.Commit(ctx); err != nil {
			return application.Refund{}, "", false, fmt.Errorf("commit pending refund retry: %w", err)
		}
		return refund, paymentReference, false, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return application.Refund{}, "", false, fmt.Errorf("load idempotent refund: %w", err)
	}
	if status != "PAID" {
		return application.Refund{}, "", false, application.ErrOrderNotRefundable
	}
	var pending bool
	if err := tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM refunds WHERE order_id=$1 AND status='PENDING')", orderID).Scan(&pending); err != nil {
		return application.Refund{}, "", false, fmt.Errorf("check pending refund: %w", err)
	}
	if pending {
		return application.Refund{}, "", false, application.ErrRefundInProgress
	}
	var paymentReference string
	var capturedAmount int64
	if err := tx.QueryRow(ctx, `SELECT id,provider_reference,amount_minor,currency FROM payments WHERE order_id=$1 AND status='PAID' ORDER BY id DESC LIMIT 1 FOR UPDATE`, orderID).Scan(&refund.PaymentID, &paymentReference, &capturedAmount, &refund.Currency); errors.Is(err, pgx.ErrNoRows) {
		return application.Refund{}, "", false, application.ErrOrderNotRefundable
	} else if err != nil {
		return application.Refund{}, "", false, fmt.Errorf("load paid payment: %w", err)
	}
	if paymentReference == "" || capturedAmount <= 0 {
		return application.Refund{}, "", false, application.ErrOrderNotRefundable
	}
	var refundedAmount int64
	if err := tx.QueryRow(ctx, "SELECT COALESCE(sum(amount_minor),0) FROM refunds WHERE payment_id=$1 AND status='SUCCEEDED'", refund.PaymentID).Scan(&refundedAmount); err != nil {
		return application.Refund{}, "", false, fmt.Errorf("sum completed refunds: %w", err)
	}
	remaining := capturedAmount - refundedAmount
	if input.AmountMinor == 0 {
		refund.AmountMinor = remaining
	} else {
		refund.AmountMinor = input.AmountMinor
	}
	if refund.AmountMinor <= 0 || refund.AmountMinor > remaining {
		return application.Refund{}, "", false, application.ErrOrderNotRefundable
	}
	refund.OrderID, refund.Status, refund.Reason = orderID, "PENDING", input.Reason
	err = tx.QueryRow(ctx, `INSERT INTO refunds(order_id,payment_id,idempotency_key,request_hash,amount_minor,currency,status,reason)
		VALUES($1,$2,$3,$4,$5,$6,'PENDING',$7) RETURNING id,created_at`, orderID, refund.PaymentID, key, requestHash[:], refund.AmountMinor, refund.Currency, input.Reason).Scan(&refund.ID, &refund.CreatedAt)
	if err != nil {
		return application.Refund{}, "", false, fmt.Errorf("reserve refund: %w", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO audit_logs(actor_user_id,action,entity_type,entity_id,details) VALUES($1,'payment.refund_requested','refund',$2,jsonb_build_object('order_id',$3::bigint,'amount_minor',$4::bigint))`, actorID, fmt.Sprint(refund.ID), orderID, refund.AmountMinor); err != nil {
		return application.Refund{}, "", false, fmt.Errorf("audit refund request: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return application.Refund{}, "", false, fmt.Errorf("commit refund reservation: %w", err)
	}
	return refund, paymentReference, false, nil
}

func (s *RefundStore) FinalizeRefund(ctx context.Context, refund application.Refund, providerReference string, actorID int64) (application.Refund, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return application.Refund{}, fmt.Errorf("begin refund finalization: %w", err)
	}
	defer tx.Rollback(context.Background())
	var status string
	if err := tx.QueryRow(ctx, "SELECT status FROM refunds WHERE id=$1 FOR UPDATE", refund.ID).Scan(&status); errors.Is(err, pgx.ErrNoRows) {
		return application.Refund{}, application.ErrInvalidRefund
	} else if err != nil {
		return application.Refund{}, fmt.Errorf("lock refund: %w", err)
	}
	if status == "SUCCEEDED" {
		if err := tx.Commit(ctx); err != nil {
			return application.Refund{}, fmt.Errorf("commit refund replay: %w", err)
		}
		refund.Status = status
		return refund, nil
	}
	if status != "PENDING" {
		return application.Refund{}, application.ErrRefundInProgress
	}
	if _, err := tx.Exec(ctx, "UPDATE refunds SET status='SUCCEEDED',provider_reference=$2,updated_at=now() WHERE id=$1", refund.ID, providerReference); err != nil {
		return application.Refund{}, fmt.Errorf("complete refund record: %w", err)
	}
	var capturedAmount, refundedAmount int64
	if err := tx.QueryRow(ctx, `SELECT p.amount_minor,COALESCE(sum(r.amount_minor) FILTER(WHERE r.status='SUCCEEDED'),0) FROM payments p LEFT JOIN refunds r ON r.payment_id=p.id WHERE p.id=$1 GROUP BY p.id`, refund.PaymentID).Scan(&capturedAmount, &refundedAmount); err != nil {
		return application.Refund{}, fmt.Errorf("check remaining refundable amount: %w", err)
	}
	if refundedAmount >= capturedAmount {
		if tag, err := tx.Exec(ctx, "UPDATE payments SET status='REFUNDED',updated_at=now() WHERE id=$1 AND status='PAID'", refund.PaymentID); err != nil {
			return application.Refund{}, fmt.Errorf("mark payment refunded: %w", err)
		} else if tag.RowsAffected() != 1 {
			return application.Refund{}, application.ErrOrderNotRefundable
		}
		if tag, err := tx.Exec(ctx, "UPDATE orders SET status='REFUNDED',updated_at=now() WHERE id=$1 AND status='PAID'", refund.OrderID); err != nil {
			return application.Refund{}, fmt.Errorf("mark order refunded: %w", err)
		} else if tag.RowsAffected() != 1 {
			return application.Refund{}, application.ErrOrderNotRefundable
		}
	}
	if _, err := tx.Exec(ctx, `INSERT INTO audit_logs(actor_user_id,action,entity_type,entity_id,details) VALUES($1,'payment.refunded','order',$2,jsonb_build_object('refund_id',$3::bigint,'amount_minor',$4::bigint,'currency',$5::text))`, actorID, fmt.Sprint(refund.OrderID), refund.ID, refund.AmountMinor, refund.Currency); err != nil {
		return application.Refund{}, fmt.Errorf("audit refund: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return application.Refund{}, fmt.Errorf("commit refund: %w", err)
	}
	refund.Status = "SUCCEEDED"
	return refund, nil
}

func refundEqualHash(left, right []byte) bool {
	if len(left) != len(right) {
		return false
	}
	var diff byte
	for i := range left {
		diff |= left[i] ^ right[i]
	}
	return diff == 0
}

var _ application.RefundStore = (*RefundStore)(nil)
