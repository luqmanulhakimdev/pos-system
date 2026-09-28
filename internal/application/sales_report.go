package application

import (
	"context"
	"errors"
	"time"
)

var ErrInvalidReportRange = errors.New("invalid report date range")

type SalesSummary struct {
	Currency       string `json:"currency"`
	PaidOrderCount int64  `json:"paid_order_count"`
	GrossMinor     int64  `json:"gross_minor"`
	RefundedMinor  int64  `json:"refunded_minor"`
	NetMinor       int64  `json:"net_minor"`
}

type SalesReport struct {
	From       string         `json:"from"`
	To         string         `json:"to"`
	Currencies []SalesSummary `json:"currencies"`
}

type SalesReportStore interface {
	SalesSummary(context.Context, time.Time, time.Time) ([]SalesSummary, error)
}

type SalesReports struct{ store SalesReportStore }

func NewSalesReports(store SalesReportStore) *SalesReports { return &SalesReports{store: store} }

func (s *SalesReports) Execute(ctx context.Context, fromValue, toValue string) (SalesReport, error) {
	if s == nil || s.store == nil {
		return SalesReport{}, ErrInvalidReportRange
	}
	from, err := time.Parse("2006-01-02", fromValue)
	if err != nil {
		return SalesReport{}, ErrInvalidReportRange
	}
	to, err := time.Parse("2006-01-02", toValue)
	if err != nil || to.Before(from) {
		return SalesReport{}, ErrInvalidReportRange
	}
	end := to.AddDate(0, 0, 1)
	if end.Sub(from) > 366*24*time.Hour {
		return SalesReport{}, ErrInvalidReportRange
	}
	summary, err := s.store.SalesSummary(ctx, from.UTC(), end.UTC())
	if err != nil {
		return SalesReport{}, err
	}
	return SalesReport{From: fromValue, To: toValue, Currencies: summary}, nil
}
