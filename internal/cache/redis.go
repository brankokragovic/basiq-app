package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/basiq-app/pkg/models"
	"github.com/redis/go-redis/v9"
)

const (
	transactionsTTL    = 30 * time.Minute
	transactionsPrefix = "transactions:"
	syncMetaPrefix     = "sync_meta:"
)

type RedisCache struct {
	client *redis.Client
}

func NewRedisCache(addr string) *RedisCache {
	client := redis.NewClient(&redis.Options{
		Addr: addr,
	})
	return &RedisCache{client: client}
}

func (r *RedisCache) Ping(ctx context.Context) error {
	return r.client.Ping(ctx).Err()
}

func (r *RedisCache) SetTransactions(ctx context.Context, connectionID string, transactions []models.Transaction) error {
	data, err := json.Marshal(transactions)
	if err != nil {
		return fmt.Errorf("marshaling transactions: %w", err)
	}

	key := transactionsPrefix + connectionID
	return r.client.Set(ctx, key, data, transactionsTTL).Err()
}

func (r *RedisCache) GetTransactions(ctx context.Context, connectionID string) ([]models.Transaction, error) {
	key := transactionsPrefix + connectionID
	data, err := r.client.Get(ctx, key).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, nil
		}
		return nil, fmt.Errorf("getting cached transactions: %w", err)
	}

	var transactions []models.Transaction
	if err := json.Unmarshal(data, &transactions); err != nil {
		return nil, fmt.Errorf("unmarshaling cached transactions: %w", err)
	}

	return transactions, nil
}

func (r *RedisCache) SetLastPostDate(ctx context.Context, connectionID, postDate string) error {
	key := syncMetaPrefix + connectionID
	return r.client.Set(ctx, key, postDate, 0).Err()
}

func (r *RedisCache) GetLastPostDate(ctx context.Context, connectionID string) (string, error) {
	key := syncMetaPrefix + connectionID
	val, err := r.client.Get(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			return "", nil
		}
		return "", fmt.Errorf("getting last post date: %w", err)
	}
	return val, nil
}

func (r *RedisCache) SetUser(ctx context.Context, userID, email string) error {
	pipe := r.client.TxPipeline()
	pipe.Set(ctx, "session:user_id", userID, 0)
	pipe.Set(ctx, "session:user_email", email, 0)
	_, err := pipe.Exec(ctx)
	return err
}

func (r *RedisCache) GetUser(ctx context.Context) (userID, email string, err error) {
	userID, err = r.client.Get(ctx, "session:user_id").Result()
	if err != nil {
		if err == redis.Nil {
			return "", "", nil
		}
		return "", "", err
	}
	email, err = r.client.Get(ctx, "session:user_email").Result()
	if err != nil {
		if err == redis.Nil {
			return userID, "", nil
		}
		return "", "", err
	}
	return userID, email, nil
}

func (r *RedisCache) SetConnection(ctx context.Context, connectionID string) error {
	return r.client.Set(ctx, "session:connection_id", connectionID, 0).Err()
}

func (r *RedisCache) GetConnection(ctx context.Context) (string, error) {
	val, err := r.client.Get(ctx, "session:connection_id").Result()
	if err != nil {
		if err == redis.Nil {
			return "", nil
		}
		return "", err
	}
	return val, nil
}

func (r *RedisCache) Close() error {
	return r.client.Close()
}
