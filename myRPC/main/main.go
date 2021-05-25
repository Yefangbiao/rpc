package main

import (
	"fmt"
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
		ServiceMethod := "Foo.Sum"
		args := fmt.Sprintf("myRPC req %d", i)
		var reply string
		err := client.Call(ServiceMethod, &args, &reply)
		if err != nil {
			log.Println(err)
		}

		log.Println("reply:", reply)
	}
}
