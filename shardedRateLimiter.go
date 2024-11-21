package main

import (
	"context"
	"fmt"
	"hash/fnv"
	"log"
	"math"
	pb "slowbro/internal/rateLimiter"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"
)

// Default values for bucket configuration
const (
	defaultTokensInBucket float64 = 100
	defaultRqCost         float64 = 1.0
	defaultRefillRate     float64 = 1.0
	maxUpdateRetries      int     = 1000
)

type ShardedRateLimiter struct {
	shards    []*rateLimiter
	numShards uint32
}

type rateLimiter struct {
	id      int
	metrics *shardMetrics
	client  *redis.Client
}

// Type to store and retrieve from data store
type redisTokenBucket struct {
	Tokens         float64   `redis:"tokens" mapstructure:"tokens"`
	MaxTokens      float64   `redis:"max_tokens" mapstructure:"max_tokens"`
	RefillRate     float64   `redis:"refill_rate" mapstructure:"refill_rate"`
	LastRefillTime time.Time `redis:"last_refill_time" mapstructure:"last_refill_time"`
}

func (bucket *redisTokenBucket) refillBucket() {
	now := time.Now()
	timeSinceRefill := now.Sub(bucket.LastRefillTime)
	tokensToAdd := bucket.RefillRate * timeSinceRefill.Seconds()
	bucket.Tokens = math.Min(bucket.Tokens+tokensToAdd, bucket.MaxTokens)
	bucket.LastRefillTime = now
}

type shardMetrics struct {
	RequestCount       atomic.Int64
	AccumulatedLatency atomic.Int64 // Divide by request count to get average
}

func NewShardedRateLimiter(dbPath string, numShards int) *ShardedRateLimiter {
	shardLimiter := ShardedRateLimiter{
		shards:    make([]*rateLimiter, numShards),
		numShards: uint32(numShards),
	}

	// TODO how many DBs can we hold in one redis instance?
	// Initialize each shard with its own rate limiter
	for i := 0; i < numShards; i++ {
		shardLimiter.shards[i] = &rateLimiter{
			client: redis.NewClient(&redis.Options{
				Addr: dbPath,
				DB:   i,
			}),
			metrics: &shardMetrics{},
			id:      i,
		}
	}

	return &shardLimiter
}

func (s *ShardedRateLimiter) getShard(key string) *rateLimiter {
	// Hash the key into a shard index
	hash := fnv.New32a()
	hash.Write([]byte(key))
	shardIndex := hash.Sum32() % s.numShards

	return s.shards[shardIndex]
}

func getKeyFromRequest(req *pb.RateLimitRequest) string {
	return fmt.Sprintf("%s:%s:%s", req.ServiceName, req.Endpoint, req.UserId)
}

func decodeBucket(data map[string]string) (*redisTokenBucket, error) {
	bucket := &redisTokenBucket{}

	// Parse float values
	tokens, err := strconv.ParseFloat(data["tokens"], 64)
	if err != nil {
		return nil, fmt.Errorf("failed to parse tokens: %w", err)
	}
	bucket.Tokens = tokens

	maxTokens, err := strconv.ParseFloat(data["max_tokens"], 64)
	if err != nil {
		return nil, fmt.Errorf("failed to parse max_tokens: %w", err)
	}
	bucket.MaxTokens = maxTokens

	refillRate, err := strconv.ParseFloat(data["refill_rate"], 64)
	if err != nil {
		return nil, fmt.Errorf("failed to parse refill_rate: %w", err)
	}
	bucket.RefillRate = refillRate

	// Parse time value
	lastRefill, err := time.Parse(time.RFC3339, data["last_refill_time"])
	if err != nil {
		return nil, fmt.Errorf("failed to parse last_refill_time: %w", err)
	}
	bucket.LastRefillTime = lastRefill

	return bucket, nil
}

func (bucket *redisTokenBucket) toRedisMap() map[string]interface{} {
	return map[string]interface{}{
		"tokens":           bucket.Tokens,
		"max_tokens":       bucket.MaxTokens,
		"refill_rate":      bucket.RefillRate,
		"last_refill_time": bucket.LastRefillTime.Format(time.RFC3339),
	}
}

