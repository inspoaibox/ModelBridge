package billing

import (
	"context"
	"os"
	"strings"
	"testing"

	dbpkg "ai-token/internal/db"
)

func TestReportingQueriesAgainstPostgres(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("DATABASE_URL is not configured")
	}
	ctx := context.Background()
	conn, err := dbpkg.Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if err := dbpkg.Migrate(ctx, conn, "../../migrations"); err != nil {
		t.Fatal(err)
	}
	service, err := NewSQLService(conn)
	if err != nil {
		t.Fatal(err)
	}
	usage, err := service.ListUsageRecords(ctx, UsageQuery{Limit: 20})
	if err != nil {
		t.Fatalf("usage report query failed: %v", err)
	}
	if usage.Limit != 20 || usage.Offset != 0 {
		t.Fatalf("unexpected usage pagination: %#v", usage)
	}
	finance, err := service.ListFinanceReport(ctx, FinanceQuery{Limit: 20})
	if err != nil {
		t.Fatalf("finance report query failed: %v", err)
	}
	if finance.Limit != 20 || finance.Offset != 0 {
		t.Fatalf("unexpected finance pagination: %#v", finance)
	}
	prices, err := service.ListPriceMatrix(ctx)
	if err != nil {
		t.Fatalf("price matrix query failed: %v", err)
	}
	for _, price := range prices {
		for _, estimate := range price.CostEstimates {
			if estimate.GroupID == "" || estimate.ChannelID == "" {
				t.Fatalf("price matrix cost estimate is missing route identity: %#v", estimate)
			}
		}
	}
}
