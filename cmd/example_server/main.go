package main

import (
	"log"
	"net"

	filesystem "github.com/RGood/fs-xfer/pkg/generated/filesystem"
	"github.com/RGood/fs-xfer/pkg/server"
	"google.golang.org/grpc"
)

func main() {
	lis, err := net.Listen("tcp", ":50051")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	grpcServer := grpc.NewServer()
	filesystem.RegisterStorageServiceServer(grpcServer, server.NewLocalStorageService("./data"))
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
