package main

import (
	"context"
	"fmt"
	"google.golang.org/grpc"
	"net"
	pb "rpc/grpc/1.introduction/proto"
)

const PORT = "50051"

func main() {
	// 创建 gRPC Server 对象，你可以理解为它是 Server 端的抽象对象
	server := grpc.NewServer()
	// 将 SayHello（其包含需要被调用的服务端接口）注册到 gRPC Server 的内部注册中心。
	//这样可以在接受到请求时，通过内部的服务发现，发现该服务端接口并转接进行逻辑处理
	pb.RegisterGreeterServer(server, &GreetService{})

	// 创建 Listen，监听 TCP 端口
	lis, err := net.Listen("tcp", ":"+PORT)
	if err != nil {
		fmt.Printf("net.Listen err: %v", err)
		return
	}

	// gRPC Server 开始 lis.Accept，直到 Stop 或 GracefulStop
	server.Serve(lis)
}

// 实现SayHello
type GreetService struct{}

func (s *GreetService) SayHello(ctx context.Context, r *pb.HelloRequest) (*pb.HelloReply, error) {
	return &pb.HelloReply{Message: "Hello," + r.GetName()}, nil
}
