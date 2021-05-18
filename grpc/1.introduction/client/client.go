package main

import (
	"context"
	"google.golang.org/grpc"
	"log"

	pb "rpc/grpc/1.introduction/proto"
)

const PORT = "50051"

func main() {
	// 创建与给定目标（服务端）的连接交互
	conn, err := grpc.Dial("localhost:"+PORT, grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		log.Fatalf("grpc.Dial err: %v", err)
	}
	defer conn.Close()

	// 创建grpc客户端
	client := pb.NewGreeterClient(conn)
	// 发送 RPC 请求，等待同步响应，得到回调后返回响应结果
	resp, err := client.SayHello(context.Background(), &pb.HelloRequest{
		Name: "Gopher",
	})
	if err != nil {
		log.Fatalf("client.Search err: %v", err)
	}

	// 输出响应结果
	log.Printf("resp: %s", resp)
}
