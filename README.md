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
