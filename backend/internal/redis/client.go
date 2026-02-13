package redis

import (
	"context"
	"fmt"
	"log"

	"github.com/redis/go-redis/v9"
)

// Client is the shared Redis client for the application.
var Client *redis.Client

// Initialize parses the Redis URL and establishes a connection.
func Initialize(redisURL string) error {
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return fmt.Errorf("invalid redis URL: %w", err)
	}
	Client = redis.NewClient(opts)

	if err := Client.Ping(context.Background()).Err(); err != nil {
		return fmt.Errorf("redis ping failed: %w", err)
	}
	log.Printf("Redis connected: %s", redisURL)
	return nil
}

// Close shuts down the Redis client.
func Close() {
	if Client != nil {
		Client.Close()
	}
}
