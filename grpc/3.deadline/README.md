# 超时
有时候调用具有时间的限制，这时候就需要合理的设置超时

## 为什么要设置哟超时
+ 当未设置 Deadlines 时，将采用默认的 DEADLINE_EXCEEDED（这个时间非常大）
+ 如果产生了阻塞等待，就会造成大量正在进行的请求都会被保留，并且所有请求都有可能达到最大超时
+ 这会使服务面临资源耗尽的风险，例如内存，这会增加服务的延迟，或者在最坏的情况下可能导致整个进程崩溃

## gRPC
**proto**

这里我们复用 ``introduction``的代码
```protobuf
syntax = "proto3";

package proto;

// The greeting service definition.
service Greeter {
  // Sends a greeting
  rpc SayHello (HelloRequest) returns (HelloReply) {}
}

// The request message containing the user's name.
message HelloRequest {
  string name = 1;
}

// The response message containing the greetings
message HelloReply {
  string message = 1;
}
```



**client**

```go
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
```

`	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)`100毫秒之后超时



**Server**

服务端，使用一个通道模拟处理时间。如果超时返回错误

```go
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

```



**输出**

```
client.SayHello: rpc error: code = DeadlineExceeded desc = context deadline exceeded
```

