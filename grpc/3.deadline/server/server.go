package main

import (
	"context"
	"google.golang.org/grpc"
	"log"
	"net"
	pb "rpc/grpc/3.deadline/proto"
	"time"
)

const PORT = "50051"

func main() {
	server := grpc.NewServer()
	pb.RegisterGreeterServer(server, &HelloService{})

	lis, err := net.Listen("tcp", ":"+PORT)
	if err != nil {
		log.Fatalf("server: %v", err)
	}

	server.Serve(lis)
}

type HelloService struct{}

func (s *HelloService) SayHello(ctx context.Context, req *pb.HelloRequest) (*pb.HelloReply, error) {
	// 模拟调用
	waitc := make(chan interface{})

	go func() {
		defer close(waitc)
		time.Sleep(500 * time.Millisecond)
	}()

	for {
		select {
		case <-ctx.Done():
			// 处理超时
			return nil, ctx.Err()
		case <-waitc:
			return &pb.HelloReply{Message: "Hello, " + req.GetName()}, nil
		}
	}
}
