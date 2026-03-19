package app

import (
	"context"
	"fmt"
	"log"

	"github.com/basiq-app/internal/basiq"
	"github.com/basiq-app/internal/cache"
	"github.com/basiq-app/internal/config"
	"github.com/basiq-app/internal/service"
)

type App struct {
	Cfg     *config.Config
	Client  *basiq.Client
	Cache   *cache.RedisCache
	Service *service.Service
}

func Setup(ctx context.Context) (*App, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("loading config: %w", err)
	}

	redisCache := cache.NewRedisCache(cfg.RedisHost)

	if err := redisCache.Ping(ctx); err != nil {
		return nil, fmt.Errorf("connecting to redis: %w", err)
	}
	log.Println("connected to redis")

	client := basiq.NewClient(cfg.APIURL, cfg.APIKey)

	log.Println("authenticating with Basiq API...")
	if err := client.Authenticate(ctx); err != nil {
		redisCache.Close()
		return nil, fmt.Errorf("authenticating: %w", err)
	}
	log.Println("authenticated successfully")

	svc := service.New(client, redisCache)

	return &App{
		Cfg:     cfg,
		Client:  client,
		Cache:   redisCache,
		Service: svc,
	}, nil
}

func (a *App) Close() {
	a.Cache.Close()
}
