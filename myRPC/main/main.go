package main

import (
	"log"
	"net"
	"rpc/myRPC"
	"time"
)

const Addr = "localhost:50001"

func server(addr string) {
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return
	}
	err = myRPC.Register(&Foo{})
	if err != nil {
		log.Println(err)
	}
	myRPC.Accept(lis)
}

func main() {
	// 启动服务端
	go server(Addr)

	// 新建客户端
	client, err := myRPC.Dial("tcp", Addr)
	if err != nil {
		log.Println(err)
		return
	}

	time.Sleep(1 * time.Second)
	// send request & receive response
	for i := 0; i < 5; i++ {
		ServiceMethod := "Foo.Add"
		args := Args{
			A: i,
			B: i + 1,
		}
		var reply Reply
		err := client.Call(ServiceMethod, &args, &reply)
		if err != nil {
			log.Println(err)
		}

		log.Println("reply:", reply)
	}
}

type Foo struct {
}

type Args struct {
	A, B int
}

type Reply struct {
	Val int
}

func (foo *Foo) Add(args *Args, reply *Reply) error {
	a, b := args.A, args.B
	reply.Val = a + b
	return nil
}
