# Slowbro

**Currently under construction - still a WIP**

# TODO
- Configuration is not currently integrated - config messages can be received but limits will not yet be checked against these
- Batch processing needs to be implemented for each shard to reduce the number of Redis calls, which are currently the main source of latency
- 

## About
Slowbro is a rate limiting microservice that I developed as part of my journey learning Go and web development. 
While not intended to be ready for real-world applicaitons, it is meant to be functional and written with performance and good software engineering principles in mind.
Slowbro uses gRPC to define a communication interface. The gRPC proto file is included in the repo so client programs can generate the required code. 
Slowbro also uses Redis for distributed state management and configuration updates. Redis and gRPC were two of the main technologies I wanted to learn about while working on this implementation.

## Requirements
Slowbro requires a running Redis instance to work correctly. You can follow the Redis installation guide [here](https://redis.io/docs/latest/operate/oss_and_stack/install/install-stack/) to get started.

## Usage 
Slowbro is easy to start - currently it takes no options, but uses hardcoded local ports for communication:
- Port 8080 for incoming and outbound requests over gRPC
- Port 6379 for Redis

Both of these will be configurable at launch at some point. Once Slowbro is running, it will be ready to accept requests over gRPC.

## Internals
There are two RPCs available in slowbro currently:
```
    rpc CheckRateLimit(RateLimitRequest) returns (RateLimitResponse) {}
    rpc ConfigureRateLimit(RateLimitEndpointPerUserConfig) returns (RateLimitConfigResponse) {}
```

`ConfigureRateLimit` takes a service name, endpoint, token fill rate, and token limit and sets up the configuration in a Redis instance. This configuration is propagated from the node that received the config message to other nodes via Redis' pubsub mechanism. The config message response is a message containing an error string, which will be empty if no error occured.

`CheckRateLimit` accepts a user ID, service name, and endpoint. A token bucket is maintained on a per-user per-endpoint basis, with the limit for the user depending on the configuration for each endpoint. 

The limit for each user:endpoint pair is stored in Redis with a key formed as `service:endpoint:user_id`. When a request is received, the request is sent to a shard based on the hash of the key. Each shard has a Redis DB to store keys. The bucket for the key is then retrieved from Redis, refilled based on the time since last update, and checked against the cost of the request. The limiter will then send back a response, containing a boolean indicating if the request can be allowed an an error message.

