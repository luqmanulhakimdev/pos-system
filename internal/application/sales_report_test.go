package application

import (
	"context"
	"testing"
	"time"
)

type salesReportStoreStub struct {
	from, to time.Time
	result   []SalesSummary
}

func (s *salesReportStoreStub) SalesSummary(_ context.Context, from, to time.Time) ([]SalesSummary, error) {
	s.from, s.to = from, to
	return s.result, nil
}

func TestSalesReportsValidatesInclusiveDateRangeAndReturnsCurrencyTotals(t *testing.T) {
	store := &salesReportStoreStub{result: []SalesSummary{{Currency: "IDR", PaidOrderCount: 3, GrossMinor: 2500, RefundedMinor: 500, NetMinor: 2000}}}
	report, err := NewSalesReports(store).Execute(context.Background(), "2026-09-01", "2026-09-30")
	if err != nil || report.From != "2026-09-01" || report.To != "2026-09-30" || len(report.Currencies) != 1 || report.Currencies[0].NetMinor != 2000 {
		t.Fatalf("report=%#v err=%v", report, err)
	}
	if !store.from.Equal(time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)) || !store.to.Equal(time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("range=(%s,%s)", store.from, store.to)
	}
}

func TestSalesReportsRejectsInvalidOrExcessiveRanges(t *testing.T) {
	service := NewSalesReports(&salesReportStoreStub{})
	for _, dates := range [][2]string{{"", "2026-09-30"}, {"2026-09-31", "2026-10-01"}, {"2026-10-02", "2026-10-01"}, {"2024-09-01", "2026-09-01"}} {
		if _, err := service.Execute(context.Background(), dates[0], dates[1]); err != ErrInvalidReportRange {
			t.Fatalf("range=%v err=%v", dates, err)
		}
	}
}
