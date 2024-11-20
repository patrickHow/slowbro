package main

import (
	"context"
	"fmt"
	"log"
	"net"

	pb "slowbro/internal/rateLimiter"

	"google.golang.org/grpc"
)

type rateLimiterServer struct {
	pb.UnimplementedRateLimiterServer
	// Other internal variables as required
}

func (s *rateLimiterServer) CheckRateLimit(ctx context.Context, rq *pb.RateLimitRequest) (*pb.RateLimitResponse, error) {
	// TODO skeleton code for now

	fmt.Println("Got request from: ", fmt.Sprintf("%s:%s:%s", rq.ServiceName, rq.Endpoint, rq.UserId))

	return &pb.RateLimitResponse{
		Allowed:  true,
		ErrorMsg: "",
	}, nil
}

func newServer() *rateLimiterServer {
	return &rateLimiterServer{}
}

func main() {
	fmt.Println("Slowbro starting...")
	// Set up our gRPC server, on a basic local port for now
	lis, err := net.Listen("tcp", "localhost:8080")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	// TODO once we have actual options to configure, potentially from comand line
	// via flags package?
	var opts []grpc.ServerOption
	grpcServer := grpc.NewServer(opts...)
	pb.RegisterRateLimiterServer(grpcServer, newServer())

	grpcServer.Serve(lis)
}
