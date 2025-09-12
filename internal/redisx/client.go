package redisx

import (
	"context"
	"github.com/redis/go-redis/v9"
	"time"
)

func New(addr string) *redis.Client {
	r := redis.NewClient(&redis.Options{Addr: addr})
	_ = r.WithTimeout(2 * time.Second)
	return r
}

func Exists(ctx context.Context, rdb *redis.Client, key string) (bool, error) {
	n, err := rdb.Exists(ctx, key).Result()
	return n > 0, err
}
