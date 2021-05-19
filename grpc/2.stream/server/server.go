package main

import (
	"google.golang.org/grpc"
	"io"
	"log"
	"net"
	pb "rpc/grpc/2.stream/proto"
)

// 存储学生信息
var studentInfo = map[int32]*stuInfo{}

func init() {
	// 初始化一些数据
	studentInfo[1] = &stuInfo{
		name: "张三",
		age:  23,
	}
	studentInfo[2] = &stuInfo{
		name: "李四",
		age:  30,
	}
}

type stuInfo struct {
	name string
	age  int
}

const PORT = "50051"

func main() {
	server := grpc.NewServer()
	pb.RegisterStreamServiceServer(server, &StreamService{})

	lis, err := net.Listen("tcp", ":"+PORT)
	if err != nil {
		log.Printf("net.Listen err: %v", err)
		return
	}

	server.Serve(lis)
}

type StreamService struct{}

func (s *StreamService) List(r *pb.StreamRangeRequest, stream pb.StreamService_ListServer) error {
	begin := r.GetBegin()
	end := r.GetEnd()

	// 如果有这个id,将信息发送给客户端
	for i := begin; i <= end; i++ {
		if info, ok := studentInfo[i]; ok {
			err := stream.Send(&pb.StreamStuResponse{
				Name: info.name,
				Age:  int32(info.age),
			})
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *StreamService) Update(stream pb.StreamService_UpdateServer) error {
	var okCount int32
	for {
		stuInfo, err := stream.Recv()
		if err == io.EOF {
			return stream.SendAndClose(&pb.StreamOKResponse{OK: int32(okCount)})
		}
		if err != nil {
			return err
		}
		id := stuInfo.GetId()
		if _, ok := studentInfo[id]; ok {
			studentInfo[id].age = int(stuInfo.GetAge())
			okCount++
		}
	}
	return nil
}

func (s *StreamService) Check(stream pb.StreamService_CheckServer) error {
	for {
		req, err := stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}

		begin, end := req.GetBegin(), req.GetEnd()
		for i := begin; i <= end; i++ {
			if info, ok := studentInfo[i]; ok {
				err := stream.Send(&pb.StreamStuResponse{
					Name: info.name,
					Age:  int32(info.age),
				})
				if err != nil {
					return err
				}
			}
		}

	}
	return nil
}
