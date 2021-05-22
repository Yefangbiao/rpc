package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"rpc/myRPC"
	"rpc/myRPC/codec"
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

	// 启动客户端
	conn, _ := net.Dial("tcp", Addr)
	defer func() { _ = conn.Close() }()

	time.Sleep(1 * time.Second)
	json.NewEncoder(conn).Encode(myRPC.DefaultOption)
	cc := codec.NewJsonCodec(conn)
	// send request & receive response
	for i := 0; i < 5; i++ {
		h := &codec.Header{
			ServiceMethod: "Foo.Sum",
			Seq:           uint64(i),
		}
		_ = cc.Write(h, fmt.Sprintf("myRPC req %d", h.Seq))
		_ = cc.ReadHeader(h)
		var reply string
		_ = cc.ReadBody(&reply)
		log.Println("reply:", h, reply)
	}
}
