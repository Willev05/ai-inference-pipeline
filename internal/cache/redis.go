package cache

import (
	"ai-ingerence-pipeline/pkg/protocol"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"time"

	"github.com/redis/go-redis/v9"
)

//Wrapper for the redis.Client methods.

type Cache struct {
	client *redis.Client
}

// Returns a new cache struct.
func NewCache(redisAddr string) *Cache {
	redisOpt := redis.Options{
		Addr: redisAddr,
	}

	redisDB := redis.NewClient(&redisOpt)
	return &Cache{client: redisDB}
}

// The get function, simply wraps the redis get, returning the string and error.
func (c *Cache) Get(ctx context.Context, key string) (string, error) {
	return c.client.Get(ctx, key).Result()
}

// The set function, wraps redis and returns the potential error.
func (c *Cache) Set(ctx context.Context, key string, value string, ttl time.Duration) error {
	return c.client.Set(ctx, key, value, ttl).Err()
}

// Hashes the prompt for feeding to redis. Takes the model as well since different models yield different responses.
func (c *Cache) HashRequest(request *protocol.PromptRequest) string {
	hash := sha256.Sum256([]byte(*request.Model + *request.Prompt))
	return "cache:prompt:" + hex.EncodeToString(hash[:])
}

// Ping to test connection to redis.
func (c *Cache) Ping(ctx context.Context) error {
	return c.client.Ping(ctx).Err()
}
