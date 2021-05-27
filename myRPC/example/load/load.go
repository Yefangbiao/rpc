package main

import (
	"context"
	"log"
	"net"
	"rpc/myRPC"
	"rpc/myRPC/xclient"
	"sync"
	"time"
)

// 负载均衡测试
type Foo int
type Args struct{ Num1, Num2 int }

func (f *Foo) Sum(args *Args, reply *int) error {
	*reply = args.Num1 + args.Num2
	return nil
}

func (f *Foo) Sleep(args *Args, reply *int) error {
	time.Sleep(time.Second * time.Duration(args.Num1))
	*reply = args.Num1 + args.Num2
	return nil
}

func startServer(addr string) {
	var foo Foo
	l, err := net.Listen("tcp", addr)
	if err != nil {
		log.Println("startServer: " + err.Error())
	}

	server := myRPC.NewServer()
	_ = server.Register(&foo)
	server.Accept(l)
}

func foo(xc *xclient.XClient, ctx context.Context, typ, serviceMethod string, args *Args) {
	var reply int
	var err error
	switch typ {
	case "call":
		err = xc.Call(ctx, serviceMethod, args, &reply)
	case "broadcast":
		err = xc.Broadcast(ctx, serviceMethod, args, &reply)
	}
	if err != nil {
		log.Printf("%s %s error: %v", typ, serviceMethod, err)
	} else {
		log.Printf("%s %s success: %d + %d = %d", typ, serviceMethod, args.Num1, args.Num2, reply)
	}
}

func call(addr1, addr2 string) {
	d := xclient.NewMultiServerDiscovery([]string{addr1, addr2})
	xc := xclient.NewXClient(d, xclient.RandomSelect, &myRPC.DefaultOption)
	defer func() { _ = xc.Close() }()
	// send request & receive response
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			foo(xc, context.Background(), "call", "Foo.Sum", &Args{Num1: i, Num2: i * i})
		}(i)
	}
	wg.Wait()
}

func broadcast(addr1, addr2 string) {
	d := xclient.NewMultiServerDiscovery([]string{addr1, addr2})
	xc := xclient.NewXClient(d, xclient.RandomSelect, nil)
	defer func() { _ = xc.Close() }()
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			foo(xc, context.Background(), "broadcast", "Foo.Sum", &Args{Num1: i, Num2: i * i})
			// expect 2 - 5 timeout
			ctx, _ := context.WithTimeout(context.Background(), time.Second*2)
			foo(xc, ctx, "broadcast", "Foo.Sleep", &Args{Num1: i, Num2: i * i})
		}(i)
	}
	wg.Wait()
}

func main() {
	addr1 := "localhost:50001"
	addr2 := "localhost:50005"
	go startServer(addr1)
	go startServer(addr2)

	time.Sleep(time.Second)
	call(addr1, addr2)
	broadcast(addr1, addr2)
}