func (s *ShardedRateLimiter) CheckRateLimit(req *pb.RateLimitRequest) (bool, error) {
	key := getKeyFromRequest(req)
	shard := s.getShard(key)

	log.Printf("[INFO] Processing request for key: %s on shard: %d", key, shard.id)
	return shard.CheckLimit(req)
}

func (r *rateLimiter) CheckLimit(req *pb.RateLimitRequest) (bool, error) {
	start := time.Now()
	key := getKeyFromRequest(req)
	r.metrics.RequestCount.Add(1)
	defer func() {
		latency := time.Since(start).Microseconds()
		r.metrics.AccumulatedLatency.Add(latency)
		log.Printf("[DEBUG] Request completed for key: %s, latency: %dμs", key, latency)
	}()

	// Atomically update the bucket
	for retries := 0; retries < maxUpdateRetries; retries++ {
		if retries > 0 {
			log.Printf("[WARN] Retry attempt %d for key: %s", retries, key)
		}

		err := r.client.Watch(context.Background(), func(tx *redis.Tx) error {
			// Retrieve or create bucket
			bucketData, err := tx.HGetAll(context.Background(), key).Result()

			if err != nil {
				log.Printf("[ERROR] Redis HGetAll failed for key: %s, error: %v", key, err)
				return err
			}

			var bucket *redisTokenBucket
			if len(bucketData) == 0 {
				log.Printf("[INFO] Creating new bucket for key: %s", key)
				bucket = &redisTokenBucket{
					Tokens:         defaultTokensInBucket,
					MaxTokens:      defaultTokensInBucket,
					RefillRate:     defaultRefillRate,
					LastRefillTime: time.Now(),
				}
			} else {
				bucket, err = decodeBucket(bucketData)
				if err != nil {
					log.Printf("[ERROR] Failed to decode bucket for key: %s, error: %v", key, err)
					return err
				}
				log.Printf("[DEBUG] Retrieved bucket for key: %s, current tokens: %.2f", key, bucket.Tokens)
			}

			// Refill tokens, storing the old token value so we can check if we need to update
			oldTokens := bucket.Tokens
			bucket.refillBucket()

			if bucket.Tokens != oldTokens {
				log.Printf("[DEBUG] Refilled bucket for key: %s, old tokens: %.2f, new tokens: %.2f",
					key, oldTokens, bucket.Tokens)
			}

			// TODO make the cost part of the request package
			limitOk := false
			if bucket.Tokens > defaultRqCost {
				bucket.Tokens -= defaultRqCost
				limitOk = true
				log.Printf("[DEBUG] Consumed %.2f tokens for key: %s, remaining: %.2f",
					defaultRqCost, key, bucket.Tokens)
			} else {
				log.Printf("[INFO] Rate limit exceeded for key: %s, available tokens: %.2f",
					key, bucket.Tokens)
			}

			// Begin transaction to update bucket in store, if required
			// Occurs if tokens were taken or added
			if oldTokens != bucket.Tokens {
				_, err = tx.TxPipelined(context.Background(), func(pipe redis.Pipeliner) error {
					return pipe.HSet(context.Background(), key, bucket.toRedisMap()).Err()
				})

				// If we pipeline fails, "refund" the request cost so we don't drain the whole bucket on a failed request
				if err != nil {
					log.Printf("[ERROR] Failed to update bucket for key: %s, error: %v", key, err)
					bucket.Tokens += defaultRqCost
				}
			}

			if limitOk {
				return err
			} else {
				return fmt.Errorf("insufficient tokens")
			}

		}, key)

		if err == nil {
			return true, nil // Succsefully consumed token
		}

		if err == redis.TxFailedErr {
			// Retry transasction
			continue
		}

		// Other error
		return false, err
	}

	log.Printf("[ERROR] Max retries exceeded for key: %s", key)
	return false, fmt.Errorf("max retries exceeded for request")
}
