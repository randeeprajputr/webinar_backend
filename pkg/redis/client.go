package redis

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// Client wraps go-redis client with optional logger.
type Client struct {
	*redis.Client
	logger *zap.Logger
}

// NewClient creates a Redis client and verifies connectivity.
func NewClient(ctx context.Context, addr, password string, db int, logger *zap.Logger) (*Client, error) {
	rdb := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
	})

	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("redis ping: %w", err)
	}

	logger.Info("Redis client connected", zap.String("addr", addr))
	return &Client{Client: rdb, logger: logger}, nil
}
