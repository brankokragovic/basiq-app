package service

import (
	"context"
	"testing"

	"github.com/basiq-app/pkg/models"
)

type mockFetcher struct {
	transactions []models.Transaction
}

func (m *mockFetcher) GetTransactions(_ context.Context, _ string) ([]models.Transaction, error) {
	return m.transactions, nil
}

func (m *mockFetcher) GetTransactionsSince(_ context.Context, _, _ string) ([]models.Transaction, error) {
	return m.transactions, nil
}

type mockCache struct {
	transactions []models.Transaction
	lastPostDate string
	saved        []models.Transaction
	savedDate    string
}

func (m *mockCache) GetTransactions(_ context.Context, _ string) ([]models.Transaction, error) {
	return m.transactions, nil
}

func (m *mockCache) SetTransactions(_ context.Context, _ string, txns []models.Transaction) error {
	m.saved = txns
	return nil
}

func (m *mockCache) GetLastPostDate(_ context.Context, _ string) (string, error) {
	return m.lastPostDate, nil
}

func (m *mockCache) SetLastPostDate(_ context.Context, _, date string) error {
	m.savedDate = date
	return nil
}

func TestSyncFullSync(t *testing.T) {
	fetcher := &mockFetcher{
		transactions: []models.Transaction{
			{ID: "1", PostDate: "2026-01-01"},
			{ID: "2", PostDate: "2026-01-02"},
		},
	}
	cache := &mockCache{}

	svc := New(fetcher, cache)
	result, err := svc.SyncTransactions(context.Background(), "user1", "conn1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result) != 2 {
		t.Fatalf("got %d transactions, want 2", len(result))
	}
	if cache.savedDate != "2026-01-02" {
		t.Errorf("saved date = %q, want %q", cache.savedDate, "2026-01-02")
	}
}

func TestSyncIncrementalNewTransactions(t *testing.T) {
	cached := []models.Transaction{
		{ID: "1", PostDate: "2026-01-01"},
		{ID: "2", PostDate: "2026-01-02"},
	}
	fetcher := &mockFetcher{
		transactions: []models.Transaction{
			{ID: "2", PostDate: "2026-01-02"},
			{ID: "3", PostDate: "2026-01-03"},
		},
	}
	cache := &mockCache{
		transactions: cached,
		lastPostDate: "2026-01-02",
	}

	svc := New(fetcher, cache)
	result, err := svc.SyncTransactions(context.Background(), "user1", "conn1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result) != 3 {
		t.Fatalf("got %d transactions, want 3", len(result))
	}

	ids := make(map[string]bool)
	for _, tx := range result {
		ids[tx.ID] = true
	}
	for _, id := range []string{"1", "2", "3"} {
		if !ids[id] {
			t.Errorf("missing transaction %s", id)
		}
	}

	if cache.savedDate != "2026-01-03" {
		t.Errorf("saved date = %q, want %q", cache.savedDate, "2026-01-03")
	}
	if len(cache.saved) != 3 {
		t.Errorf("saved %d transactions, want 3", len(cache.saved))
	}
}

func TestSyncIncrementalNoNewTransactions(t *testing.T) {
	cached := []models.Transaction{
		{ID: "1", PostDate: "2026-01-01"},
		{ID: "2", PostDate: "2026-01-02"},
	}
	fetcher := &mockFetcher{
		transactions: []models.Transaction{
			{ID: "2", PostDate: "2026-01-02"},
		},
	}
	cache := &mockCache{
		transactions: cached,
		lastPostDate: "2026-01-02",
	}

	svc := New(fetcher, cache)
	result, err := svc.SyncTransactions(context.Background(), "user1", "conn1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result) != 2 {
		t.Fatalf("got %d transactions, want 2", len(result))
	}
	if cache.savedDate != "2026-01-02" {
		t.Errorf("saved date = %q, want %q", cache.savedDate, "2026-01-02")
	}
}

