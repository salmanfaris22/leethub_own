package cache

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/redis/go-redis/v9"
)

func ConnectRedis() *redis.Client {
	rdb := redis.NewClient(&redis.Options{
		Addr:     "localhost:6379", // change if using Docker
		Password: "",
		DB:       0,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Fatalf("❌ Failed to connect to Redis: %v", err)
	}
	fmt.Println("✅ Connected to Redis")
	return rdb
}

func SavePrice(ctx context.Context, rdb *redis.Client, key string, price float64) {
	err := rdb.Set(ctx, key, fmt.Sprintf("%f", price), 0).Err()
	if err != nil {
		log.Printf("⚠️ Redis set error for %s: %v", key, err)
	}
}

func GetAllKeysValues(ctx context.Context, rdb *redis.Client) (map[string]string, error) {
	data := make(map[string]string)
	keys, err := rdb.Keys(ctx, "*").Result()
	if err != nil {
		return nil, err
	}

	for _, key := range keys {
		val, err := rdb.Get(ctx, key).Result()
		if err == nil {
			data[key] = val
		}
	}
	return data, nil
}
