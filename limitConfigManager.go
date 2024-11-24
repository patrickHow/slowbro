package main

import (
	pb "slowbro/internal/rateLimiter"

	"github.com/redis/go-redis/v9"
)

const DefaultConfigDBInstace int = 0

type EndpointLimitConfig struct {
	TokenLimit float64 `redis:"token_limit"`
	RefillRate float64 `redis:"refill_rate"`
	Version    int     `redis:"version"`
}

type EndpointConfigManager struct {
	client    *redis.Client
	endpoints map[string]EndpointLimitConfig
}

func NewEndpointConfigManager(dbPath string) *EndpointConfigManager {
	mgr := EndpointConfigManager{
		client: redis.NewClient(&redis.Options{
			Addr: dbPath,
			DB:   DefaultConfigDBInstace,
		}),
		endpoints: make(map[string]EndpointLimitConfig),
	}

	return &mgr
}

func (cfg *EndpointConfigManager) UpdateOrAddEndpoint(req *pb.RateLimitEndpointPerUserConfig) error {

}
