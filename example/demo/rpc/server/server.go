package main

import (
	"github.com/tiandh987/rpc/example/demo/service/Arith"
	"log"
	"net"
	"net/rpc"
)


func main() {
	rpc.Register(new(Arith.Arith))
	listener, err := net.Listen("tcp", ":1235")
	if err != nil {
		log.Fatal(err)
	}

	rpc.Accept(listener)
}
