package service

import (
	"context"
	"fmt"
	"log"
	"math"
	"sort"
	"strconv"

	"github.com/basiq-app/pkg/models"
)

type TransactionFetcher interface {
	GetTransactions(ctx context.Context, userID string) ([]models.Transaction, error)
	GetTransactionsSince(ctx context.Context, userID, sinceDate string) ([]models.Transaction, error)
}

type TransactionCache interface {
	GetTransactions(ctx context.Context, connectionID string) ([]models.Transaction, error)
	SetTransactions(ctx context.Context, connectionID string, transactions []models.Transaction) error
	GetLastPostDate(ctx context.Context, connectionID string) (string, error)
	SetLastPostDate(ctx context.Context, connectionID, postDate string) error
}

type Service struct {
	client TransactionFetcher
	cache  TransactionCache
}

func New(client TransactionFetcher, cache TransactionCache) *Service {
	return &Service{
		client: client,
		cache:  cache,
	}
}

func (s *Service) SyncTransactions(ctx context.Context, userID, connectionID string) ([]models.Transaction, error) {
	existing, err := s.cache.GetTransactions(ctx, connectionID)
	if err != nil {
		log.Printf("cache read error: %v", err)
	}

	lastPostDate, err := s.cache.GetLastPostDate(ctx, connectionID)
	if err != nil {
		log.Printf("sync meta read error: %v", err)
	}

	var newTxns []models.Transaction

	if lastPostDate != "" && len(existing) > 0 {
		log.Printf("incremental sync: fetching transactions since %s", lastPostDate)
		newTxns, err = s.client.GetTransactionsSince(ctx, userID, lastPostDate)
		if err != nil {
			return nil, fmt.Errorf("fetching new transactions: %w", err)
		}
	} else {
		log.Println("full sync: fetching all transactions")
		newTxns, err = s.client.GetTransactions(ctx, userID)
		if err != nil {
			return nil, fmt.Errorf("fetching transactions: %w", err)
		}
	}

	all := mergeTransactions(existing, newTxns)

	latestPostDate := lastPostDate
	for _, tx := range all {
		if tx.PostDate > latestPostDate {
			latestPostDate = tx.PostDate
		}
	}

	if err := s.cache.SetTransactions(ctx, connectionID, all); err != nil {
		log.Printf("cache write error: %v", err)
	}
	if latestPostDate != "" {
		if err := s.cache.SetLastPostDate(ctx, connectionID, latestPostDate); err != nil {
			log.Printf("sync meta write error: %v", err)
		}
	}

	return all, nil
}

func mergeTransactions(existing, incoming []models.Transaction) []models.Transaction {
	seen := make(map[string]struct{}, len(existing))
	merged := make([]models.Transaction, 0, len(existing)+len(incoming))

	for _, tx := range existing {
		seen[tx.ID] = struct{}{}
		merged = append(merged, tx)
	}
	for _, tx := range incoming {
		if _, ok := seen[tx.ID]; !ok {
			merged = append(merged, tx)
		}
	}

	return merged
}

type CategoryAverage struct {
	Code    string  `json:"code"`
	Average float64 `json:"average"`
}

func CalculateAverages(transactions []models.Transaction) []CategoryAverage {
	type accumulator struct {
		sum   float64
		count int
	}

	categories := make(map[string]*accumulator)

	for _, tx := range transactions {
		code := tx.SubClass.Code
		if code == "" || tx.Direction == "credit" {
			continue
		}

		amount, err := strconv.ParseFloat(tx.Amount, 64)
		if err != nil {
			continue
		}

		if _, ok := categories[code]; !ok {
			categories[code] = &accumulator{}
		}
		categories[code].sum += math.Abs(amount)
		categories[code].count++
	}

	var results []CategoryAverage
	for code, acc := range categories {
		results = append(results, CategoryAverage{
			Code:    code,
			Average: acc.sum / float64(acc.count),
		})
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Code < results[j].Code
	})

	return results
}
