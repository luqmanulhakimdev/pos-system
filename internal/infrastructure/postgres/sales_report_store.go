package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/luqmanulhakimdev/pos-system/internal/application"
)

type SalesReportStore struct{ pool *pgxpool.Pool }

func NewSalesReportStore(pool *pgxpool.Pool) *SalesReportStore { return &SalesReportStore{pool: pool} }

func (s *SalesReportStore) SalesSummary(ctx context.Context, from, to time.Time) ([]application.SalesSummary, error) {
	rows, err := s.pool.Query(ctx, `WITH paid_sales AS (
		SELECT currency,count(*)::bigint AS paid_order_count,sum(total_minor)::bigint AS gross_minor
		FROM orders WHERE status IN ('PAID','REFUNDED') AND created_at >= $1 AND created_at < $2 GROUP BY currency
	), paid_refunds AS (
		SELECT o.currency,sum(r.amount_minor)::bigint AS refunded_minor
		FROM refunds r JOIN orders o ON o.id=r.order_id
		WHERE r.status='SUCCEEDED' AND o.created_at >= $1 AND o.created_at < $2 GROUP BY o.currency
	)
	SELECT s.currency,s.paid_order_count,s.gross_minor,COALESCE(r.refunded_minor,0)::bigint,s.gross_minor-COALESCE(r.refunded_minor,0)::bigint
	FROM paid_sales s LEFT JOIN paid_refunds r ON r.currency=s.currency ORDER BY s.currency`, from, to)
	if err != nil {
		return nil, fmt.Errorf("query sales report: %w", err)
	}
	defer rows.Close()
	result := make([]application.SalesSummary, 0)
	for rows.Next() {
		var item application.SalesSummary
		if err := rows.Scan(&item.Currency, &item.PaidOrderCount, &item.GrossMinor, &item.RefundedMinor, &item.NetMinor); err != nil {
			return nil, fmt.Errorf("scan sales report: %w", err)
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read sales report: %w", err)
	}
	return result, nil
}

var _ application.SalesReportStore = (*SalesReportStore)(nil)
