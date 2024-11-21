package main

import (
	"context"
	"fmt"
	"hash/fnv"
	pb "slowbro/internal/rateLimiter"

	"github.com/redis/go-redis/v9"
)

type ShardedRateLimiter struct {
	shards    []*rateLimiter
	numShards uint32
}

type rateLimiter struct {
	client *redis.Client
}

func NewShardedRateLimiter(dbPath string, numShards uint32) *ShardedRateLimiter {
	shardLimiter := ShardedRateLimiter{
		shards:    make([]*rateLimiter, numShards),
		numShards: numShards,
	}

	// TODO how many DBs can we hold in one redis instance?
	// Set up a redis client for each limiter
	for i, lim := range shardLimiter.shards {
		lim.client = redis.NewClient(&redis.Options{
			Addr: dbPath,
			DB:   i,
		})
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

func (r *rateLimiter) CheckRateLimit(key string) bool {

}

func (s *ShardedRateLimiter) CheckRateLimit(req *pb.RateLimitRequest) bool {
	key := getKeyFromRequest(req)
	shard := s.getShard(key)

	return shard.CheckLimit(req)
}

func (r *rateLimiter) CheckLimit(req *pb.RateLimitRequest) bool {
	ctx := context.Background()

	// See if the key for the request already exists

}
