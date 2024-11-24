package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"runtime"
	"time"

	pb "slowbro/internal/rateLimiter"

	"google.golang.org/grpc"
)

type rateLimiterServer struct {
	pb.UnimplementedRateLimiterServer
	limiter *ShardedRateLimiter
}

func (s *rateLimiterServer) CheckRateLimit(ctx context.Context, req *pb.RateLimitRequest) (*pb.RateLimitResponse, error) {

	allowed, err := s.limiter.CheckRateLimit(req)

	// Parse out the error message - passing err.Error() directly to the response causes a segfault
	// I guess protobuf doesn't like nil strings
	var emsg string
	if err != nil {
		emsg = err.Error()
	} else {
		emsg = ""
	}

	return &pb.RateLimitResponse{
		Allowed:  allowed,
		ErrorMsg: emsg,
	}, nil
}

func (s *rateLimiterServer) ConfigureRateLimit(ctx context.Context, req *pb.RateLimitEndpointPerUserConfig) (*pb.RateLimitConfigResponse, error) {

}

func newServer() *rateLimiterServer {
	return &rateLimiterServer{
		limiter: NewShardedRateLimiter("localhost:6379", runtime.GOMAXPROCS(0)),
	}
}

func main() {
	fmt.Println("Slowbro starting...")
	// Set up our gRPC server, on a basic local port for now
	lis, err := net.Listen("tcp", "localhost:8080")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	server := newServer()

	// TODO once we have actual options to configure, potentially from comand line
	// via flags package?
	var opts []grpc.ServerOption
	grpcServer := grpc.NewServer(opts...)
	pb.RegisterRateLimiterServer(grpcServer, server)

	// Spawn a background metrics process
	go func() {
		for {
			time.Sleep(10 * time.Second)
			server.limiter.PrintMetrics()
		}
	}()

	grpcServer.Serve(lis)
}
