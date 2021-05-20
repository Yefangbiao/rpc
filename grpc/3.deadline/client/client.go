package main

import (
	"context"
	"google.golang.org/grpc"
	"log"
	"time"

	pb "rpc/grpc/1.introduction/proto"
)

const PORT = "50051"

func main() {
	conn, err := grpc.Dial("localhost:"+PORT, grpc.WithInsecure())
	if err != nil {
		log.Fatalf("grpc.Dial: %v", err)
	}

	client := pb.NewGreeterClient(conn)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	resp, err := client.SayHello(ctx, &pb.HelloRequest{Name: "World!"})
	if err != nil {
		log.Fatalf("client.SayHello: %v", err)
	}
	log.Printf("received: %v", resp.GetMessage())
}
