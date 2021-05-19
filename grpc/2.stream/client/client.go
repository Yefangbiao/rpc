package main

import (
	"context"
	"fmt"
	"google.golang.org/grpc"
	"io"
	"log"

	pb "rpc/grpc/2.stream/proto"
)

const PORT = "50051"

type stuInfo struct {
	name string
	age  int
}

func main() {
	conn, err := grpc.Dial("localhost:"+PORT, grpc.WithInsecure())
	if err != nil {
		log.Fatalf("grpc.Dial err: %v", err)
	}
	defer conn.Close()

	client := pb.NewStreamServiceClient(conn)

	List(client, 1, 2)
	Update(client)
	Check(client)
}

func Update(client pb.StreamServiceClient) {
	stream, err := client.Update(context.Background())
	if err != nil {
		log.Fatalf("%v.Update(_) = _, %v", client, err)
	}

	// 更新一些数据
	for i := 1; i <= 2; i++ {
		err := stream.Send(&pb.StreamUpdateRequest{
			Id:  int32(i),
			Age: int32(i * 10),
		})
		if err != nil {
			log.Fatalf("grpc.Update err: %v", err)
		}
	}

	reply, err := stream.CloseAndRecv()
	if err != nil {
		log.Fatalf("%v.CloseAndRecv() got error %v, want %v", stream, err, nil)
	}

	log.Println("更改成功数量", reply.OK)
}

func List(client pb.StreamServiceClient, begin, end int32) {
	stream, err := client.List(context.Background(), &pb.StreamRangeRequest{
		Begin: begin,
		End:   end,
	})
	if err != nil {
		log.Fatalf("grpc.List err: %v", err)
	}
	for {
		stuInfo, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Fatalf("%v.List(_) = _, %v", client, err)
		}
		log.Println(stuInfo.Name, stuInfo.Age)
	}
}

func Check(client pb.StreamServiceClient) {
	stream, err := client.Check(context.Background())
	if err != nil {
		log.Fatalf("%v.Update(_) = _, %v", client, err)
	}

	waitc := make(chan interface{})

	go func() {
		// 接收消息
		for {
			info, err := stream.Recv()
			if err == io.EOF {
				// read done.
				close(waitc)
				return
			}
			if err != nil {
				log.Fatalf("Failed to receive a note : %v", err)
			}
			log.Printf("Got message of Student(%s, %d)", info.Name, info.Age)
		}
	}()

	for i := 0; i < 1; i++ {
		if err := stream.Send(&pb.StreamRangeRequest{Begin: int32(i + 1), End: int32(i + 2)}); err != nil {
			log.Fatalf("Failed to send a message : %v", err)
		}
	}
	stream.CloseSend()
	fmt.Printf("here")
	<-waitc
}