func TestSyncCacheExpiredButMetaExists(t *testing.T) {
	fetcher := &mockFetcher{
		transactions: []models.Transaction{
			{ID: "1", PostDate: "2026-01-01"},
			{ID: "2", PostDate: "2026-01-02"},
			{ID: "3", PostDate: "2026-01-03"},
		},
	}
	cache := &mockCache{
		transactions: nil,
		lastPostDate: "2026-01-02",
	}

	svc := New(fetcher, cache)
	result, err := svc.SyncTransactions(context.Background(), "user1", "conn1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result) != 3 {
		t.Fatalf("got %d transactions, want 3", len(result))
	}
	if cache.savedDate != "2026-01-03" {
		t.Errorf("saved date = %q, want %q", cache.savedDate, "2026-01-03")
	}
}

func TestCalculateAverages(t *testing.T) {
	transactions := []models.Transaction{
		{ID: "1", Amount: "50.00", Direction: "debit", SubClass: models.SubClass{Code: "451"}},
		{ID: "2", Amount: "30.00", Direction: "debit", SubClass: models.SubClass{Code: "451"}},
		{ID: "3", Amount: "10.00", Direction: "debit", SubClass: models.SubClass{Code: "412"}},
		{ID: "4", Amount: "20.00", Direction: "debit", SubClass: models.SubClass{Code: "412"}},
		{ID: "5", Amount: "100.00", Direction: "credit", SubClass: models.SubClass{Code: "451"}},
		{ID: "6", Amount: "15.00", Direction: "debit", SubClass: models.SubClass{Code: ""}},
	}

	results := CalculateAverages(transactions)

	expected := map[string]float64{
		"451": 40.0,
		"412": 15.0,
	}

	if len(results) != len(expected) {
		t.Fatalf("got %d categories, want %d", len(results), len(expected))
	}

	for _, r := range results {
		want, ok := expected[r.Code]
		if !ok {
			t.Errorf("unexpected category %q", r.Code)
			continue
		}
		if r.Average != want {
			t.Errorf("category %q: got %.2f, want %.2f", r.Code, r.Average, want)
		}
	}
}

func TestCalculateAveragesNegativeAmounts(t *testing.T) {
	transactions := []models.Transaction{
		{ID: "1", Amount: "-25.00", Direction: "debit", SubClass: models.SubClass{Code: "451"}},
		{ID: "2", Amount: "-75.00", Direction: "debit", SubClass: models.SubClass{Code: "451"}},
	}

	results := CalculateAverages(transactions)

	if len(results) != 1 {
		t.Fatalf("got %d categories, want 1", len(results))
	}
	if results[0].Average != 50.0 {
		t.Errorf("got %.2f, want 50.00", results[0].Average)
	}
}

func TestCalculateAveragesEmpty(t *testing.T) {
	results := CalculateAverages(nil)
	if len(results) != 0 {
		t.Fatalf("got %d categories, want 0", len(results))
	}
}

func TestMergeTransactions(t *testing.T) {
	existing := []models.Transaction{
		{ID: "1", Amount: "10.00"},
		{ID: "2", Amount: "20.00"},
	}
	incoming := []models.Transaction{
		{ID: "2", Amount: "20.00"},
		{ID: "3", Amount: "30.00"},
	}

	merged := mergeTransactions(existing, incoming)

	if len(merged) != 3 {
		t.Fatalf("got %d transactions, want 3", len(merged))
	}

	ids := make(map[string]bool)
	for _, tx := range merged {
		ids[tx.ID] = true
	}
	for _, id := range []string{"1", "2", "3"} {
		if !ids[id] {
			t.Errorf("missing transaction %s", id)
		}
	}
}

func TestMergeTransactionsNoOverlap(t *testing.T) {
	existing := []models.Transaction{{ID: "1"}}
	incoming := []models.Transaction{{ID: "2"}}

	merged := mergeTransactions(existing, incoming)

	if len(merged) != 2 {
		t.Fatalf("got %d transactions, want 2", len(merged))
	}
}

func TestMergeTransactionsEmpty(t *testing.T) {
	merged := mergeTransactions(nil, nil)
	if len(merged) != 0 {
		t.Fatalf("got %d transactions, want 0", len(merged))
	}
}
