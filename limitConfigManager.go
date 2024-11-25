package main

import (
	"context"
	"fmt"
	"log"
	pb "slowbro/internal/rateLimiter"
	"strconv"

	"github.com/redis/go-redis/v9"
)

const (
	defaultConfigDBInstace int    = 0
	maxConfigUpdateRetries int    = 1000
	configUpdateChannel    string = "endpointconfig"
)

type EndpointLimitConfig struct {
	TokenLimit float64 `redis:"token_limit"`
	RefillRate float64 `redis:"refill_rate"`
	Version    int     `redis:"version"`
}

type EndpointConfigManager struct {
	client    *redis.Client
	endpoints map[string]*EndpointLimitConfig
}

func decodeConfig(data map[string]string) (*EndpointLimitConfig, error) {
	config := &EndpointLimitConfig{}

	// Parse float values
	limit, err := strconv.ParseFloat(data["token_limit"], 64)
	if err != nil {
		return nil, fmt.Errorf("failed to parse limit: %v", err)
	}
	config.TokenLimit = limit

	refill, err := strconv.ParseFloat(data["refill_rate"], 64)
	if err != nil {
		return nil, fmt.Errorf("failed to parse refill rate: %v", err)
	}
	config.RefillRate = refill

	version, err := strconv.Atoi(data["version"])
	if err != nil {
		return nil, fmt.Errorf("failed to parse version: %v", err)
	}
	config.Version = version

	return config, nil
}

func (config *EndpointLimitConfig) toRedisMap() map[string]interface{} {
	return map[string]interface{}{
		"token_limit": config.TokenLimit,
		"refill_rate": config.RefillRate,
		"version":     config.Version,
	}
}

func NewEndpointConfigManager(dbPath string) *EndpointConfigManager {
	mgr := EndpointConfigManager{
		client: redis.NewClient(&redis.Options{
			Addr: dbPath,
			DB:   defaultConfigDBInstace,
		}),
		endpoints: make(map[string]*EndpointLimitConfig),
	}

	return &mgr
}

func getEndpointKey(req *pb.RateLimitEndpointPerUserConfig) string {
	return fmt.Sprintf("%s:%s", req.ServiceName, req.Endpoint)
}

func (mgr *EndpointConfigManager) UpdateOrAddEndpoint(req *pb.RateLimitEndpointPerUserConfig) error {

	for i := 0; i < maxConfigUpdateRetries; i++ {
		err := mgr.tryUpdate(req)

		if err == redis.TxFailedErr {
			continue
		}

		// Return any other error (or nil, indicating success)
		return err
	}

	log.Printf("[ERROR] Max retries exceeded for key: %v", getEndpointKey(req))
	return fmt.Errorf("max retries exceeded for request")
}

func (mgr *EndpointConfigManager) tryUpdate(req *pb.RateLimitEndpointPerUserConfig) error {
	key := getEndpointKey(req)

	err := mgr.client.Watch(context.Background(), func(tx *redis.Tx) error {
		// Retrieve or create config
		configData, err := tx.HGetAll(context.Background(), key).Result()
		if err != nil {
			log.Printf("[ERROR] Redis HGetAll failed for key: %v", key)
			return err
		}

		var config *EndpointLimitConfig
		if len(configData) == 0 {
			log.Printf("[INFO] Creating new config for key: %v", key)
			config = &EndpointLimitConfig{
				TokenLimit: float64(req.MaxTokens),
				RefillRate: float64(req.TokenFillRate),
				Version:    0,
			}
		} else {
			config, err = decodeConfig(configData)
			if err != nil {
				log.Printf("[ERROR] Failed to decode config for key: %s, error: %v", key, err)
				return err
			}
			log.Printf("[DEBUG] Retrieved config for key: %s", key)
		}

		// Update the configuration
		config.RefillRate = float64(req.TokenFillRate)
		config.TokenLimit = float64(req.MaxTokens)
		config.Version++

		// Attempt to update the database
		_, err = tx.TxPipelined(context.Background(), func(pipe redis.Pipeliner) error {
			err = pipe.HSet(context.Background(), key, config.toRedisMap()).Err()
			if err != nil {
				return err
			}

			// Success, update the in-memory struct
			mgr.endpoints[key] = config
			return nil
		})

		if err != nil {
			log.Printf("[ERROR] Failed to update config for key: %s", key)
			return err
		}

		return nil
	}, key)

	return err
}

func (mgr *EndpointConfigManager) GetEndpointConfig(key string) *EndpointLimitConfig {
	config, ok := mgr.endpoints[key]

	if !ok {
		// Config not found, return a default struct
		return &EndpointLimitConfig{
			TokenLimit: 0.0,
			RefillRate: 0.0,
			Version:    0,
		}
	}
	return config
}
